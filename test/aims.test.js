import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { afterEach, beforeEach, test } from 'node:test';
import {
  loadConfig,
  loadState,
  profileDir,
  profileForHomeEnv,
  profileHomeEnv,
  saveConfig,
  updateProfileState,
} from '../src/config.js';
import { buildEnv, claudeKeychainService, evaluate, pickProfile } from '../src/health.js';
import { handle } from '../src/mcp.js';
import { addProfile, failover, removeProfile, statusReport, useProfile } from '../src/ops.js';
import { launch } from '../src/run.js';
import { envLines, shellInit } from '../src/shell.js';
import { windowsFromStatus } from '../src/statusline.js';
import { classifyFailure } from '../src/tools.js';
import { Sink, sandbox, seedClaudeHome, write } from './helpers.js';

let sb;
beforeEach(() => {
  sb = sandbox();
});
afterEach(() => sb.restore());

/** personal = existing ~/.claude login, work = new profile with its own login. */
function twoClaudeProfiles() {
  seedClaudeHome(sb.home);
  addProfile('claude', 'personal', { existing: true });
  addProfile('claude', 'work');
  const cfg = loadConfig();
  write(path.join(profileDir(cfg, 'claude', 'work'), '.credentials.json'), {
    claudeAiOauth: { accessToken: 'a', refreshToken: 'r', subscriptionType: 'team' },
  });
  return cfg;
}

const headless = (extra) =>
  launch({ id: 'claude', args: ['-p', 'hi'], quiet: true, input: Buffer.alloc(0), sink: new Sink(), errSink: new Sink(), ...extra });

test('classifies the real CLI failure messages', () => {
  assert.equal(classifyFailure("You've hit your limit · resets 5pm"), 'limit');
  assert.equal(classifyFailure("You've hit your usage limit. Upgrade to Plus to continue using Codex"), 'limit');
  assert.equal(classifyFailure('Usage limit reached'), 'limit');
  assert.equal(classifyFailure('OAuth token revoked · Please run /login'), 'auth');
  assert.equal(classifyFailure('Invalid API key · Please run /login'), 'auth');
  assert.equal(
    classifyFailure('Your access token could not be refreshed because your refresh token was already used. Please log out and sign in again.'),
    'auth',
  );
  assert.equal(classifyFailure('Error: ENOENT: no such file'), null);
});

test('macOS keychain entry name matches Claude Code (sha256 of the config dir)', () => {
  assert.equal(claudeKeychainService(null), 'Claude Code-credentials');
  const dir = '/Users/me/.aims/profiles/claude/work';
  const h = crypto.createHash('sha256').update(dir).digest('hex').slice(0, 8);
  assert.equal(claudeKeychainService(dir), `Claude Code-credentials-${h}`);
});

test('adds profiles: existing login runs with CLAUDE_CONFIG_DIR unset, new one gets its own dir', () => {
  const cfg = twoClaudeProfiles();
  assert.equal(profileHomeEnv(cfg, 'claude', 'personal'), null);
  const workDir = profileDir(cfg, 'claude', 'work');
  assert.equal(profileHomeEnv(cfg, 'claude', 'work'), workDir);
  assert.equal(cfg.tools.claude.active, 'personal');
  assert.equal(profileForHomeEnv(cfg, 'claude', undefined), 'personal');
  assert.equal(profileForHomeEnv(cfg, 'claude', workDir), 'work');

  // shared transcripts are visible from the work profile
  assert.ok(fs.existsSync(path.join(workDir, 'projects', '-repo', 'a.jsonl')));
  // .claude.json is seeded without account data
  const seeded = JSON.parse(fs.readFileSync(path.join(workDir, '.claude.json'), 'utf8'));
  assert.deepEqual(Object.keys(seeded.mcpServers), ['github']);
  assert.equal(seeded.userID, undefined);
  assert.equal(seeded.oauthAccount, undefined);
  assert.deepEqual(seeded.projects['/repo'], { allowedTools: ['Bash(ls)'], hasTrustDialogAccepted: true });

  const { env, dropped } = buildEnv(cfg, 'claude', 'personal', { CLAUDE_CONFIG_DIR: '/x', ANTHROPIC_API_KEY: 'k', PATH: '/bin' });
  assert.equal(env.CLAUDE_CONFIG_DIR, undefined);
  assert.equal(env.ANTHROPIC_API_KEY, undefined);
  assert.deepEqual(dropped, ['ANTHROPIC_API_KEY']);
  assert.equal(env.AIMS_SESSION_PROFILE, 'claude:personal');

  assert.throws(() => addProfile('claude', 'other', { existing: true }), /already uses the existing/);
  assert.throws(() => addProfile('claude', 'bad name'), /invalid profile name/);
});

test('keeps a user-set CLAUDE_CONFIG_DIR as the hub, verbatim', () => {
  process.env.CLAUDE_CONFIG_DIR = path.join(sb.home, 'custom-claude');
  addProfile('claude', 'personal', { existing: true });
  delete process.env.CLAUDE_CONFIG_DIR;
  const cfg = loadConfig();
  assert.equal(cfg.tools.claude.hub, path.join(sb.home, 'custom-claude'));
  assert.equal(profileHomeEnv(cfg, 'claude', 'personal'), path.join(sb.home, 'custom-claude'));
});

test('evaluate / pickProfile honour login state, cooldowns and usage', () => {
  let cfg = twoClaudeProfiles();
  addProfile('claude', 'spare'); // never logged in
  cfg = loadConfig();
  let pick = pickProfile(cfg, loadState(), 'claude', 'personal');
  assert.equal(pick.name, 'personal');

  updateProfileState('claude', 'personal', (s) => ({ ...s, until: new Date(Date.now() + 60000).toISOString(), reason: 'limit' }));
  pick = pickProfile(cfg, loadState(), 'claude', 'personal');
  assert.equal(pick.name, 'work');
  assert.match(pick.skipped[0].reasons[0], /^limit for/);

  updateProfileState('claude', 'work', (s) => ({
    ...s,
    usage: { windows: [{ label: '5h', pct: 99, resetsAt: new Date(Date.now() + 3600e3).toISOString() }] },
  }));
  pick = pickProfile(cfg, loadState(), 'claude', 'personal');
  assert.equal(pick.name, null);
  assert.deepEqual(
    pick.skipped.map((s) => s.name),
    ['personal', 'work', 'spare'],
  );
  assert.deepEqual(evaluate(cfg, loadState(), 'claude', 'spare').reasons, ['not logged in']);

  // an expired window no longer blocks
  updateProfileState('claude', 'work', (s) => ({
    ...s,
    usage: { windows: [{ label: '5h', pct: 99, resetsAt: new Date(Date.now() - 1000).toISOString() }] },
  }));
  assert.equal(pickProfile(cfg, loadState(), 'claude', 'personal').name, 'work');
});

test('headless run fails over to the next profile on a usage limit', async () => {
  twoClaudeProfiles();
  write(path.join(sb.home, '.claude', 'LIMITED'), '');
  const sink = new Sink();
  const r = await headless({ sink });
  assert.equal(r.code, 0);
  assert.equal(r.profile, 'work');
  assert.equal(sink.data.trim(), 'OK from work prompt=hi', 'failed attempt output is not mixed in');
  const st = loadState().claude.personal;
  assert.equal(st.reason, 'limit');
  assert.ok(Date.parse(st.until) > Date.now());

  // next run skips the limited profile up front
  const again = await headless();
  assert.equal(again.profile, 'work');
});

test('headless run marks a dead login and fails over', async () => {
  const cfg = twoClaudeProfiles();
  fs.rmSync(path.join(profileDir(cfg, 'claude', 'work'), '.credentials.json'));
  // work is active and not logged in -> skipped before launching
  useProfile('claude', 'work');
  const r = await headless();
  assert.equal(r.profile, 'personal');
});

test('explicit profile disables failover', async () => {
  twoClaudeProfiles();
  write(path.join(sb.home, '.claude', 'LIMITED'), '');
  const sink = new Sink();
  const r = await headless({ profile: 'personal', sink });
  assert.equal(r.code, 1);
  assert.equal(r.profile, 'personal');
  assert.equal(r.failure, 'limit');
  assert.match(sink.data, /hit your limit/);
});

test('terminal pin (AIMS_CLAUDE_PROFILE) beats the active profile', async () => {
  twoClaudeProfiles();
  process.env.AIMS_CLAUDE_PROFILE = 'work';
  const r = await headless();
  assert.equal(r.profile, 'work');
});

test('failover command marks the current profile and activates the next', () => {
  twoClaudeProfiles();
  const r = failover('claude', { minutes: 30 });
  assert.equal(r.from, 'personal');
  assert.equal(r.to, 'work');
  assert.equal(loadConfig().tools.claude.active, 'work');
  const until = Date.parse(loadState().claude.personal.until);
  assert.ok(until > Date.now() + 25 * 60000 && until < Date.now() + 35 * 60000);
  assert.throws(() => failover('claude'), /no other usable claude profile/);
  assert.equal(loadConfig().tools.claude.active, 'work', 'nothing changes when failover is impossible');
});

test('failover uses the reset time reported by the status line', () => {
  twoClaudeProfiles();
  const resetsAt = Math.floor(Date.now() / 1000) + 1800;
  const windows = windowsFromStatus({ rate_limits: { five_hour: { used_percentage: 98, resets_at: resetsAt } } });
  updateProfileState('claude', 'personal', (s) => ({ ...s, usage: { windows } }));
  failover('claude');
  assert.equal(Date.parse(loadState().claude.personal.until), resetsAt * 1000);
});

test('status line parser accepts epoch seconds, ISO and remaining/limit shapes', () => {
  const w = windowsFromStatus({
    rate_limits: {
      five_hour: { used_percentage: 42.44, resets_at: 1791300000 },
      seven_day: { remaining: 25, limit: 100, resets_at: '2026-10-10T00:00:00Z' },
    },
  });
  assert.deepEqual(w, [
    { label: '5h', pct: 42.4, resetsAt: new Date(1791300000 * 1000).toISOString() },
    { label: '7d', pct: 75, resetsAt: '2026-10-10T00:00:00.000Z' },
  ]);
  assert.deepEqual(windowsFromStatus({}), []);
});

test('use without a tool switches every tool that has the profile', () => {
  twoClaudeProfiles();
  fs.mkdirSync(path.join(sb.home, '.codex'), { recursive: true });
  addProfile('codex', 'personal', { existing: true });
  addProfile('codex', 'work');
  assert.deepEqual(useProfile(null, 'work'), ['claude', 'codex']);
  const cfg = loadConfig();
  assert.equal(cfg.tools.claude.active, 'work');
  assert.equal(cfg.tools.codex.active, 'work');
  assert.throws(() => useProfile(null, 'nope'), /no profile named/);
});

test('rm --purge deletes the profile dir but not shared data', () => {
  const cfg = twoClaudeProfiles();
  const dir = profileDir(cfg, 'claude', 'work');
  const r = removeProfile('claude', 'work', { purge: true });
  assert.ok(r.purged);
  assert.equal(fs.existsSync(dir), false);
  assert.ok(fs.existsSync(path.join(sb.home, '.claude', 'projects', '-repo', 'a.jsonl')));
  assert.ok(fs.existsSync(path.join(sb.home, '.claude', '.credentials.json')));
  // the adopted login can be removed from aims but its files are never purged
  removeProfile('claude', 'personal', { purge: true });
  assert.ok(fs.existsSync(path.join(sb.home, '.claude', 'settings.json')));
});

test('env and shell-init output', () => {
  const cfg = twoClaudeProfiles();
  const bash = envLines('bash', { profile: 'work' });
  assert.match(bash, /export AIMS_CLAUDE_PROFILE='work'/);
  assert.ok(bash.includes(`export CLAUDE_CONFIG_DIR='${profileDir(cfg, 'claude', 'work')}'`));
  assert.match(envLines('fish', { tool: 'claude', profile: 'personal' }), /set -e CLAUDE_CONFIG_DIR/);
  assert.match(envLines('powershell', { reset: true }), /Remove-Item Env:AIMS_CLAUDE_PROFILE/);
  assert.match(shellInit('bash'), /^claude\(\) \{ .* claude "\$@"; \}$/m);
  assert.match(shellInit('fish'), /^function codex --wraps codex; .* codex \$argv; end$/m);
  assert.match(shellInit('powershell'), /^function claude \{ .* claude @args \}$/m);
});

test('status report', () => {
  twoClaudeProfiles();
  const [claude] = statusReport({ ids: ['claude'] });
  assert.deepEqual(
    claude.profiles.map((p) => [p.name, p.active, p.email, p.plan, p.usable]),
    [
      ['personal', true, 'me@gmail.com', 'pro', true],
      ['work', false, null, 'team', true],
    ],
  );
});

test('config survives unknown / stale entries', () => {
  twoClaudeProfiles();
  const cfg = loadConfig();
  cfg.tools.claude.order = ['ghost', 'work'];
  cfg.tools.claude.active = 'ghost';
  saveConfig(cfg);
  const again = loadConfig();
  assert.deepEqual(again.tools.claude.order, ['work', 'personal']);
  assert.equal(again.tools.claude.active, null);
});

test('MCP: handshake, tool list, switching, errors', async () => {
  twoClaudeProfiles();
  const init = await handle({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18' } });
  assert.equal(init.result.protocolVersion, '2025-06-18');
  const future = await handle({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2099-01-01' } });
  assert.equal(future.result.protocolVersion, '2025-11-25');
  assert.equal(await handle({ jsonrpc: '2.0', method: 'notifications/initialized' }), null);
  const list = await handle({ jsonrpc: '2.0', id: 2, method: 'tools/list' });
  assert.deepEqual(
    list.result.tools.map((t) => t.name),
    ['aims_status', 'aims_switch', 'aims_failover', 'aims_clear', 'aims_run', 'aims_login_help'],
  );
  const sw = await handle({
    jsonrpc: '2.0',
    id: 3,
    method: 'tools/call',
    params: { name: 'aims_switch', arguments: { tool: 'claude', profile: 'work' } },
  });
  assert.match(sw.result.content[0].text, /now "work"/);
  assert.equal(loadConfig().tools.claude.active, 'work');
  const bad = await handle({
    jsonrpc: '2.0',
    id: 4,
    method: 'tools/call',
    params: { name: 'aims_switch', arguments: { profile: 'ghost' } },
  });
  assert.equal(bad.result.isError, true);
  const unknown = await handle({ jsonrpc: '2.0', id: 5, method: 'resources/list' });
  assert.equal(unknown.error.code, -32601);
  const run = await handle({
    jsonrpc: '2.0',
    id: 6,
    method: 'tools/call',
    params: { name: 'aims_run', arguments: { tool: 'claude', prompt: 'ping', profile: 'personal' } },
  });
  assert.match(run.result.content[0].text, /\[claude\/personal exit 0\]\nOK from default prompt=ping/);
});

test('a failed tool call inside a successful stream does not trigger failover', async () => {
  twoClaudeProfiles();
  // the fake prints a stream-json-ish tool error mentioning a rate limit, then succeeds
  process.env.FAKE_STREAM_TOOL_ERROR = '1';
  try {
    const sink = new Sink();
    const r = await headless({ sink });
    assert.equal(r.code, 0);
    assert.equal(r.profile, 'personal');
    assert.equal(((loadState().claude || {}).personal || {}).until, undefined);
  } finally {
    delete process.env.FAKE_STREAM_TOOL_ERROR;
  }
});

test('json result with is_error and exit 0 still fails over', async () => {
  twoClaudeProfiles();
  process.env.FAKE_JSON_LIMIT = '1';
  try {
    const r = await headless();
    assert.equal(r.profile, 'work');
  } finally {
    delete process.env.FAKE_JSON_LIMIT;
  }
});
