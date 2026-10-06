import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const FAKE_CLAUDE = path.join(path.dirname(fileURLToPath(import.meta.url)), 'fixtures', 'fake-claude.mjs');

const SAVED = ['HOME', 'USERPROFILE', 'AIMS_HOME', 'AIMS_CLAUDE_BIN', 'AIMS_CODEX_BIN', 'CLAUDE_CONFIG_DIR', 'CODEX_HOME', 'AIMS_CLAUDE_PROFILE', 'AIMS_CODEX_PROFILE', 'ANTHROPIC_API_KEY'];

/** Point HOME / AIMS_HOME at a fresh temp dir for one test. */
export function sandbox() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'aims-test-'));
  const saved = Object.fromEntries(SAVED.map((k) => [k, process.env[k]]));
  const home = path.join(root, 'home');
  fs.mkdirSync(home, { recursive: true });
  process.env.HOME = home;
  process.env.USERPROFILE = home;
  process.env.AIMS_HOME = path.join(home, '.aims');
  process.env.AIMS_CLAUDE_BIN = FAKE_CLAUDE;
  for (const k of ['CLAUDE_CONFIG_DIR', 'CODEX_HOME', 'AIMS_CLAUDE_PROFILE', 'AIMS_CODEX_PROFILE', 'ANTHROPIC_API_KEY']) {
    delete process.env[k];
  }
  return {
    root,
    home,
    restore() {
      for (const [k, v] of Object.entries(saved)) {
        if (v === undefined) delete process.env[k];
        else process.env[k] = v;
      }
      fs.rmSync(root, { recursive: true, force: true });
    },
  };
}

export function write(file, content) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, typeof content === 'string' ? content : JSON.stringify(content));
}

/** A stream-like sink for launch() output. */
export class Sink {
  constructor() {
    this.data = '';
  }
  write(d) {
    this.data += d.toString();
    return true;
  }
}

/** Seed an existing Claude login in ~/.claude for the "personal" account. */
export function seedClaudeHome(home, { email = 'me@gmail.com' } = {}) {
  write(path.join(home, '.claude', '.credentials.json'), {
    claudeAiOauth: { accessToken: 'a', refreshToken: 'r', subscriptionType: 'pro' },
  });
  write(path.join(home, '.claude.json'), {
    oauthAccount: { emailAddress: email },
    userID: 'do-not-copy',
    mcpServers: { github: { command: 'gh-mcp' } },
    projects: { '/repo': { allowedTools: ['Bash(ls)'], hasTrustDialogAccepted: true, lastCost: 1 } },
  });
  write(path.join(home, '.claude', 'settings.json'), { model: 'opus' });
  write(path.join(home, '.claude', 'projects', '-repo', 'a.jsonl'), '{}\n');
}
