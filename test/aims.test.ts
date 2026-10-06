import { afterEach, beforeEach, expect, test } from 'bun:test';
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { loadConfig, loadState, profileDir, profileForHomeEnv, profileHomeEnv, saveConfig, updateProfileState } from '../src/config.ts';
import { buildEnv, claudeKeychainService, evaluate, pickProfile } from '../src/health.ts';
import { handle } from '../src/mcp.ts';
import { addProfile, failover, removeProfile, statusReport, useProfile } from '../src/ops.ts';
import { type LaunchOptions, launch } from '../src/run.ts';
import { envLines, shellInit } from '../src/shell.ts';
import { windowsFromStatus } from '../src/statusline.ts';
import { classifyFailure } from '../src/tools.ts';
import type { Config } from '../src/types.ts';
import { type Sandbox, Sink, read, sandbox, seedClaudeHome, write } from './helpers.ts';

let sb: Sandbox;
beforeEach(() => {
  sb = sandbox();
});
afterEach(() => sb.restore());

/** personal = existing ~/.claude login, work = new profile with its own login. */
function twoClaudeProfiles(): Config {
  seedClaudeHome(sb.home);
  addProfile('claude', 'personal', { existing: true });
  addProfile('claude', 'work');
  const cfg = loadConfig();
  write(path.join(profileDir(cfg, 'claude', 'work'), '.credentials.json'), {
    claudeAiOauth: { accessToken: 'a', refreshToken: 'r', subscriptionType: 'team' },
  });
  return cfg;
}

const STREAM = ['-p', '--output-format', 'stream-json', '--verbose', '--', 'hi'];
const headless = (extra: Partial<LaunchOptions> = {}) =>
  launch({ id: 'claude', args: ['-p', 'hi'], quiet: true, input: null, sink: new Sink(), errSink: new Sink(), ...extra });

const claudeState = (name: string) => loadState().claude?.[name] ?? {};

async function withEnv<T>(vars: Record<string, string>, fn: () => Promise<T>): Promise<T> {
  Object.assign(process.env, vars);
  try {
    return await fn();
  } finally {
    for (const k of Object.keys(vars)) delete process.env[k];
  }
}

test('classifies the real CLI failure messages', () => {
  expect(classifyFailure("You've hit your limit · resets 5pm")).toBe('limit');
  expect(classifyFailure("You've hit your usage limit. Upgrade to Plus to continue using Codex")).toBe('limit');
  expect(classifyFailure('Usage limit reached')).toBe('limit');
  expect(classifyFailure('OAuth token revoked · Please run /login')).toBe('auth');
  expect(classifyFailure('Invalid API key · Please run /login')).toBe('auth');
  expect(
    classifyFailure('Your access token could not be refreshed because your refresh token was already used. Please log out and sign in again.'),
  ).toBe('auth');
  expect(classifyFailure('Error: ENOENT: no such file')).toBeNull();
});

test('macOS keychain entry name matches Claude Code (sha256 of the config dir)', () => {
  expect(claudeKeychainService(null)).toBe('Claude Code-credentials');
  const dir = '/Users/me/.aims/profiles/claude/work';
  const h = crypto.createHash('sha256').update(dir).digest('hex').slice(0, 8);
  expect(claudeKeychainService(dir)).toBe(`Claude Code-credentials-${h}`);
});

test('adds profiles: existing login runs with CLAUDE_CONFIG_DIR unset, new one gets its own dir', () => {
  const cfg = twoClaudeProfiles();
  expect(profileHomeEnv(cfg, 'claude', 'personal')).toBeNull();
  const workDir = profileDir(cfg, 'claude', 'work');
  expect(profileHomeEnv(cfg, 'claude', 'work')).toBe(workDir);
  expect(cfg.tools.claude.active).toBe('personal');
  expect(profileForHomeEnv(cfg, 'claude', undefined)).toBe('personal');
  expect(profileForHomeEnv(cfg, 'claude', workDir)).toBe('work');

  // shared transcripts are visible from the work profile
  expect(fs.existsSync(path.join(workDir, 'projects', '-repo', 'a.jsonl'))).toBe(true);
  // .claude.json is seeded without account data
  const seeded = JSON.parse(read(path.join(workDir, '.claude.json')));
  expect(Object.keys(seeded.mcpServers)).toEqual(['github']);
  expect(seeded.userID).toBeUndefined();
  expect(seeded.oauthAccount).toBeUndefined();
  expect(seeded.projects['/repo']).toEqual({ allowedTools: ['Bash(ls)'], hasTrustDialogAccepted: true });

  const { env, dropped } = buildEnv(cfg, 'claude', 'personal', { CLAUDE_CONFIG_DIR: '/x', ANTHROPIC_API_KEY: 'k', PATH: '/bin' });
  expect(env.CLAUDE_CONFIG_DIR).toBeUndefined();
  expect(env.ANTHROPIC_API_KEY).toBeUndefined();
  expect(dropped).toEqual(['ANTHROPIC_API_KEY']);
  expect(env.AIMS_SESSION_PROFILE).toBe('claude:personal');

  expect(() => addProfile('claude', 'other', { existing: true })).toThrow(/already uses the existing/);
  expect(() => addProfile('claude', 'bad name')).toThrow(/invalid profile name/);
});

test('keeps a user-set CLAUDE_CONFIG_DIR as the hub, verbatim', () => {
  const custom = path.join(sb.home, 'custom-claude');
  process.env.CLAUDE_CONFIG_DIR = custom;
  addProfile('claude', 'personal', { existing: true });
  delete process.env.CLAUDE_CONFIG_DIR;
  const cfg = loadConfig();
  expect(cfg.tools.claude.hub).toBe(custom);
  expect(profileHomeEnv(cfg, 'claude', 'personal')).toBe(custom);
});

test('evaluate / pickProfile honour login state, cooldowns and usage', () => {
  twoClaudeProfiles();
  addProfile('claude', 'spare'); // never logged in
  const cfg = loadConfig();
  expect(pickProfile(cfg, loadState(), 'claude', 'personal').name).toBe('personal');

  updateProfileState('claude', 'personal', (s) => ({ ...s, until: new Date(Date.now() + 60000).toISOString(), reason: 'limit' }));
  let pick = pickProfile(cfg, loadState(), 'claude', 'personal');
  expect(pick.name).toBe('work');
  expect(pick.skipped[0]?.reasons[0]).toMatch(/^limit for/);

  const window = (offset: number) => ({
    usage: { windows: [{ label: '5h', pct: 99, resetsAt: new Date(Date.now() + offset).toISOString() }] },
  });
  updateProfileState('claude', 'work', (s) => ({ ...s, ...window(3600e3) }));
  pick = pickProfile(cfg, loadState(), 'claude', 'personal');
  expect(pick.name).toBeNull();
  expect(pick.skipped.map((s) => s.name)).toEqual(['personal', 'work', 'spare']);
  expect(evaluate(cfg, loadState(), 'claude', 'spare').reasons).toEqual(['not logged in']);

  // an expired window no longer blocks
  updateProfileState('claude', 'work', (s) => ({ ...s, ...window(-1000) }));
  expect(pickProfile(cfg, loadState(), 'claude', 'personal').name).toBe('work');
});

test('headless run that failed before doing anything is retried on the next profile', async () => {
  twoClaudeProfiles();
  write(path.join(sb.home, '.claude', 'LIMITED'), '');
  const sink = new Sink();
  const r = await headless({ sink, args: STREAM });
  expect(r).toMatchObject({ code: 0, profile: 'work', finalText: 'OK from work prompt=hi' });
  expect(sink.data).toContain('OK from work');
  expect(sink.data).not.toContain('hit your limit'); // the failed attempt's output is not mixed in
  expect(claudeState('personal').reason).toBe('limit');
  expect(Date.parse(claudeState('personal').until!)).toBeGreaterThan(Date.now());

  // the next run skips the limited profile up front
  expect((await headless()).profile).toBe('work');
});

test('a profile that is not logged in is skipped before launching', async () => {
  const cfg = twoClaudeProfiles();
  fs.rmSync(path.join(profileDir(cfg, 'claude', 'work'), '.credentials.json'));
  useProfile('claude', 'work');
  expect((await headless()).profile).toBe('personal');
});

test('explicit profile disables failover', async () => {
  twoClaudeProfiles();
  write(path.join(sb.home, '.claude', 'LIMITED'), '');
  const sink = new Sink();
  const r = await headless({ profile: 'personal', sink });
  expect(r).toMatchObject({ code: 1, profile: 'personal', failure: 'limit' });
  expect(sink.data).toMatch(/hit your limit/);
});

test('terminal pin (AIMS_CLAUDE_PROFILE) beats the active profile', async () => {
  twoClaudeProfiles();
  process.env.AIMS_CLAUDE_PROFILE = 'work';
  expect((await headless()).profile).toBe('work');
});

test('a failed tool call inside a successful stream does not trigger failover', async () => {
  twoClaudeProfiles();
  const r = await withEnv({ FAKE_STREAM_TOOL_ERROR: '1' }, () => headless({ args: STREAM }));
  expect(r).toMatchObject({ code: 0, profile: 'personal' });
  expect(claudeState('personal').until).toBeUndefined();
});

test('a failed final result with exit code 0 still counts as a failure', async () => {
  twoClaudeProfiles();
  const r = await withEnv({ FAKE_JSON_LIMIT: '1' }, () => headless({ args: STREAM }));
  expect(r.profile).toBe('work');
});

test('failover command marks the current profile and activates the next', () => {
  twoClaudeProfiles();
  const r = failover('claude', { minutes: 30 });
  expect(r).toMatchObject({ from: 'personal', to: 'work' });
  expect(loadConfig().tools.claude.active).toBe('work');
  const until = Date.parse(claudeState('personal').until!);
  expect(until).toBeGreaterThan(Date.now() + 25 * 60000);
  expect(until).toBeLessThan(Date.now() + 35 * 60000);
  expect(() => failover('claude')).toThrow(/no other usable claude profile/);
  expect(loadConfig().tools.claude.active).toBe('work'); // nothing changes when failover is impossible
  expect(claudeState('work').until).toBeUndefined();
});

test('failover uses the reset time reported by the status line', () => {
  twoClaudeProfiles();
  const resetsAt = Math.floor(Date.now() / 1000) + 1800;
  const windows = windowsFromStatus({ rate_limits: { five_hour: { used_percentage: 98, resets_at: resetsAt } } });
  updateProfileState('claude', 'personal', (s) => ({ ...s, usage: { windows } }));
  failover('claude');
  expect(Date.parse(claudeState('personal').until!)).toBe(resetsAt * 1000);
});

test('status line parser accepts epoch seconds, ISO and remaining/limit shapes', () => {
  expect(
    windowsFromStatus({
      rate_limits: {
        five_hour: { used_percentage: 42.44, resets_at: 1791300000 },
        seven_day: { remaining: 25, limit: 100, resets_at: '2026-10-10T00:00:00Z' },
      },
    }),
  ).toEqual([
    { label: '5h', pct: 42.4, resetsAt: new Date(1791300000 * 1000).toISOString() },
    { label: '7d', pct: 75, resetsAt: '2026-10-10T00:00:00.000Z' },
  ]);
  expect(windowsFromStatus({})).toEqual([]);
});

test('use without a tool switches every tool that has the profile', () => {
  twoClaudeProfiles();
  fs.mkdirSync(path.join(sb.home, '.codex'), { recursive: true });
  addProfile('codex', 'personal', { existing: true });
  addProfile('codex', 'work');
  expect(useProfile(null, 'work')).toEqual(['claude', 'codex']);
  const cfg = loadConfig();
  expect(cfg.tools.claude.active).toBe('work');
  expect(cfg.tools.codex.active).toBe('work');
  expect(() => useProfile(null, 'nope')).toThrow(/no profile named/);
});

test('rm --purge deletes the profile dir but not shared data', () => {
  const cfg = twoClaudeProfiles();
  const dir = profileDir(cfg, 'claude', 'work');
  expect(removeProfile('claude', 'work', { purge: true }).purged).toBe(true);
  expect(fs.existsSync(dir)).toBe(false);
  expect(fs.existsSync(path.join(sb.home, '.claude', 'projects', '-repo', 'a.jsonl'))).toBe(true);
  expect(fs.existsSync(path.join(sb.home, '.claude', '.credentials.json'))).toBe(true);
  // the adopted login can be removed from aims, but its files are never purged
  removeProfile('claude', 'personal', { purge: true });
  expect(fs.existsSync(path.join(sb.home, '.claude', 'settings.json'))).toBe(true);
});

test('env and shell-init output', () => {
  const cfg = twoClaudeProfiles();
  const bash = envLines('bash', { profile: 'work' });
  expect(bash).toContain("export AIMS_CLAUDE_PROFILE='work'");
  expect(bash).toContain(`export CLAUDE_CONFIG_DIR='${profileDir(cfg, 'claude', 'work')}'`);
  expect(envLines('fish', { tool: 'claude', profile: 'personal' })).toContain('set -e CLAUDE_CONFIG_DIR');
  expect(envLines('powershell', { reset: true })).toContain('Remove-Item Env:AIMS_CLAUDE_PROFILE');
  expect(shellInit('bash')).toMatch(/^claude\(\) \{ .* claude "\$@"; \}$/m);
  expect(shellInit('fish')).toMatch(/^function codex --wraps codex; .* codex \$argv; end$/m);
  expect(shellInit('powershell')).toMatch(/^function claude \{ .* claude @args \}$/m);
});

test('status report', () => {
  twoClaudeProfiles();
  const [claude] = statusReport({ ids: ['claude'] });
  expect(claude!.profiles.map((p) => [p.name, p.active, p.email, p.plan, p.usable])).toEqual([
    ['personal', true, 'me@gmail.com', 'pro', true],
    ['work', false, null, 'team', true],
  ]);
});

test('config survives unknown / stale entries', () => {
  twoClaudeProfiles();
  const cfg = loadConfig();
  cfg.tools.claude.order = ['ghost', 'work'];
  cfg.tools.claude.active = 'ghost';
  saveConfig(cfg);
  const again = loadConfig();
  expect(again.tools.claude.order).toEqual(['work', 'personal']);
  expect(again.tools.claude.active).toBeNull();
});

test('MCP: handshake, tool list, switching, errors, run', async () => {
  twoClaudeProfiles();
  const call = (id: number, name: string, args: object) =>
    handle({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args as Record<string, unknown> } }) as Promise<
      Record<string, any>
    >;

  const init = (await handle({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18' } })) as any;
  expect(init.result.protocolVersion).toBe('2025-06-18');
  const future = (await handle({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2099-01-01' } })) as any;
  expect(future.result.protocolVersion).toBe('2025-11-25');
  expect(await handle({ jsonrpc: '2.0', method: 'notifications/initialized' })).toBeNull();

  const list = (await handle({ jsonrpc: '2.0', id: 2, method: 'tools/list' })) as any;
  expect(list.result.tools.map((t: { name: string }) => t.name)).toEqual([
    'aims_status',
    'aims_switch',
    'aims_failover',
    'aims_clear',
    'aims_run',
    'aims_login_help',
  ]);

  const sw = await call(3, 'aims_switch', { tool: 'claude', profile: 'work' });
  expect(sw.result.content[0].text).toMatch(/now "work"/);
  expect(loadConfig().tools.claude.active).toBe('work');

  expect((await call(4, 'aims_switch', { profile: 'ghost' })).result.isError).toBe(true);
  expect((await call(5, 'aims_clear', { tool: 'nope' })).error.code).toBe(-32602);
  expect(((await handle({ jsonrpc: '2.0', id: 6, method: 'resources/list' })) as any).error.code).toBe(-32601);

  const run = await call(7, 'aims_run', { tool: 'claude', prompt: 'ping', profile: 'personal' });
  expect(run.result.content[0].text).toBe('[claude/personal exit 0]\nOK from default prompt=ping');
});
