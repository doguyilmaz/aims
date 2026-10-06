import path from 'node:path';
import type { FailureKind, ToolId } from './types.ts';
import { home } from './util.ts';

export interface HeadlessOptions {
  prompt: string;
  model?: string;
  /** 'continue' = most recent conversation, otherwise a session id. */
  resume?: string | null;
}

export interface ToolSpec {
  id: ToolId;
  title: string;
  bin: string;
  /** Env var that overrides the binary (tests, custom installs). */
  binEnv: string;
  /** Env var that points the tool at a config directory. */
  homeEnv: string;
  defaultHub: () => string;
  /**
   * Entries of the tool's home that every profile links to. Anything not
   * listed -- above all the login files -- stays private to the profile.
   */
  shared: Array<string | RegExp>;
  /** Shared dirs created in the hub up front so profiles never fork them. */
  sharedDirs: string[];
  /** Conversation data: when a profile stops sharing, it starts empty instead of with a copy. */
  history: Array<string | RegExp>;
  /** Variables that override the profile's login; dropped from the child env. */
  conflictingEnv: string[];
  isHeadless: (args: string[]) => boolean;
  loginArgs: string[];
  logoutArgs: string[];
  resumeArgs: string[];
  headlessArgs: (o: HeadlessOptions) => string[];
  loginTip: string;
}

export const TOOLS: Record<ToolId, ToolSpec> = {
  claude: {
    id: 'claude',
    title: 'Claude Code',
    bin: 'claude',
    binEnv: 'AIMS_CLAUDE_BIN',
    homeEnv: 'CLAUDE_CONFIG_DIR',
    defaultHub: () => path.join(home(), '.claude'),
    shared: [
      'projects', // transcripts + auto memory -> `claude --continue` works across accounts
      'file-history',
      'todos',
      'plans',
      'tasks',
      'history.jsonl',
      'settings.json',
      'CLAUDE.md',
      'keybindings.json',
      'commands',
      'agents',
      'skills',
      'output-styles',
      'hooks',
      'plugins',
      'rules',
    ],
    sharedDirs: ['projects', 'file-history', 'todos', 'plans', 'commands', 'agents', 'skills'],
    history: ['projects', 'file-history', 'todos', 'plans', 'tasks', 'history.jsonl'],
    conflictingEnv: ['ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'CLAUDE_CODE_OAUTH_TOKEN'],
    isHeadless: (args) => args.includes('-p') || args.includes('--print'),
    loginArgs: ['auth', 'login'],
    logoutArgs: ['auth', 'logout'],
    resumeArgs: ['--continue'],
    // stream-json shows tool calls, so a failed run can be retried only when nothing ran
    headlessArgs: ({ prompt, model, resume }) => [
      ...(resume === 'continue' ? ['--continue'] : resume ? ['--resume', resume] : []),
      '-p',
      '--output-format',
      'stream-json',
      '--verbose',
      ...(model ? ['--model', model] : []),
      '--',
      prompt,
    ],
    loginTip:
      'The browser login uses whichever claude.ai account the browser is signed into. ' +
      'Use a private window, or pass the e-mail: aims login claude <profile> -- --email you@company.com',
  },
  codex: {
    id: 'codex',
    title: 'Codex',
    bin: 'codex',
    binEnv: 'AIMS_CODEX_BIN',
    homeEnv: 'CODEX_HOME',
    defaultHub: () => path.join(home(), '.codex'),
    shared: [
      'sessions', // rollouts -> `codex resume` works across accounts
      'archived_sessions',
      'history.jsonl',
      'config.toml',
      'AGENTS.md',
      'AGENTS.override.md',
      'prompts',
      'skills',
      'rules',
      'plugins',
      'memories',
      /^(state|memories|goals|queue)_\d+\.sqlite$/,
    ],
    sharedDirs: ['sessions', 'archived_sessions', 'prompts', 'skills'],
    history: ['sessions', 'archived_sessions', 'history.jsonl', 'memories', /\.sqlite$/],
    conflictingEnv: ['CODEX_API_KEY'],
    isHeadless: (args) => {
      const first = args.find((a) => !a.startsWith('-'));
      return first === 'exec' || first === 'e' || first === 'review';
    },
    loginArgs: ['login'],
    logoutArgs: ['logout'],
    resumeArgs: ['resume', '--last'],
    headlessArgs: ({ prompt, model, resume }) => [
      'exec',
      ...(resume === 'continue' ? ['resume', '--last'] : resume ? ['resume', resume] : []),
      '--json',
      '--skip-git-repo-check',
      ...(model ? ['--model', model] : []),
      '--',
      prompt,
    ],
    loginTip:
      'The browser login uses whichever ChatGPT account the browser is signed into. ' +
      'Use a private window, or device login: aims login codex <profile> -- --device-auth',
  },
};

export const TOOL_IDS = Object.keys(TOOLS) as ToolId[];

export function isToolId(id: unknown): id is ToolId {
  return typeof id === 'string' && Object.hasOwn(TOOLS, id);
}

export function getTool(id: string): ToolSpec {
  if (!isToolId(id)) throw new Error(`unknown tool "${id}" (expected: ${TOOL_IDS.join(', ')})`);
  return TOOLS[id];
}

export function binFor(tool: ToolSpec): string {
  return process.env[tool.binEnv] || tool.bin;
}

/**
 * Messages the CLIs print when a login is dead or a plan limit is hit.
 * Only ever applied to provider/CLI error output (see analyze.ts), never to
 * prompts or model text, so plain status codes are safe to match.
 */
const AUTH_RE =
  /please run \/login|oauth token (?:has expired|revoked)|invalid api key|not logged in|could not be refreshed|refresh token (?:was|has been) (?:already used|revoked)|sign in again|signing in again|log out and sign in|authentication required|run codex login|unauthori[sz]ed|\b401\b|token_expired/i;
const LIMIT_RE =
  /you(?:'|’)ve hit your (?:usage |session |weekly )?limit|hit your usage limit|usage limit reached|usage limit for|limit reached|rate[ _-]?limit(?:ed| exceeded)|quota exceeded|spend cap|too many requests|\b429\b/i;

export function classifyFailure(text: string | null | undefined): FailureKind | null {
  if (!text) return null;
  if (AUTH_RE.test(text)) return 'auth';
  if (LIMIT_RE.test(text)) return 'limit';
  return null;
}
