// Regression tests for issues found in review.
import { afterEach, beforeEach, expect, test } from 'bun:test';
import fs from 'node:fs';
import path from 'node:path';
import { analyzerFor } from '../src/analyze.ts';
import { configPath, loadConfig, loadState, profileDir, saveConfig, updateProfileState } from '../src/config.ts';
import { handle } from '../src/mcp.ts';
import { addProfile, failover, removeProfile, useProfile } from '../src/ops.ts';
import { type LaunchOptions, launch, syncProfile } from '../src/run.ts';
import { TOOLS, classifyFailure } from '../src/tools.ts';
import type { Config } from '../src/types.ts';
import { writeJsonAtomic } from '../src/util.ts';
import { type Sandbox, Sink, read, sandbox, seedClaudeHome, write } from './helpers.ts';

let sb: Sandbox;
beforeEach(() => {
  sb = sandbox();
});
afterEach(() => sb.restore());

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

const run = (args: string[], extra: Partial<LaunchOptions> = {}) =>
  launch({ id: 'claude', args, quiet: true, input: null, sink: new Sink(), errSink: new Sink(), ...extra });
const STREAM = ['-p', '--output-format', 'stream-json', '--verbose', '--', 'deploy'];
const hub = () => path.join(sb.home, '.claude');

async function withEnv<T>(vars: Record<string, string>, fn: () => Promise<T>): Promise<T> {
  Object.assign(process.env, vars);
  try {
    return await fn();
  } finally {
    for (const k of Object.keys(vars)) delete process.env[k];
  }
}

test('--dir may not overlap home, a hub or ~/.aims; custom dirs are never purged', () => {
  fs.mkdirSync(hub(), { recursive: true });
  expect(() => addProfile('claude', 'a', { dir: sb.home })).toThrow(/contains/);
  expect(() => addProfile('claude', 'b', { dir: hub() })).toThrow(/contains/);
  expect(() => addProfile('claude', 'c', { dir: path.join(hub(), 'sub') })).toThrow(/inside/);
  expect(() => addProfile('claude', 'd', { dir: path.join(sb.home, '.aims', 'x') })).toThrow(/inside/);
  const custom = path.join(sb.home, '.claude-work');
  addProfile('claude', 'work', { dir: custom });
  expect(() => addProfile('codex', 'clash', { dir: custom })).toThrow(/overlaps/);
  write(path.join(hub(), 'keep.txt'), 'x');
  expect(() => removeProfile('claude', 'work', { purge: true })).toThrow(/custom directory/);
  expect(fs.existsSync(custom)).toBe(true);
  expect(read(path.join(hub(), 'keep.txt'))).toBe('x');
});

test('a malformed config.json is reported, never overwritten', () => {
  twoClaudeProfiles();
  const broken = read(configPath()).replace(/}\s*$/, ',}');
  fs.writeFileSync(configPath(), broken);
  expect(() => loadConfig()).toThrow(/not valid JSON/);
  expect(() => useProfile(null, 'work')).toThrow(/not valid JSON/);
  expect(read(configPath())).toBe(broken);
});

test('aims_run without any profiles captures output instead of writing to the MCP stream', async () => {
  seedClaudeHome(sb.home);
  const res = (await handle({
    jsonrpc: '2.0',
    id: 1,
    method: 'tools/call',
    params: { name: 'aims_run', arguments: { tool: 'claude', prompt: 'ping' } },
  })) as any;
  expect(res.result.content[0].text).toBe('[claude/default exit 0]\nOK from default prompt=ping');
});

test('failover marks the profile that was actually launched, without stretching known resets', async () => {
  twoClaudeProfiles();
  const personalUntil = new Date(Date.now() + 2 * 3600e3).toISOString();
  updateProfileState('claude', 'personal', (s) => ({ ...s, until: personalUntil, reason: 'limit' }));
  expect((await run(['-p', 'x'])).profile).toBe('work'); // auto-skipped personal
  const r = failover('claude', { to: 'personal' });
  expect(r.from).toBe('work');
  expect(loadState().claude?.personal?.until).toBe(personalUntil);
});

test('failure classification ignores the prompt and model text', () => {
  const codex = analyzerFor('codex', ['exec', '--', 'why does our API say rate limit exceeded?']);
  for (const l of ['OpenAI Codex v0.160.1', 'user', 'why does our API say rate limit exceeded?']) codex.line('err', l);
  codex.line('err', '2026-10-06T11:57:47.870383Z ERROR codex_login::auth::manager: Failed to refresh token status=401 Unauthorized');
  codex.line('err', 'ERROR: Reconnecting... 2/5');
  expect(codex.errorText()).not.toContain('rate limit');
  expect(classifyFailure(codex.errorText())).toBe('auth');

  const json = analyzerFor('codex', ['exec', '--json', '--', 'x']);
  json.line('out', JSON.stringify({ type: 'item.completed', item: { type: 'command_execution' } }));
  json.line('out', JSON.stringify({ type: 'turn.failed', error: { message: 'workspace routing discovery unauthorized (401)' } }));
  expect(json.activity()).toBe('some');
  expect(json.resultFailed()).toBe(true);
  expect(classifyFailure(json.errorText())).toBe('auth');

  const claude = analyzerFor('claude', STREAM);
  claude.line('out', JSON.stringify({ type: 'user', message: { content: [{ type: 'tool_result', content: '429 rate limit exceeded' }] } }));
  claude.line('out', JSON.stringify({ type: 'result', is_error: true, result: 'API Error: 500 Internal server error' }));
  expect(classifyFailure(claude.errorText())).toBeNull();
});

test('a run that may have changed things is not retried', async () => {
  twoClaudeProfiles();
  write(path.join(hub(), 'LIMITED'), '');
  const log = path.join(sb.root, 'side-effects.log');
  // text output hides tool calls: mark the profile but don't re-run the task
  let r = await withEnv({ FAKE_TOOL_FIRST: '1', FAKE_SIDE_EFFECTS: log }, () => run(['-p', 'deploy']));
  expect(r).toMatchObject({ code: 1, profile: 'personal', failure: 'limit' });
  expect(read(log)).toBe('git push as default\n');
  expect(loadState().claude?.personal?.reason).toBe('limit');
  // stream-json shows the tool call: same outcome, now provably
  useProfile('claude', 'personal');
  fs.rmSync(log);
  updateProfileState('claude', 'personal', ({ until, ...s }) => s);
  r = await withEnv({ FAKE_TOOL_FIRST: '1', FAKE_SIDE_EFFECTS: log }, () => run(STREAM));
  expect(r).toMatchObject({ code: 1, profile: 'personal', failure: 'limit' });
  expect(read(log)).toBe('git push as default\n');
  // the next run goes to the other account up front
  expect((await run(['-p', 'next'])).profile).toBe('work');
});

test('SIGTERM to aims stops a headless child instead of orphaning it', async () => {
  twoClaudeProfiles();
  const pidFile = path.join(sb.root, 'child.pid');
  const proc = Bun.spawn(['bun', path.join(import.meta.dir, '..', 'src', 'cli.ts'), 'claude', '-p', 'hi'], {
    env: { ...process.env, FAKE_SLEEP: '20000', FAKE_PID_FILE: pidFile },
    stdin: 'ignore',
    stdout: 'ignore',
    stderr: 'ignore',
  });
  for (let i = 0; i < 100 && !fs.existsSync(pidFile); i++) await Bun.sleep(50);
  const childPid = Number(read(pidFile));
  expect(childPid).toBeGreaterThan(0);
  proc.kill('SIGTERM');
  await proc.exited;
  let alive = true;
  for (let i = 0; i < 40 && alive; i++) {
    try {
      process.kill(childPid, 0);
      await Bun.sleep(50);
    } catch {
      alive = false;
    }
  }
  expect(alive).toBe(false);
});

test('purge first moves real files the tool wrote into the shared folder', () => {
  const cfg = twoClaudeProfiles();
  write(path.join(hub(), 'history.jsonl'), 'old\n');
  const dir = profileDir(cfg, 'claude', 'work');
  syncProfile(cfg, 'claude', 'work');
  fs.unlinkSync(path.join(dir, 'history.jsonl'));
  write(path.join(dir, 'history.jsonl'), 'written by work\n');
  removeProfile('claude', 'work', { purge: true });
  expect(fs.existsSync(dir)).toBe(false);
  expect(read(path.join(hub(), 'history.jsonl'))).toBe('old\nwritten by work\n');
});

test('exclude / isolated added later detach the existing links', () => {
  twoClaudeProfiles();
  let cfg = loadConfig();
  const dir = profileDir(cfg, 'claude', 'work');
  expect(fs.lstatSync(path.join(dir, 'settings.json')).isSymbolicLink()).toBe(true);

  cfg.tools.claude.profiles.work!.exclude = ['settings.json'];
  saveConfig(cfg);
  syncProfile(loadConfig(), 'claude', 'work');
  const settings = path.join(dir, 'settings.json');
  expect(fs.lstatSync(settings).isSymbolicLink()).toBe(false);
  fs.writeFileSync(settings, '{"forceLoginOrgUUID":"work-org"}');
  expect(JSON.parse(read(path.join(hub(), 'settings.json')))).toEqual({ model: 'opus' });

  cfg = loadConfig();
  cfg.tools.claude.profiles.work!.isolated = true;
  saveConfig(cfg);
  syncProfile(loadConfig(), 'claude', 'work');
  expect(fs.existsSync(path.join(dir, 'projects'))).toBe(false); // conversation data is not copied
  expect(fs.existsSync(path.join(hub(), 'projects', '-repo', 'a.jsonl'))).toBe(true);
  expect(fs.readdirSync(dir).some((n) => fs.lstatSync(path.join(dir, n)).isSymbolicLink())).toBe(false);
});

test('prompts that start with "-" reach the tool as prompts', async () => {
  twoClaudeProfiles();
  expect(TOOLS.claude.headlessArgs({ prompt: '- list files' }).slice(-2)).toEqual(['--', '- list files']);
  expect(TOOLS.codex.headlessArgs({ prompt: '- list files' }).slice(-2)).toEqual(['--', '- list files']);
  const r = await run(TOOLS.claude.headlessArgs({ prompt: '- list files' }), { forceHeadless: true });
  expect(r.finalText).toBe('OK from default prompt=- list files');
});

test('writes go through symlinked config files (dotfiles) instead of replacing them', () => {
  const real = path.join(sb.root, 'dotfiles', 'settings.json');
  write(real, '{"a":1}');
  const link = path.join(sb.home, '.claude', 'settings.json');
  fs.mkdirSync(path.dirname(link), { recursive: true });
  fs.symlinkSync(real, link);
  writeJsonAtomic(link, { a: 2 });
  expect(fs.lstatSync(link).isSymbolicLink()).toBe(true);
  expect(JSON.parse(read(real))).toEqual({ a: 2 });
});
