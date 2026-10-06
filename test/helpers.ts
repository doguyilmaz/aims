import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import type { Sink as SinkType } from '../src/types.ts';
import { home as aimsHomeDir } from '../src/util.ts';

export const FAKE_CLAUDE = path.join(import.meta.dir, 'fixtures', 'fake-claude.ts');

const SAVED = [
  'HOME',
  'USERPROFILE',
  'AIMS_HOME',
  'AIMS_CLAUDE_BIN',
  'AIMS_CODEX_BIN',
  'CLAUDE_CONFIG_DIR',
  'CODEX_HOME',
  'AIMS_CLAUDE_PROFILE',
  'AIMS_CODEX_PROFILE',
  'ANTHROPIC_API_KEY',
];

export interface Sandbox {
  root: string;
  home: string;
  restore(): void;
}

/** Point HOME / AIMS_HOME at a fresh temp dir for one test. */
export function sandbox(): Sandbox {
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
  // Never let a test touch the real ~/.claude or ~/.codex.
  if (aimsHomeDir() !== home) throw new Error(`sandbox HOME not honoured: ${aimsHomeDir()}`);
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

export function write(file: string, content: string | object): void {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, typeof content === 'string' ? content : JSON.stringify(content));
}

export const read = (file: string) => fs.readFileSync(file, 'utf8');

/** Collects launch() output. */
export class Sink implements SinkType {
  data = '';
  private decoder = new TextDecoder();
  write(d: string | Uint8Array): boolean {
    this.data += typeof d === 'string' ? d : this.decoder.decode(d, { stream: true });
    return true;
  }
}

/** An existing Claude login in ~/.claude (the "personal" account). */
export function seedClaudeHome(home: string, email = 'me@gmail.com'): void {
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
