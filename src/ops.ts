import fs from 'node:fs';
import path from 'node:path';
import pkg from '../package.json' with { type: 'json' };
import skillText from '../skills/aims/SKILL.md' with { type: 'text' };
import {
  aimsHome,
  ensureHub,
  getProfileState,
  loadConfig,
  loadState,
  profileDir,
  profileHomeEnv,
  removeProfileState,
  saveConfig,
} from './config.ts';
import { clearProfile, evaluate, markLimited, pickProfile, usageWindows } from './health.ts';
import { removeLinks } from './links.ts';
import { runSync } from './proc.ts';
import { pinnedProfile, preferredProfile, syncProfile } from './run.ts';
import { TOOLS, TOOL_IDS, binFor, getTool } from './tools.ts';
import type { Config, LinkReport, ProfileConfig, ToolId, UsageWindow } from './types.ts';
import { expandHome, home, isWithin, readJson, toDate, which, writeJsonAtomic } from './util.ts';

export const VERSION: string = pkg.version;

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,40}$/;

function hubClaudeJson(cfg: Config): string {
  const env = cfg.tools.claude.hubEnv;
  return env ? path.join(env, '.claude.json') : path.join(home(), '.claude.json');
}

type ClaudeJson = Record<string, unknown> & {
  mcpServers?: Record<string, unknown>;
  projects?: Record<string, Record<string, unknown>>;
};

const CLAUDE_JSON_KEYS = ['theme', 'editorMode', 'mcpServers', 'autoUpdates', 'verbose', 'preferredNotifChannel'];
const CLAUDE_PROJECT_KEYS = [
  'allowedTools',
  'mcpServers',
  'enabledMcpjsonServers',
  'disabledMcpjsonServers',
  'hasTrustDialogAccepted',
  'hasCompletedProjectOnboarding',
];

/**
 * A new Claude profile starts with the non-account parts of ~/.claude.json:
 * user MCP servers, per-project trust and tool permissions, theme.
 * Account fields (oauthAccount, userID, caches) are never copied.
 */
function seedClaudeJson(cfg: Config, dir: string): boolean {
  const target = path.join(dir, '.claude.json');
  if (fs.existsSync(target)) return false;
  const src = readJson<ClaudeJson | null>(hubClaudeJson(cfg), null);
  if (!src) return false;
  const out: ClaudeJson = {};
  for (const k of CLAUDE_JSON_KEYS) if (src[k] !== undefined) out[k] = src[k];
  if (src.projects && typeof src.projects === 'object') {
    out.projects = {};
    for (const [p, v] of Object.entries(src.projects)) {
      const keep: Record<string, unknown> = {};
      for (const k of CLAUDE_PROJECT_KEYS) if (v?.[k] !== undefined) keep[k] = v[k];
      if (Object.keys(keep).length) out.projects[p] = keep;
    }
  }
  writeJsonAtomic(target, out, 0o600);
  return true;
}

/** Copy user-scope MCP servers that exist in ~/.claude.json but not yet in a profile. */
export function syncClaudeMcp(cfg: Config): string[] {
  const servers = readJson<ClaudeJson>(hubClaudeJson(cfg), {}).mcpServers ?? {};
  const changed: string[] = [];
  for (const [name, p] of Object.entries(cfg.tools.claude.profiles)) {
    if (p.existing) continue;
    const file = path.join(profileDir(cfg, 'claude', name), '.claude.json');
    const data = readJson<ClaudeJson | null>(file, null);
    if (!data) continue;
    const missing = Object.keys(servers).filter((k) => !data.mcpServers?.[k]);
    if (!missing.length) continue;
    data.mcpServers = { ...(data.mcpServers ?? {}) };
    for (const k of missing) data.mcpServers[k] = servers[k];
    writeJsonAtomic(file, data, 0o600);
    changed.push(`${name}: ${missing.join(', ')}`);
  }
  return changed;
}

/**
 * A custom profile dir must not overlap anything aims links into or might
 * delete: the home dir, a hub, ~/.aims, or another profile.
 */
function checkCustomDir(cfg: Config, dir: string): string {
  const protectedDirs = [home(), aimsHome()];
  for (const id of TOOL_IDS) {
    protectedDirs.push(TOOLS[id].defaultHub());
    if (cfg.tools[id].hub) protectedDirs.push(cfg.tools[id].hub!);
  }
  for (const p of protectedDirs) {
    if (isWithin(p, dir)) throw new Error(`--dir ${dir} contains ${p}; pick an empty, dedicated directory`);
    if (p !== home() && isWithin(dir, p)) throw new Error(`--dir ${dir} is inside ${p}; pick a directory outside it`);
  }
  for (const id of TOOL_IDS) {
    for (const n of Object.keys(cfg.tools[id].profiles)) {
      const other = profileDir(cfg, id, n);
      if (isWithin(dir, other) || isWithin(other, dir)) throw new Error(`--dir ${dir} overlaps ${id} profile "${n}" (${other})`);
    }
  }
  return dir;
}

export function addProfile(
  id: ToolId,
  name: string,
  o: { existing?: boolean; isolated?: boolean; dir?: string } = {},
): { dir: string; report: LinkReport[]; active: boolean } {
  if (!NAME_RE.test(name)) throw new Error(`invalid profile name "${name}" (letters, digits, . _ -)`);
  const cfg = loadConfig();
  ensureHub(cfg, id);
  const t = cfg.tools[id];
  if (t.profiles[name]) throw new Error(`${id} profile "${name}" already exists`);
  const profile: ProfileConfig = { createdAt: new Date().toISOString() };
  if (o.existing) {
    const other = Object.keys(t.profiles).find((n) => t.profiles[n]!.existing);
    if (other) throw new Error(`"${other}" already uses the existing ${id} login`);
    profile.existing = true;
  } else {
    if (o.isolated) profile.isolated = true;
    if (o.dir) profile.dir = checkCustomDir(cfg, path.resolve(expandHome(o.dir)));
  }
  t.profiles[name] = profile;
  t.order.push(name);
  t.active ??= name;
  const dir = profileDir(cfg, id, name);
  let report: LinkReport[] = [];
  if (!o.existing) {
    fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
    if (id === 'claude') seedClaudeJson(cfg, dir);
    report = syncProfile(cfg, id, name);
  }
  saveConfig(cfg);
  return { dir, report, active: t.active === name };
}

export function removeProfile(
  id: ToolId,
  name: string,
  o: { purge?: boolean; force?: boolean } = {},
): { dir: string; purged: boolean; report: LinkReport[] } {
  const cfg = loadConfig();
  const t = cfg.tools[id];
  const p = t.profiles[name];
  if (!p) throw new Error(`no ${id} profile "${name}"`);
  const dir = profileDir(cfg, id, name);
  let report: LinkReport[] = [];
  const purge = Boolean(o.purge) && !p.existing && fs.existsSync(dir);
  if (purge) {
    // Only ever delete what aims created itself.
    if (p.dir || !isWithin(dir, path.join(aimsHome(), 'profiles')) || dir === path.join(aimsHome(), 'profiles')) {
      throw new Error(`${dir} is a custom directory; remove the profile without --purge and delete it yourself`);
    }
    // Move anything the tool wrote into the profile (e.g. a link replaced by a real
    // file) into the shared folder first, so purging loses nothing shared.
    if (!p.isolated) {
      report = syncProfile(cfg, id, name, { fix: true });
      const stuck = report.filter((r) => r.action === 'conflict');
      if (stuck.length && !o.force) {
        throw new Error(
          `not purging ${dir}: ${stuck.map((r) => `"${r.name}" (${r.detail})`).join(', ')}. ` +
            'Resolve it, or add --force to delete anyway.',
        );
      }
    }
  }
  delete t.profiles[name];
  t.order = t.order.filter((n) => n !== name);
  if (t.active === name) t.active = t.order[0] ?? null;
  saveConfig(cfg);
  removeProfileState(id, name);
  if (purge) {
    removeLinks(dir); // never follow links into the shared hub
    fs.rmSync(dir, { recursive: true, force: true });
  }
  return { dir, purged: purge, report };
}

/** Set the active profile. Without a tool: every tool that has a profile with this name. */
export function useProfile(id: ToolId | null, name: string): ToolId[] {
  const cfg = loadConfig();
  const ids = id ? [id] : TOOL_IDS.filter((x) => cfg.tools[x].profiles[name]);
  if (!ids.length) throw new Error(`no profile named "${name}" (see: aims ls)`);
  for (const x of ids) {
    if (!cfg.tools[x].profiles[name]) throw new Error(`no ${x} profile "${name}"`);
    cfg.tools[x].active = name;
  }
  saveConfig(cfg);
  return ids;
}

export function setOrder(id: ToolId, names: string[]): string[] {
  const cfg = loadConfig();
  const t = cfg.tools[id];
  for (const n of names) if (!t.profiles[n]) throw new Error(`no ${id} profile "${n}"`);
  t.order = [...names, ...t.order.filter((n) => !names.includes(n))];
  saveConfig(cfg);
  return t.order;
}

/** The profile aims launched most recently (auto-failover may have picked a non-active one). */
export function lastUsedProfile(cfg: Config, id: ToolId): string | null {
  const state = loadState();
  let best: string | null = null;
  let bestAt = 0;
  for (const n of Object.keys(cfg.tools[id].profiles)) {
    const at = Date.parse(getProfileState(state, id, n).lastUsedAt ?? '');
    if (at > bestAt) {
      best = n;
      bestAt = at;
    }
  }
  return best;
}

/** Mark the current profile as limited and make the next usable one active. */
export function failover(
  id: ToolId,
  o: { from?: string; to?: string; minutes?: number; reason?: string } = {},
): { from: string; to: string; until: string | undefined; pinned: string | null } {
  const cfg = loadConfig();
  const t = cfg.tools[id];
  const current = o.from ?? lastUsedProfile(cfg, id) ?? preferredProfile(cfg, id);
  if (!current || !t.profiles[current]) throw new Error(`no ${id} profile ${current ? `"${current}"` : 'configured'}`);
  let next = o.to;
  if (next) {
    if (!t.profiles[next]) throw new Error(`no ${id} profile "${next}"`);
  } else {
    // Pick the target first: nothing changes when there is none.
    const pick = pickProfile(cfg, loadState(), id, null, { exclude: [current] });
    if (!pick.name) {
      const why = pick.skipped.map((s) => `${s.name}: ${s.reasons.join(', ')}`).join('; ');
      throw new Error(`no other usable ${id} profile${why ? ` (${why})` : ''}`);
    }
    next = pick.name;
  }
  markLimited(cfg, id, current, { minutes: o.minutes, reason: o.reason ?? 'limit' });
  t.active = next;
  saveConfig(cfg);
  return { from: current, to: next, until: getProfileState(loadState(), id, current).until, pinned: pinnedProfile(id) };
}

export function clear(id: ToolId, name?: string): string[] {
  const cfg = loadConfig();
  if (name && !cfg.tools[id].profiles[name]) throw new Error(`no ${id} profile "${name}"`);
  const names = name ? [name] : Object.keys(cfg.tools[id].profiles);
  for (const n of names) clearProfile(id, n);
  return names;
}

export interface ProfileStatus {
  name: string;
  active: boolean;
  pinned: boolean;
  existing: boolean;
  isolated: boolean;
  dir: string;
  email: string | null;
  plan: string | null;
  loggedIn: boolean | null;
  method: string | null;
  usable: boolean;
  reasons: string[];
  usage: UsageWindow[];
  usageAt: string | null;
  until: string | null;
}

export interface ToolStatus {
  tool: ToolId;
  title: string;
  hub: string | null;
  active: string | null;
  pinned: string | null;
  profiles: ProfileStatus[];
}

export function statusReport(o: { ids?: ToolId[] } = {}): ToolStatus[] {
  const cfg = loadConfig();
  const state = loadState();
  return (o.ids ?? TOOL_IDS).map((id) => {
    const t = cfg.tools[id];
    const pinned = pinnedProfile(id);
    const profiles = t.order.map((name): ProfileStatus => {
      const ev = evaluate(cfg, state, id, name);
      const st = ev.state;
      const p = t.profiles[name]!;
      const until = toDate(st.until);
      return {
        name,
        active: t.active === name,
        pinned: pinned === name,
        existing: Boolean(p.existing),
        isolated: Boolean(p.isolated),
        dir: profileDir(cfg, id, name),
        email: ev.cred.email ?? st.account?.email ?? null,
        plan: ev.cred.plan ?? st.account?.plan ?? null,
        loggedIn: ev.cred.present,
        method: ev.cred.method,
        usable: ev.usable,
        reasons: ev.reasons,
        usage: usageWindows(st),
        usageAt: st.usage?.at ?? null,
        until: until && until.getTime() > Date.now() ? until.toISOString() : null,
      };
    });
    return { tool: id, title: TOOLS[id].title, hub: t.hub, active: t.active, pinned, profiles };
  });
}

// ---------------------------------------------------------------------------
// setup: skill, MCP server registration, Claude status line

/** A `bun build --compile` binary carries its sources in a virtual filesystem. */
const COMPILED = import.meta.path.includes('$bunfs') || import.meta.path.includes('~BUN');

/** How the tools should invoke aims. */
export function aimsCommand(): string[] {
  if (COMPILED) return [process.execPath];
  if (which('aims')) return ['aims'];
  return [process.execPath, path.join(import.meta.dir, 'cli.ts')];
}

function shellJoin(parts: string[]): string {
  return parts.map((p) => (/[\s"'$`\\]/.test(p) ? `"${p.replace(/(["\\$`])/g, '\\$1')}"` : p)).join(' ');
}

interface InstallHome {
  label: string;
  dir: string;
  env: string | null;
}

/** Homes to install into: the hub plus isolated profiles (the others share the hub's files). */
function installHomes(cfg: Config, id: ToolId): InstallHome[] {
  const homes: InstallHome[] = [{ label: 'hub', dir: ensureHub(cfg, id), env: cfg.tools[id].hubEnv }];
  for (const [name, p] of Object.entries(cfg.tools[id].profiles)) {
    if (p.isolated) homes.push({ label: name, dir: profileDir(cfg, id, name), env: profileHomeEnv(cfg, id, name) });
  }
  return homes;
}

function installSkill(cfg: Config, id: ToolId): string[] {
  return installHomes(cfg, id).map((h) => {
    const dst = path.join(h.dir, 'skills', 'aims');
    fs.mkdirSync(dst, { recursive: true });
    fs.writeFileSync(path.join(dst, 'SKILL.md'), skillText);
    return `${id} skill -> ${dst}`;
  });
}

function registerMcp(cfg: Config, id: ToolId): string[] {
  const tool = getTool(id);
  const bin = which(binFor(tool));
  if (!bin) return [`${id}: not installed, skipped MCP registration`];
  const cmd = [...aimsCommand(), 'mcp'];
  // Claude keeps user-scope MCP servers in each login's own .claude.json.
  const targets: Array<{ label: string; env: string | null }> =
    id === 'claude'
      ? [
          ...(Object.values(cfg.tools.claude.profiles).some((p) => p.existing)
            ? []
            : [{ label: 'default login', env: cfg.tools.claude.hubEnv }]),
          ...Object.keys(cfg.tools.claude.profiles).map((n) => ({ label: n, env: profileHomeEnv(cfg, 'claude', n) })),
        ]
      : installHomes(cfg, id).map((h) => ({ label: h.label, env: h.env }));
  return targets.map(({ label, env: homeEnv }) => {
    const env = { ...process.env };
    if (homeEnv) env[tool.homeEnv] = homeEnv;
    else delete env[tool.homeEnv];
    const args =
      id === 'claude' ? ['mcp', 'add', '--scope', 'user', 'aims', '--', ...cmd] : ['mcp', 'add', 'aims', '--', ...cmd];
    const r = runSync(bin, args, { env, timeoutMs: 60000 });
    const out = `${r.stdout}${r.stderr}`.trim();
    if (r.code === 0) return `${id} MCP server registered (${label})`;
    if (/already exists/i.test(out)) return `${id} MCP server already registered (${label})`;
    return `${id} MCP registration failed (${label}): ${out.split('\n').pop() || `exit ${r.code}`}`;
  });
}

function installStatusline(cfg: Config): string[] {
  const cmd = `${shellJoin(aimsCommand())} statusline`;
  const msgs: string[] = [];
  for (const h of installHomes(cfg, 'claude')) {
    const file = path.join(h.dir, 'settings.json');
    let settings: { statusLine?: { command?: string } } & Record<string, unknown> = {};
    if (fs.existsSync(file)) {
      const parsed = readJson<typeof settings | null>(file, null);
      if (!parsed) {
        msgs.push(`claude status line: ${file} is not valid JSON, left untouched`);
        continue;
      }
      settings = parsed;
    }
    const existingCmd = settings.statusLine?.command;
    if (existingCmd && !/\baims\b.*\bstatusline\b/.test(existingCmd)) {
      cfg.statusline.chain = existingCmd;
      msgs.push(`claude status line: your previous command is kept and shown first (${existingCmd})`);
    }
    settings.statusLine = { type: 'command', command: cmd, padding: 0 } as typeof settings.statusLine;
    writeJsonAtomic(file, settings);
    msgs.push(`claude status line -> ${file}`);
  }
  saveConfig(cfg);
  return msgs;
}

export function setup(o: { ids?: ToolId[]; skill?: boolean; mcp?: boolean; statusline?: boolean } = {}): string[] {
  const { ids = TOOL_IDS, skill = true, mcp = true, statusline = false } = o;
  const cfg = loadConfig();
  for (const id of ids) ensureHub(cfg, id);
  saveConfig(cfg);
  const msgs: string[] = [];
  for (const id of ids) {
    if (skill) msgs.push(...installSkill(cfg, id));
    if (mcp) msgs.push(...registerMcp(cfg, id));
  }
  if (statusline && ids.includes('claude')) msgs.push(...installStatusline(cfg));
  return msgs;
}
