import { loadConfig, profileHomeEnv } from './config.ts';
import { aimsCommand } from './ops.ts';
import { preferredProfile } from './run.ts';
import { TOOL_IDS, getTool } from './tools.ts';
import type { ToolId } from './types.ts';

export const SHELLS = ['bash', 'zsh', 'fish', 'powershell'] as const;
export type Shell = (typeof SHELLS)[number];

export function isShell(s: unknown): s is Shell {
  return typeof s === 'string' && (SHELLS as readonly string[]).includes(s);
}

export function detectShell(): Shell {
  if (process.platform === 'win32' && !process.env.SHELL) return 'powershell';
  const sh = (process.env.SHELL ?? '').split(/[\\/]/).pop();
  return isShell(sh) ? sh : 'bash';
}

function q(shell: Shell, v: string): string {
  if (shell === 'powershell') return `'${v.replace(/'/g, "''")}'`;
  if (shell === 'fish') return `'${v.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;
  return `'${v.replace(/'/g, `'\\''`)}'`;
}

function setVar(shell: Shell, k: string, v: string): string {
  if (shell === 'powershell') return `$env:${k} = ${q(shell, v)}`;
  if (shell === 'fish') return `set -gx ${k} ${q(shell, v)}`;
  return `export ${k}=${q(shell, v)}`;
}

function unsetVar(shell: Shell, k: string): string {
  if (shell === 'powershell') return `Remove-Item Env:${k} -ErrorAction SilentlyContinue`;
  if (shell === 'fish') return `set -e ${k}`;
  return `unset ${k}`;
}

/** Wrapper functions so plain `claude` / `codex` go through aims. */
export function shellInit(shell: Shell): string {
  const cmd = aimsCommand();
  const onPath = cmd.length === 1 && cmd[0] === 'aims';
  const lines = [`# aims shell integration (${shell}): \`claude\` and \`codex\` now use the active aims profile`];
  for (const tool of TOOL_IDS) {
    if (shell === 'powershell') {
      const inv = onPath ? 'aims' : `& ${cmd.map((p) => q(shell, p)).join(' ')}`;
      lines.push(`function ${tool} { ${inv} ${tool} @args }`);
    } else if (shell === 'fish') {
      const inv = onPath ? 'command aims' : cmd.map((p) => q(shell, p)).join(' ');
      lines.push(`function ${tool} --wraps ${tool}; ${inv} ${tool} $argv; end`);
    } else {
      const inv = onPath ? 'command aims' : cmd.map((p) => q(shell, p)).join(' ');
      lines.push(`${tool}() { ${inv} ${tool} "$@"; }`);
    }
  }
  return `${lines.join('\n')}\n`;
}

/**
 * Lines that pin this terminal to profiles. Also sets CLAUDE_CONFIG_DIR /
 * CODEX_HOME, so even the bare binaries use the right login.
 */
export function envLines(shell: Shell, o: { tool?: ToolId | null; profile?: string | null; reset?: boolean } = {}): string {
  const cfg = loadConfig();
  const ids = o.tool ? [o.tool] : TOOL_IDS.filter((id) => !o.profile || cfg.tools[id].profiles[o.profile]);
  if (!ids.length) throw new Error(`no profile named "${o.profile}"`);
  const lines: string[] = [];
  for (const id of ids) {
    const t = getTool(id);
    const pinVar = `AIMS_${id.toUpperCase()}_PROFILE`;
    if (o.reset) {
      lines.push(unsetVar(shell, pinVar));
      const hubEnv = cfg.tools[id].hubEnv;
      lines.push(hubEnv ? setVar(shell, t.homeEnv, hubEnv) : unsetVar(shell, t.homeEnv));
      continue;
    }
    const name = o.profile ? (cfg.tools[id].profiles[o.profile] ? o.profile : null) : preferredProfile(cfg, id);
    if (!name) {
      if (o.tool) throw new Error(`no ${id} profile "${o.profile ?? ''}"`);
      continue;
    }
    lines.push(setVar(shell, pinVar, name));
    const homeEnv = profileHomeEnv(cfg, id, name);
    lines.push(homeEnv ? setVar(shell, t.homeEnv, homeEnv) : unsetVar(shell, t.homeEnv));
  }
  return `${lines.join('\n')}\n`;
}
