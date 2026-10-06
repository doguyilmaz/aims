import { loadConfig, profileHomeEnv } from './config.js';
import { aimsCommand } from './ops.js';
import { preferredProfile } from './run.js';
import { TOOL_IDS, getTool } from './tools.js';

export const SHELLS = ['bash', 'zsh', 'fish', 'powershell'];

export function detectShell() {
  if (process.platform === 'win32' && !process.env.SHELL) return 'powershell';
  const sh = (process.env.SHELL || '').split(/[\\/]/).pop();
  return SHELLS.includes(sh) ? sh : 'bash';
}

function q(shell, v) {
  if (shell === 'powershell') return `'${v.replace(/'/g, "''")}'`;
  if (shell === 'fish') return `'${v.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;
  return `'${v.replace(/'/g, `'\\''`)}'`;
}

function setVar(shell, k, v) {
  if (shell === 'powershell') return `$env:${k} = ${q(shell, v)}`;
  if (shell === 'fish') return `set -gx ${k} ${q(shell, v)}`;
  return `export ${k}=${q(shell, v)}`;
}

function unsetVar(shell, k) {
  if (shell === 'powershell') return `Remove-Item Env:${k} -ErrorAction SilentlyContinue`;
  if (shell === 'fish') return `set -e ${k}`;
  return `unset ${k}`;
}

/** Wrapper functions so plain `claude` / `codex` go through aims. */
export function shellInit(shell) {
  const cmd = aimsCommand();
  const lines = [`# aims shell integration (${shell}): \`claude\` and \`codex\` now use the active aims profile`];
  for (const tool of TOOL_IDS) {
    if (shell === 'powershell') {
      const inv = cmd.length === 1 ? cmd[0] : `& ${cmd.map((p) => q(shell, p)).join(' ')}`;
      lines.push(`function ${tool} { ${inv} ${tool} @args }`);
    } else if (shell === 'fish') {
      lines.push(`function ${tool} --wraps ${tool}; ${cmd.map((p) => (p === 'aims' ? 'command aims' : q(shell, p))).join(' ')} ${tool} $argv; end`);
    } else {
      const inv = cmd.length === 1 ? 'command aims' : cmd.map((p) => q(shell, p)).join(' ');
      lines.push(`${tool}() { ${inv} ${tool} "$@"; }`);
    }
  }
  return lines.join('\n') + '\n';
}

/**
 * Lines that pin this terminal to profiles: also sets CLAUDE_CONFIG_DIR /
 * CODEX_HOME so even the bare binaries use the right login.
 */
export function envLines(shell, { tool, profile, reset = false } = {}) {
  const cfg = loadConfig();
  const ids = tool ? [tool] : TOOL_IDS.filter((id) => !profile || cfg.tools[id].profiles[profile]);
  if (!ids.length) throw new Error(`no profile named "${profile}"`);
  const lines = [];
  for (const id of ids) {
    const t = getTool(id);
    const pinVar = `AIMS_${id.toUpperCase()}_PROFILE`;
    if (reset) {
      lines.push(unsetVar(shell, pinVar));
      const hubEnv = cfg.tools[id].hubEnv;
      lines.push(hubEnv ? setVar(shell, t.homeEnv, hubEnv) : unsetVar(shell, t.homeEnv));
      continue;
    }
    const name = profile ? (cfg.tools[id].profiles[profile] ? profile : null) : preferredProfile(cfg, id);
    if (!name) {
      if (tool) throw new Error(`no ${id} profile "${profile}"`);
      continue;
    }
    lines.push(setVar(shell, pinVar, name));
    const home = profileHomeEnv(cfg, id, name);
    lines.push(home ? setVar(shell, t.homeEnv, home) : unsetVar(shell, t.homeEnv));
  }
  return lines.join('\n') + '\n';
}
