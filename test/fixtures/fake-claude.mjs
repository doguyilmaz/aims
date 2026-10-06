#!/usr/bin/env node
// Stand-in for the `claude` CLI used by the tests. Behaviour depends on the
// config dir it is pointed at, exactly like the real one.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const args = process.argv.slice(2);
const envDir = process.env.CLAUDE_CONFIG_DIR;
const dir = envDir || path.join(os.homedir(), '.claude');
const jsonFile = envDir ? path.join(envDir, '.claude.json') : path.join(os.homedir(), '.claude.json');
const credFile = path.join(dir, '.credentials.json');
const read = (f) => {
  try {
    return JSON.parse(fs.readFileSync(f, 'utf8'));
  } catch {
    return null;
  }
};
const label = path.basename(envDir || 'default');

if (args[0] === 'auth' && args[1] === 'status') {
  const cred = read(credFile);
  const acct = (read(jsonFile) || {}).oauthAccount || {};
  console.log(JSON.stringify({ loggedIn: !!cred, authMethod: cred ? 'claude.ai' : 'none', email: acct.emailAddress || null, subscriptionType: cred ? cred.claudeAiOauth.subscriptionType : null }));
  process.exit(cred ? 0 : 1);
}
if (args[0] === 'auth' && args[1] === 'login') {
  const email = process.env[`FAKE_EMAIL_${label.toUpperCase()}`] || `${label}@example.com`;
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(credFile, JSON.stringify({ claudeAiOauth: { accessToken: 'a', refreshToken: 'r', expiresAt: Date.now() + 3600e3, subscriptionType: 'max' } }));
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
  if (!fs.existsSync(credFile)) {
    console.log('Not logged in · Please run /login');
    process.exit(1);
  }
  if (process.env.FAKE_STREAM_TOOL_ERROR) {
    console.log(JSON.stringify({ type: 'user', message: { content: [{ type: 'tool_result', is_error: true, content: 'curl: 429 rate limit exceeded' }] } }));
  }
  if (process.env.FAKE_JSON_LIMIT && label === 'default') {
    console.log(JSON.stringify({ type: 'result', subtype: 'success', is_error: true, result: "You've hit your limit · resets 5pm" }));
    process.exit(0);
  }
  if (fs.existsSync(path.join(dir, 'LIMITED'))) {
    console.log("You've hit your limit · resets 5pm");
    process.exit(1);
  }
  const stdin = process.stdin.isTTY ? '' : fs.readFileSync(0, 'utf8');
  console.log(`OK from ${label}${stdin ? ` stdin=${stdin.trim()}` : ''} prompt=${args[args.indexOf('-p') + 1]}`);
  process.exit(0);
}
console.log(`interactive ${label} args=${JSON.stringify(args)} key=${process.env.ANTHROPIC_API_KEY || ''} session=${process.env.AIMS_SESSION_PROFILE || ''}`);
