#!/usr/bin/env bun
// Stand-in for the `claude` CLI in tests. Like the real one, its behaviour
// depends on the config dir it is pointed at (CLAUDE_CONFIG_DIR or ~/.claude).
//
// Knobs (files in the config dir / env vars):
//   LIMITED             -> -p runs fail with a usage-limit error
//   FAKE_TOOL_FIRST=1   -> run a "tool" (logged to FAKE_SIDE_EFFECTS) before failing
//   FAKE_SLEEP=ms       -> wait before answering (pid written to FAKE_PID_FILE)
//   FAKE_STREAM_TOOL_ERROR=1, FAKE_JSON_LIMIT=1 -> output edge cases
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const args = process.argv.slice(2);
const envDir = process.env.CLAUDE_CONFIG_DIR;
const homeDir = process.env.HOME || os.homedir();
const dir = envDir || path.join(homeDir, '.claude');
const jsonFile = envDir ? path.join(envDir, '.claude.json') : path.join(homeDir, '.claude.json');
const credFile = path.join(dir, '.credentials.json');
const label = path.basename(envDir || 'default');
const read = (f: string): any => {
  try {
    return JSON.parse(fs.readFileSync(f, 'utf8'));
  } catch {
    return null;
  }
};
const emit = (o: object) => console.log(JSON.stringify(o));

if (args[0] === 'auth' && args[1] === 'status') {
  const cred = read(credFile);
  const acct = (read(jsonFile) || {}).oauthAccount || {};
  emit({
    loggedIn: !!cred,
    authMethod: cred ? 'claude.ai' : 'none',
    email: acct.emailAddress || null,
    subscriptionType: cred ? cred.claudeAiOauth.subscriptionType : null,
  });
  process.exit(cred ? 0 : 1);
}
if (args[0] === 'auth' && args[1] === 'login') {
  const email = process.env[`FAKE_EMAIL_${label.toUpperCase()}`] || `${label}@example.com`;
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(
    credFile,
    JSON.stringify({ claudeAiOauth: { accessToken: 'a', refreshToken: 'r', expiresAt: Date.now() + 3600e3, subscriptionType: 'max' } }),
  );
  const j = read(jsonFile) || {};
  j.oauthAccount = { emailAddress: email };
  fs.writeFileSync(jsonFile, JSON.stringify(j));
  console.log('Login successful.');
  process.exit(0);
}
if (args[0] === 'auth' && args[1] === 'logout') {
  fs.rmSync(credFile, { force: true });
  process.exit(0);
}
if (args[0] === 'mcp' && args[1] === 'add') {
  const j = read(jsonFile) || {};
  j.mcpServers = j.mcpServers || {};
  if (j.mcpServers.aims) {
    console.error('MCP server aims already exists in user config');
    process.exit(1);
  }
  const sep = args.indexOf('--');
  j.mcpServers.aims = { type: 'stdio', command: args[sep + 1], args: args.slice(sep + 2) };
  fs.writeFileSync(jsonFile, JSON.stringify(j));
  process.exit(0);
}

if (args.includes('-p') || args.includes('--print')) {
  const sep = args.indexOf('--');
  const prompt = sep >= 0 ? args[sep + 1] : args[args.indexOf('-p') + 1];
  const fmtAt = args.indexOf('--output-format');
  const format = fmtAt >= 0 ? args[fmtAt + 1] : 'text';
  const stream = format === 'stream-json';
  const finish = (ok: boolean, text: string, code: number) => {
    if (format === 'text') console.log(text);
    else emit({ type: 'result', subtype: ok ? 'success' : 'error', is_error: !ok, result: text });
    process.exit(code);
  };
  if (process.env.FAKE_SLEEP) {
    if (process.env.FAKE_PID_FILE) fs.writeFileSync(process.env.FAKE_PID_FILE, String(process.pid));
    await Bun.sleep(Number(process.env.FAKE_SLEEP));
  }
  if (stream) emit({ type: 'system', subtype: 'init' });
  if (!fs.existsSync(credFile)) finish(false, 'Not logged in · Please run /login', 1);
  if (process.env.FAKE_STREAM_TOOL_ERROR) {
    emit({ type: 'user', message: { content: [{ type: 'tool_result', is_error: true, content: 'curl: 429 rate limit exceeded' }] } });
  }
  if (process.env.FAKE_JSON_LIMIT && label === 'default') {
    emit({ type: 'result', subtype: 'success', is_error: true, result: "You've hit your limit · resets 5pm" });
    process.exit(0);
  }
  if (fs.existsSync(path.join(dir, 'LIMITED'))) {
    if (process.env.FAKE_TOOL_FIRST) {
      if (process.env.FAKE_SIDE_EFFECTS) fs.appendFileSync(process.env.FAKE_SIDE_EFFECTS, `git push as ${label}\n`);
      if (stream) emit({ type: 'assistant', message: { content: [{ type: 'tool_use', name: 'Bash', input: { command: 'git push' } }] } });
    }
    finish(false, "You've hit your limit · resets 5pm", 1);
  }
  if (stream) emit({ type: 'assistant', message: { content: [{ type: 'text', text: 'OK' }] } });
  const stdin = process.stdin.isTTY ? '' : fs.readFileSync(0, 'utf8');
  finish(true, `OK from ${label}${stdin ? ` stdin=${stdin.trim()}` : ''} prompt=${prompt}`, 0);
}

console.log(
  `interactive ${label} args=${JSON.stringify(args)} key=${process.env.ANTHROPIC_API_KEY || ''} session=${process.env.AIMS_SESSION_PROFILE || ''}`,
);
