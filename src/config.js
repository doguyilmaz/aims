import path from 'node:path';
import { TOOLS, TOOL_IDS, getTool } from './tools.js';
import { expandHome, home, readJson, writeJsonAtomic } from './util.js';

/**
 * ~/.aims/config.json  -- profiles and preferences (safe to hand-edit)
 * ~/.aims/state.json   -- machine-written health data (cooldowns, usage)
 * ~/.aims/profiles/<tool>/<name>/  -- one CLAUDE_CONFIG_DIR / CODEX_HOME per account
 */
export function aimsHome() {
  return path.resolve(expandHome(process.env.AIMS_HOME || path.join(home(), '.aims')));
}

export const configPath = () => path.join(aimsHome(), 'config.json');
export const statePath = () => path.join(aimsHome(), 'state.json');

export const DEFAULT_FAILOVER = {
  auto: true, // pick another profile when the chosen one is limited / logged out
  cooldownMinutes: 300, // how long a limited profile is skipped when no reset time is known
  threshold: 95, // usage % (from the Claude status line / Codex live check) treated as "limited"
};

function blankTool() {
  return { hub: null, hubEnv: null, active: null, order: [], profiles: {} };
}

export function loadConfig() {
  const raw = readJson(configPath(), {}) || {};
  const cfg = {
    version: 1,
    failover: { ...DEFAULT_FAILOVER, ...(raw.failover || {}) },
    statusline: { ...(raw.statusline || {}) },
    tools: {},
  };
  for (const id of TOOL_IDS) {
    const t = { ...blankTool(), ...((raw.tools || {})[id] || {}) };
    t.profiles = { ...(t.profiles || {}) };
    t.order = (t.order || []).filter((n) => t.profiles[n]);
    for (const n of Object.keys(t.profiles)) if (!t.order.includes(n)) t.order.push(n);
    if (t.active && !t.profiles[t.active]) t.active = null;
    cfg.tools[id] = t;
  }
  return cfg;
}

export function saveConfig(cfg) {
  writeJsonAtomic(configPath(), cfg, 0o600);
}

/**
 * The "hub" is the tool's normal home (~/.claude, ~/.codex). Shared entries of
 * every profile point into it, so plain `claude` / `codex` keep seeing the same
 * history and settings. Captured once so later shells that export
 * CLAUDE_CONFIG_DIR for a profile don't move it.
 */
export function ensureHub(cfg, id) {
  const tool = getTool(id);
  const t = cfg.tools[id];
  if (t.hub) return t.hub;
  const fromEnv = process.env[tool.homeEnv];
  const profilesRoot = path.join(aimsHome(), 'profiles');
  if (fromEnv && !path.resolve(fromEnv).startsWith(profilesRoot)) {
    t.hub = path.resolve(expandHome(fromEnv));
    t.hubEnv = fromEnv; // keep the exact string: Claude hashes it into the keychain entry name
  } else {
    t.hub = tool.defaultHub();
    t.hubEnv = null;
  }
  return t.hub;
}

export function profileDir(cfg, id, name) {
  const p = cfg.tools[id].profiles[name];
  if (!p) throw new Error(`no ${id} profile "${name}"`);
  if (p.existing) return ensureHub(cfg, id);
  return p.dir ? path.resolve(expandHome(p.dir)) : path.join(aimsHome(), 'profiles', id, name);
}

/**
 * Value for CLAUDE_CONFIG_DIR / CODEX_HOME, or null to unset it.
 * The profile that adopted the existing login must run with the variable
 * *unset* (or exactly as it was): Claude derives the keychain entry and the
 * .claude.json location from it, so even ~/.claude spelled out would point
 * at a different, empty login.
 */
export function profileHomeEnv(cfg, id, name) {
  const p = cfg.tools[id].profiles[name];
  if (p.existing) return cfg.tools[id].hubEnv || null;
  return profileDir(cfg, id, name);
}

/** Which profile owns a given CLAUDE_CONFIG_DIR / CODEX_HOME value. */
export function profileForHomeEnv(cfg, id, value) {
  const t = cfg.tools[id];
  const target = value ? path.resolve(expandHome(value)) : null;
  for (const name of Object.keys(t.profiles)) {
    const env = profileHomeEnv(cfg, id, name);
    if ((env ? path.resolve(env) : null) === target) return name;
  }
  if (target && t.hub && target === path.resolve(t.hub)) {
    return Object.keys(t.profiles).find((n) => t.profiles[n].existing) || null;
  }
  return null;
}

export function resolveToolArg(arg) {
  if (!arg || arg === 'all') return TOOL_IDS;
  if (!TOOLS[arg]) throw new Error(`unknown tool "${arg}" (expected: ${TOOL_IDS.join(', ')}, all)`);
  return [arg];
}

// ---------------------------------------------------------------------------
// state.json: { claude: { work: { until, reason, needsLogin, usage: {...}, account } } }

export function loadState() {
  return readJson(statePath(), {}) || {};
}

export function getProfileState(state, id, name) {
  return ((state || {})[id] || {})[name] || {};
}

/** Read-modify-write a single profile entry (re-reads so parallel writers rarely collide). */
export function updateProfileState(id, name, fn) {
  const state = loadState();
  state[id] = state[id] || {};
  const next = fn({ ...(state[id][name] || {}) });
  if (next === undefined) return;
  state[id][name] = next;
  writeJsonAtomic(statePath(), state, 0o600);
  return next;
}

export function removeProfileState(id, name) {
  const state = loadState();
  if (state[id] && state[id][name]) {
    delete state[id][name];
    writeJsonAtomic(statePath(), state, 0o600);
  }
}
