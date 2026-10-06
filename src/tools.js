import path from 'node:path';
import { home } from './util.js';

/**
 * Everything aims needs to know about each CLI.
 *
 * `shared` lists the entries of the tool's home directory that every profile
 * links to (history, transcripts, settings, skills...). Anything not listed --
 * most importantly the login files -- stays private to the profile. A string
 * is an exact name; a RegExp matches names found in the hub or profile dir.
 */
export const TOOLS = {
  claude: {
    id: 'claude',
    title: 'Claude Code',
    bin: 'claude',
    binEnv: 'AIMS_CLAUDE_BIN',
    homeEnv: 'CLAUDE_CONFIG_DIR',
    defaultHub: () => path.join(home(), '.claude'),
    authFiles: ['.credentials.json'],
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
    // These override the per-profile login, so aims drops them from the child env.
    conflictingEnv: ['ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'CLAUDE_CODE_OAUTH_TOKEN'],
    isHeadless: (args) => args.includes('-p') || args.includes('--print'),
    loginArgs: ['auth', 'login'],
    logoutArgs: ['auth', 'logout'],
    resumeArgs: ['--continue'],
    headlessArgs: ({ prompt, model, resume }) => [
      ...(resume === 'continue' ? ['--continue'] : resume ? ['--resume', resume] : []),
      '-p',
      prompt,
      ...(model ? ['--model', model] : []),
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
    authFiles: ['auth.json'],
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
      '--skip-git-repo-check',
      ...(model ? ['--model', model] : []),
      prompt,
    ],
    loginTip:
      'The browser login uses whichever ChatGPT account the browser is signed into. ' +
      'Use a private window, or device login: aims login codex <profile> -- --device-auth',
  },
};

export const TOOL_IDS = Object.keys(TOOLS);

export function getTool(id) {
  const t = TOOLS[id];
  if (!t) throw new Error(`unknown tool "${id}" (expected: ${TOOL_IDS.join(', ')})`);
  return t;
}

export function binFor(tool) {
  return process.env[tool.binEnv] || tool.bin;
}

/** Messages the CLIs print when a login is dead or a plan limit is hit. */
const AUTH_RE =
  /please run \/login|oauth token (?:has expired|revoked)|invalid api key|not logged in|could not be refreshed|refresh token (?:was|has been) (?:already used|revoked)|sign in again|log out and sign in|authentication required|run codex login/i;
const LIMIT_RE =
  /you(?:'|’)ve hit your (?:usage |session |weekly )?limit|hit your usage limit|usage limit reached|usage limit for|limit reached|rate[ _-]?limit(?:ed| exceeded)|quota exceeded|spend cap/i;

export function classifyFailure(text) {
  if (!text) return null;
  if (AUTH_RE.test(text)) return 'auth';
  if (LIMIT_RE.test(text)) return 'limit';
  return null;
}
