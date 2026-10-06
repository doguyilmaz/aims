import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
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
} from './config.js';
import { buildEnv, clearProfile, evaluate, markLimited, pickProfile, usageWindows } from './health.js';
import { removeLinks } from './links.js';
import { pinnedProfile, preferredProfile, syncProfile } from './run.js';
import { TOOLS, TOOL_IDS, binFor, getTool } from './tools.js';
import { home, readJson, toDate, which, writeJsonAtomic } from './util.js';

export const PACKAGE_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,40}$/;

function hubClaudeJson(cfg) {
  const env = cfg.tools.claude.hubEnv;
  return env ? path.join(env, '.claude.json') : path.join(home(), '.claude.json');
}

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
 * A new Claude profile gets the non-account parts of ~/.claude.json:
 * user MCP servers, per-project trust and tool permissions, theme.
 * Account fields (oauthAccount, userID, caches) are never copied.
 */
function seedClaudeJson(cfg, dir) {
  const target = path.join(dir, '.claude.json');
  if (fs.existsSync(target)) return false;
  const src = readJson(hubClaudeJson(cfg), null);
  if (!src) return false;
  const out = {};
  for (const k of CLAUDE_JSON_KEYS) if (src[k] !== undefined) out[k] = src[k];
  if (src.projects && typeof src.projects === 'object') {
    out.projects = {};
    for (const [p, v] of Object.entries(src.projects)) {
      const keep = {};
      for (const k of CLAUDE_PROJECT_KEYS) if (v && v[k] !== undefined) keep[k] = v[k];
      if (Object.keys(keep).length) out.projects[p] = keep;
    }
  }
  writeJsonAtomic(target, out, 0o600);
  return true;
}

/** Copy user-scope MCP servers that exist in ~/.claude.json but not yet in a profile. */
export function syncClaudeMcp(cfg) {
  const src = readJson(hubClaudeJson(cfg), {}) || {};
  const servers = src.mcpServers || {};
  const changed = [];
  for (const name of Object.keys(cfg.tools.claude.profiles)) {
    if (cfg.tools.claude.profiles[name].existing) continue;
    const file = path.join(profileDir(cfg, 'claude', name), '.claude.json');
    const data = readJson(file, null);
    if (!data) continue;
    const missing = Object.keys(servers).filter((k) => !(data.mcpServers || {})[k]);
    if (!missing.length) continue;
    data.mcpServers = { ...(data.mcpServers || {}) };
    for (const k of missing) data.mcpServers[k] = servers[k];
    writeJsonAtomic(file, data, 0o600);
    changed.push(`${name}: ${missing.join(', ')}`);
  }
  return changed;
}

export function addProfile(id, name, { existing = false, isolated = false, dir } = {}) {
  getTool(id);
  if (!NAME_RE.test(name)) throw new Error(`invalid profile name "${name}" (letters, digits, . _ -)`);
  const cfg = loadConfig();
  ensureHub(cfg, id);
  const t = cfg.tools[id];
  if (t.profiles[name]) throw new Error(`${id} profile "${name}" already exists`);
  const profile = { createdAt: new Date().toISOString() };
  let report = [];
  if (existing) {
    const other = Object.keys(t.profiles).find((n) => t.profiles[n].existing);
    if (other) throw new Error(`"${other}" already uses the existing ${id} login`);
    profile.existing = true;
  } else {
    if (isolated) profile.isolated = true;
    if (dir) profile.dir = path.resolve(dir);
  }
  t.profiles[name] = profile;
  t.order.push(name);
  if (!t.active) t.active = name;
  const pdir = profileDir(cfg, id, name);
  if (!existing) {
    fs.mkdirSync(pdir, { recursive: true, mode: 0o700 });
    if (id === 'claude') seedClaudeJson(cfg, pdir);
    report = syncProfile(cfg, id, name);
  }
  saveConfig(cfg);
  return { dir: pdir, report, active: t.active === name };
}

export function removeProfile(id, name, { purge = false } = {}) {
  const cfg = loadConfig();
  const t = cfg.tools[id];
  if (!t.profiles[name]) throw new Error(`no ${id} profile "${name}"`);
  const p = t.profiles[name];
  const dir = profileDir(cfg, id, name);
  delete t.profiles[name];
  t.order = t.order.filter((n) => n !== name);
  if (t.active === name) t.active = t.order[0] || null;
  saveConfig(cfg);
  removeProfileState(id, name);
  let purged = false;
  if (purge && !p.existing && fs.existsSync(dir)) {
    const root = path.join(aimsHome(), 'profiles');
    if (!p.dir && !path.resolve(dir).startsWith(root + path.sep)) throw new Error(`refusing to delete ${dir}`);
    removeLinks(dir); // never follow links into the shared hub
    fs.rmSync(dir, { recursive: true, force: true });
    purged = true;
  }
  return { dir, purged, active: t.active };
}

/** Set the active profile. Without a tool, every tool that has a profile with this name. */
export function useProfile(id, name) {
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

export function setOrder(id, names) {
  const cfg = loadConfig();
  const t = cfg.tools[id];
  for (const n of names) if (!t.profiles[n]) throw new Error(`no ${id} profile "${n}"`);
  t.order = [...names, ...t.order.filter((n) => !names.includes(n))];
  saveConfig(cfg);
  return t.order;
}

/** Mark the current profile as limited and make the next usable one active. */
export function failover(id, { from, to, minutes, reason = 'limit' } = {}) {
  const cfg = loadConfig();
  const t = cfg.tools[id];
  const current = from || preferredProfile(cfg, id);
  if (!current) throw new Error(`no ${id} profiles configured`);
  markLimited(cfg, id, current, { minutes, reason });
  let next = to;
  if (next) {
    if (!t.profiles[next]) throw new Error(`no ${id} profile "${next}"`);
  } else {
    const pick = pickProfile(cfg, loadState(), id, null, { exclude: [current] });
    next = pick.name;
    if (!next) {
      const why = pick.skipped.map((s) => `${s.name}: ${s.reasons.join(', ')}`).join('; ');
      throw new Error(`no other usable ${id} profile${why ? ` (${why})` : ''}`);
    }
  }
  t.active = next;
  saveConfig(cfg);
  const st = getProfileState(loadState(), id, current);
  return { from: current, to: next, until: st.until, pinned: pinnedProfile(id) };
}

export function clear(id, name) {
  const cfg = loadConfig();
  const names = name ? [name] : Object.keys(cfg.tools[id].profiles);
  for (const n of names) clearProfile(id, n);
  return names;
}

export function statusReport({ ids = TOOL_IDS } = {}) {
  const cfg = loadConfig();
  const state = loadState();
  return ids.map((id) => {
    const t = cfg.tools[id];
    const pinned = pinnedProfile(id);
    const profiles = t.order.map((name) => {
      const ev = evaluate(cfg, state, id, name);
      const st = ev.state;
      const acct = st.account || {};
      return {
        name,
        active: t.active === name,
        pinned: pinned === name,
        existing: Boolean(t.profiles[name].existing),
        isolated: Boolean(t.profiles[name].isolated),
        dir: profileDir(cfg, id, name),
        email: ev.cred.email || acct.email || null,
        plan: ev.cred.plan || acct.plan || null,
        loggedIn: ev.cred.present,
        usable: ev.usable,
        reasons: ev.reasons,
        usage: usageWindows(st),
        usageAt: st.usage ? st.usage.at : null,
        until: st.until && toDate(st.until) > new Date() ? st.until : null,
      };
    });
    return { tool: id, title: TOOLS[id].title, hub: t.hub, active: t.active, pinned, profiles };
  });
}

// ---------------------------------------------------------------------------
// setup: skill, MCP server registration, Claude status line

/** How tools should invoke aims: the PATH command if available, else node + this script. */
export function aimsCommand() {
  if (which('aims')) return ['aims'];
  return [process.execPath, path.join(PACKAGE_ROOT, 'bin', 'aims.js')];
}

function shellJoin(parts) {
  return parts.map((p) => (/[\s"'$`\\]/.test(p) ? `"${p.replace(/(["\\$`])/g, '\\$1')}"` : p)).join(' ');
}

/** Homes to install into: the hub plus any isolated profile (others share the hub's files). */
function installHomes(cfg, id) {
  const homes = [{ label: 'hub', dir: ensureHub(cfg, id), env: cfg.tools[id].hubEnv || null }];
  for (const [name, p] of Object.entries(cfg.tools[id].profiles)) {
    if (p.isolated) homes.push({ label: name, dir: profileDir(cfg, id, name), env: profileHomeEnv(cfg, id, name) });
  }
  return homes;
}

function installSkill(cfg, id) {
  const src = path.join(PACKAGE_ROOT, 'skills', 'aims');
  return installHomes(cfg, id).map((h) => {
    const dst = path.join(h.dir, 'skills', 'aims');
    fs.mkdirSync(dst, { recursive: true });
    fs.cpSync(src, dst, { recursive: true });
    return `${id} skill -> ${dst}`;
  });
}

function registerMcp(cfg, id) {
  const tool = getTool(id);
  const bin = which(binFor(tool));
  if (!bin) return [`${id}: not installed, skipped MCP registration`];
  const cmd = [...aimsCommand(), 'mcp'];
  const targets =
    id === 'claude'
      ? // user-scope MCP servers live in each login's own .claude.json
        [
          ...(Object.values(cfg.tools.claude.profiles).some((p) => p.existing)
            ? []
            : [{ label: 'default login', env: cfg.tools.claude.hubEnv || null }]),
          ...Object.keys(cfg.tools.claude.profiles).map((n) => ({
            label: n,
            env: profileHomeEnv(cfg, 'claude', n),
          })),
        ]
      : installHomes(cfg, id).map((h) => ({ label: h.label, env: h.env }));
  return targets.map(({ label, env: homeEnv }) => {
    const env = { ...process.env };
    if (homeEnv) env[tool.homeEnv] = homeEnv;
    else delete env[tool.homeEnv];
    const args =
      id === 'claude' ? ['mcp', 'add', '--scope', 'user', 'aims', '--', ...cmd] : ['mcp', 'add', 'aims', '--', ...cmd];
    const r = spawnSync(bin, args, { env, encoding: 'utf8', timeout: 60000, shell: process.platform === 'win32' && /\.(cmd|bat)$/i.test(bin) });
    const out = `${r.stdout || ''}${r.stderr || ''}`.trim();
    if (r.status === 0) return `${id} MCP server registered (${label})`;
    if (/already exists/i.test(out)) return `${id} MCP server already registered (${label})`;
    return `${id} MCP registration failed (${label}): ${out.split('\n').pop() || `exit ${r.status}`}`;
  });
}

function installStatusline(cfg) {
  const cmd = `${shellJoin(aimsCommand())} statusline`;
  const msgs = [];
  for (const h of installHomes(cfg, 'claude')) {
    const file = path.join(h.dir, 'settings.json');
    let settings = {};
    if (fs.existsSync(file)) {
      settings = readJson(file, null);
      if (!settings) {
        msgs.push(`claude status line: ${file} is not valid JSON, left untouched`);
        continue;
      }
    }
    const existingCmd = settings.statusLine && settings.statusLine.command;
    if (existingCmd && !/\baims\b.*\bstatusline\b/.test(existingCmd)) {
      cfg.statusline.chain = existingCmd;
      msgs.push(`claude status line: your previous command is kept and shown first (${existingCmd})`);
    }
    settings.statusLine = { type: 'command', command: cmd, padding: 0 };
    writeJsonAtomic(file, settings);
    msgs.push(`claude status line -> ${file}`);
  }
  saveConfig(cfg);
  return msgs;
}

export function setup({ ids = TOOL_IDS, skill = true, mcp = true, statusline = false } = {}) {
  const cfg = loadConfig();
  for (const id of ids) ensureHub(cfg, id);
  saveConfig(cfg);
  const msgs = [];
  for (const id of ids) {
    if (skill) msgs.push(...installSkill(cfg, id));
    if (mcp) msgs.push(...registerMcp(cfg, id));
  }
  if (statusline && ids.includes('claude')) msgs.push(...installStatusline(cfg));
  return msgs;
}

/** Commands to log a profile in (interactive; needs a terminal and a browser). */
export function loginCommand(id, name) {
  return `aims login ${id} ${name}`;
}

export { buildEnv };
