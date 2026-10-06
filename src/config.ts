import path from 'node:path';
import { TOOLS, TOOL_IDS, getTool, isToolId } from './tools.ts';
import type { Config, FailoverConfig, ProfileState, State, ToolConfig, ToolId } from './types.ts';
import { expandHome, home, readJson, readJsonStrict, writeJsonAtomic } from './util.ts';

/**
 * ~/.aims/config.json  -- profiles and preferences (safe to hand-edit)
 * ~/.aims/state.json   -- machine-written health data (cooldowns, usage)
 * ~/.aims/profiles/<tool>/<name>/  -- one CLAUDE_CONFIG_DIR / CODEX_HOME per account
 */
export function aimsHome(): string {
  return path.resolve(expandHome(process.env.AIMS_HOME || path.join(home(), '.aims')));
}

export const configPath = () => path.join(aimsHome(), 'config.json');
export const statePath = () => path.join(aimsHome(), 'state.json');

export const DEFAULT_FAILOVER: FailoverConfig = {
  auto: true, // pick another profile when the chosen one is limited / logged out
  cooldownMinutes: 300, // how long a limited profile is skipped when no reset time is known
  threshold: 95, // usage % (Claude status line / Codex live check) treated as "limited"
};

type RawConfig = Partial<Omit<Config, 'tools'>> & { tools?: Partial<Record<ToolId, Partial<ToolConfig>>> };

export function loadConfig(): Config {
  // A hand-edit typo must not silently reset every profile: refuse to go on.
  const raw = readJsonStrict<RawConfig>(configPath()) ?? {};
  const tools = {} as Record<ToolId, ToolConfig>;
  for (const id of TOOL_IDS) {
    const r = raw.tools?.[id] ?? {};
    const profiles = { ...(r.profiles ?? {}) };
    const order = (r.order ?? []).filter((n) => profiles[n]);
    for (const n of Object.keys(profiles)) if (!order.includes(n)) order.push(n);
    tools[id] = {
      hub: r.hub ?? null,
      hubEnv: r.hubEnv ?? null,
      active: r.active && profiles[r.active] ? r.active : null,
      order,
      profiles,
    };
  }
  return {
    version: 1,
    failover: { ...DEFAULT_FAILOVER, ...(raw.failover ?? {}) },
    statusline: { ...(raw.statusline ?? {}) },
    tools,
  };
}

export function saveConfig(cfg: Config): void {
  writeJsonAtomic(configPath(), cfg, 0o600);
}

/**
 * The "hub" is the tool's normal home (~/.claude, ~/.codex). Shared entries of
 * every profile point into it, so plain `claude` / `codex` keep seeing the same
 * history and settings. Captured once, so a shell that later exports
 * CLAUDE_CONFIG_DIR for a profile doesn't move it.
 */
export function ensureHub(cfg: Config, id: ToolId): string {
  const t = cfg.tools[id];
  if (t.hub) return t.hub;
  const tool = getTool(id);
  const fromEnv = process.env[tool.homeEnv];
  const profilesRoot = path.join(aimsHome(), 'profiles');
  if (fromEnv && !path.resolve(expandHome(fromEnv)).startsWith(profilesRoot)) {
    t.hub = path.resolve(expandHome(fromEnv));
    t.hubEnv = fromEnv; // kept verbatim: Claude hashes it into the keychain entry name
  } else {
    t.hub = tool.defaultHub();
    t.hubEnv = null;
  }
  return t.hub;
}

export function profileDir(cfg: Config, id: ToolId, name: string): string {
  const p = cfg.tools[id].profiles[name];
  if (!p) throw new Error(`no ${id} profile "${name}"`);
  if (p.existing) return ensureHub(cfg, id);
  return p.dir ? path.resolve(expandHome(p.dir)) : path.join(aimsHome(), 'profiles', id, name);
}

/**
 * Value for CLAUDE_CONFIG_DIR / CODEX_HOME, or null to unset it.
 * The profile that adopted the existing login must run with the variable
 * *unset* (or exactly as the user had it): Claude derives the keychain entry
 * and the .claude.json location from it, so even ~/.claude spelled out would
 * point at a different, empty login.
 */
export function profileHomeEnv(cfg: Config, id: ToolId, name: string): string | null {
  const p = cfg.tools[id].profiles[name];
  if (!p) throw new Error(`no ${id} profile "${name}"`);
  if (p.existing) return cfg.tools[id].hubEnv;
  return profileDir(cfg, id, name);
}

/** Which profile owns a given CLAUDE_CONFIG_DIR / CODEX_HOME value. */
export function profileForHomeEnv(cfg: Config, id: ToolId, value: string | undefined): string | null {
  const t = cfg.tools[id];
  const target = value ? path.resolve(expandHome(value)) : null;
  for (const name of Object.keys(t.profiles)) {
    const env = profileHomeEnv(cfg, id, name);
    if ((env ? path.resolve(expandHome(env)) : null) === target) return name;
  }
  if (target && t.hub && target === path.resolve(t.hub)) {
    return Object.keys(t.profiles).find((n) => t.profiles[n]!.existing) ?? null;
  }
  return null;
}

export function resolveToolArg(arg: string | undefined): ToolId[] {
  if (!arg || arg === 'all') return TOOL_IDS;
  if (!isToolId(arg)) throw new Error(`unknown tool "${arg}" (expected: ${Object.keys(TOOLS).join(', ')}, all)`);
  return [arg];
}

// ---------------------------------------------------------------------------
// state.json: { claude: { work: { until, reason, needsLogin, usage, account } } }

export function loadState(): State {
  return readJson<State | null>(statePath(), null) ?? {};
}

export function getProfileState(state: State, id: ToolId, name: string): ProfileState {
  return state[id]?.[name] ?? {};
}

/** Read-modify-write one profile entry (re-reads so parallel writers rarely collide). */
export function updateProfileState(
  id: ToolId,
  name: string,
  fn: (s: ProfileState) => ProfileState,
): ProfileState {
  const state = loadState();
  const forTool = (state[id] ??= {});
  const next = fn({ ...(forTool[name] ?? {}) });
  forTool[name] = next;
  writeJsonAtomic(statePath(), state, 0o600);
  return next;
}

export function removeProfileState(id: ToolId, name: string): void {
  const state = loadState();
  if (state[id]?.[name]) {
    delete state[id][name];
    writeJsonAtomic(statePath(), state, 0o600);
  }
}
