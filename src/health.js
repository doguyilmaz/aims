import { spawn, spawnSync } from 'node:child_process';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  getProfileState,
  profileDir,
  profileHomeEnv,
  updateProfileState,
} from './config.js';
import { binFor, classifyFailure, getTool } from './tools.js';
import { IS_MAC, fmtDuration, home, readJson, toDate, which } from './util.js';

// ---------------------------------------------------------------------------
// Child environment

/** Env for running `tool` as `name`: points the tool at the profile and drops overriding vars. */
export function buildEnv(cfg, id, name, base = process.env) {
  const tool = getTool(id);
  const profile = cfg.tools[id].profiles[name];
  const env = { ...base };
  const dropped = [];
  for (const k of tool.conflictingEnv) {
    if (env[k] && !(profile.env && k in profile.env)) {
      delete env[k];
      dropped.push(k);
    }
  }
  const homeEnv = profileHomeEnv(cfg, id, name);
  if (homeEnv) env[tool.homeEnv] = homeEnv;
  else delete env[tool.homeEnv];
  Object.assign(env, profile.env || {});
  env.AIMS_SESSION_PROFILE = `${id}:${name}`;
  return { env, dropped };
}

// ---------------------------------------------------------------------------
// Offline credential inspection (no network, no token refresh)

function decodeJwt(token) {
  try {
    const part = token.split('.')[1];
    return JSON.parse(Buffer.from(part.replace(/-/g, '+').replace(/_/g, '/'), 'base64').toString('utf8'));
  } catch {
    return null;
  }
}

/** Keychain entry Claude Code uses for a given CLAUDE_CONFIG_DIR value (macOS). */
export function claudeKeychainService(homeEnvValue) {
  const suffix = homeEnvValue
    ? `-${crypto.createHash('sha256').update(homeEnvValue.normalize('NFC')).digest('hex').slice(0, 8)}`
    : '';
  return `Claude Code-credentials${suffix}`;
}

function keychainAccount() {
  try {
    return process.env.USER || os.userInfo().username;
  } catch {
    return 'claude-code-user';
  }
}

function readClaudeKeychain(homeEnvValue) {
  const res = spawnSync(
    'security',
    ['find-generic-password', '-a', keychainAccount(), '-w', '-s', claudeKeychainService(homeEnvValue)],
    { encoding: 'utf8', timeout: 5000 },
  );
  if (res.status !== 0) return { found: false };
  const out = (res.stdout || '').trim();
  for (const candidate of [out, /^[0-9a-f]+$/i.test(out) ? Buffer.from(out, 'hex').toString('utf8') : null]) {
    if (!candidate) continue;
    try {
      return { found: true, data: JSON.parse(candidate) };
    } catch {
      // try next encoding
    }
  }
  return { found: true, data: null };
}

export function claudeJsonPath(cfg, name) {
  const env = profileHomeEnv(cfg, 'claude', name);
  return env ? path.join(env, '.claude.json') : path.join(home(), '.claude.json');
}

/** { present: true|false|null, method, plan, email, org } -- null means "can't tell offline". */
export function readCredentials(cfg, id, name) {
  const profile = cfg.tools[id].profiles[name];
  const dir = profileDir(cfg, id, name);
  const penv = profile.env || {};

  if (id === 'claude') {
    const acct = (readJson(claudeJsonPath(cfg, name), {}) || {}).oauthAccount || {};
    const base = { email: acct.emailAddress || null, org: acct.organizationName || null };
    if (penv.CLAUDE_CODE_OAUTH_TOKEN || penv.ANTHROPIC_API_KEY || penv.ANTHROPIC_AUTH_TOKEN) {
      return { ...base, present: true, method: 'env' };
    }
    let oauth = null;
    let present = false;
    if (IS_MAC) {
      const kc = readClaudeKeychain(profileHomeEnv(cfg, id, name));
      present = kc.found;
      oauth = kc.data && kc.data.claudeAiOauth;
    }
    if (!present) {
      const file = readJson(path.join(dir, '.credentials.json'), null);
      if (file) {
        present = true;
        oauth = file.claudeAiOauth || null;
      }
    }
    if (!present) return { ...base, present: false, method: null };
    return {
      ...base,
      present: true,
      method: oauth ? 'claude.ai' : 'credentials',
      plan: (oauth && oauth.subscriptionType) || null,
      hasRefresh: oauth ? Boolean(oauth.refreshToken) : null,
    };
  }

  // codex
  if (penv.CODEX_API_KEY || penv.OPENAI_API_KEY) return { present: true, method: 'env' };
  const auth = readJson(path.join(dir, 'auth.json'), null);
  if (auth) {
    if (auth.OPENAI_API_KEY) return { present: true, method: 'api-key' };
    const tokens = auth.tokens || {};
    const claims = decodeJwt(tokens.id_token || '') || {};
    const oa = claims['https://api.openai.com/auth'] || {};
    return {
      present: Boolean(tokens.refresh_token || tokens.access_token),
      method: 'chatgpt',
      email: claims.email || null,
      plan: oa.chatgpt_plan_type || null,
      hasRefresh: Boolean(tokens.refresh_token),
    };
  }
  // Credentials may live in the OS keyring (cli_auth_credentials_store); ask codex.
  const bin = which(binFor(getTool('codex')));
  if (!bin) return { present: null, method: null };
  const { env } = buildEnv(cfg, id, name);
  const res = spawnSync(bin, ['login', 'status'], { env, encoding: 'utf8', timeout: 15000 });
  if (res.status === 0) return { present: true, method: 'keyring' };
  return { present: res.status === 1 ? false : null, method: null };
}

// ---------------------------------------------------------------------------
// Usability: can we hand this profile to a new session right now?

export function usageWindows(st) {
  return (st.usage && st.usage.windows) || [];
}

export function evaluate(cfg, state, id, name, { now = Date.now(), cred } = {}) {
  const st = getProfileState(state, id, name);
  const credentials = cred || readCredentials(cfg, id, name);
  const reasons = [];
  if (credentials.present === false) reasons.push('not logged in');
  else if (st.needsLogin) reasons.push('login expired');
  const until = toDate(st.until);
  if (until && until.getTime() > now) {
    reasons.push(`${st.reason || 'limited'} for ${fmtDuration(until.getTime() - now)}`);
  }
  const threshold = cfg.failover.threshold;
  for (const w of usageWindows(st)) {
    const resets = toDate(w.resetsAt);
    if (w.pct >= threshold && (!resets || resets.getTime() > now)) {
      reasons.push(`${w.label} at ${Math.round(w.pct)}%${resets ? ` for ${fmtDuration(resets.getTime() - now)}` : ''}`);
    }
  }
  return { name, usable: reasons.length === 0, reasons, cred: credentials, state: st };
}

/** First usable profile, starting with `preferred` then the configured order. */
export function pickProfile(cfg, state, id, preferred, { exclude = [] } = {}) {
  const t = cfg.tools[id];
  const names = [preferred, ...t.order].filter((n, i, a) => n && t.profiles[n] && a.indexOf(n) === i);
  const evaluated = [];
  for (const n of names) {
    if (exclude.includes(n)) continue;
    const ev = evaluate(cfg, state, id, n);
    evaluated.push(ev);
    if (ev.usable) return { ...ev, skipped: evaluated.slice(0, -1) };
  }
  return { name: null, usable: false, skipped: evaluated };
}

// ---------------------------------------------------------------------------
// State changes

export function markLimited(cfg, id, name, { minutes, reason = 'limited', resetsAt } = {}) {
  return updateProfileState(id, name, (s) => {
    let until = toDate(resetsAt);
    if (!until) {
      // Prefer the reset time the status line / live check last reported.
      const soonest = usageWindows(s)
        .filter((w) => w.pct >= cfg.failover.threshold && toDate(w.resetsAt) > new Date())
        .map((w) => toDate(w.resetsAt))
        .sort((a, b) => a - b)[0];
      until = minutes ? null : soonest;
      if (!until) until = new Date(Date.now() + (minutes || cfg.failover.cooldownMinutes) * 60000);
    }
    return { ...s, until: until.toISOString(), reason, markedAt: new Date().toISOString() };
  });
}

export function markNeedsLogin(id, name, detail) {
  return updateProfileState(id, name, (s) => ({ ...s, needsLogin: true, loginDetail: detail || null }));
}

export function clearProfile(id, name) {
  return updateProfileState(id, name, (s) => {
    const { until, reason, markedAt, needsLogin, loginDetail, ...rest } = s;
    return rest;
  });
}

export function recordUsage(id, name, windows, source) {
  return updateProfileState(id, name, (s) => ({
    ...s,
    usage: { windows, source, at: new Date().toISOString() },
  }));
}

// ---------------------------------------------------------------------------
// Live checks (network). Claude: one tiny Haiku prompt. Codex: app-server account APIs (no tokens).

function runCapture(bin, args, { env, input, timeoutMs }) {
  return new Promise((resolve) => {
    let out = '';
    let err = '';
    let settled = false;
    const child = spawn(bin, args, { env, stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
    const timer = setTimeout(() => {
      child.kill();
      finish({ code: null, out, err: `${err}\n(timed out)` });
    }, timeoutMs);
    function finish(r) {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(r);
    }
    child.stdout.on('data', (d) => (out += d));
    child.stderr.on('data', (d) => (err += d));
    child.on('error', (e) => finish({ code: null, out, err: e.message }));
    child.on('close', (code) => finish({ code, out, err }));
    child.stdin.end(input || '');
  });
}

async function liveClaude(cfg, name) {
  const bin = which(binFor(getTool('claude')));
  if (!bin) return { status: 'error', detail: 'claude not found on PATH' };
  const { env } = buildEnv(cfg, 'claude', name);
  const r = await runCapture(
    bin,
    ['-p', 'Reply with just: OK', '--model', 'haiku', '--output-format', 'json', '--no-session-persistence'],
    { env, timeoutMs: 120000 },
  );
  let parsed = null;
  try {
    parsed = JSON.parse(r.out.trim().split('\n').pop());
  } catch {
    // not JSON (old CLI or hard failure)
  }
  const text = `${parsed ? parsed.result || '' : r.out}\n${r.err}`;
  if (r.code === 0 && parsed && !parsed.is_error) return { status: 'ok' };
  const kind = classifyFailure(text);
  return { status: kind || 'error', detail: text.trim().split('\n').filter(Boolean).slice(-1)[0] || `exit ${r.code}` };
}

function windowLabel(mins, fallback) {
  if (!mins) return fallback;
  if (mins % 1440 === 0) return `${mins / 1440}d`;
  if (mins % 60 === 0) return `${mins / 60}h`;
  return `${mins}m`;
}

/** Talk JSON-RPC to `codex app-server`: account/read + account/rateLimits/read. */
export function codexAppServer(bin, env, timeoutMs = 30000) {
  return new Promise((resolve) => {
    const child = spawn(bin, ['app-server'], { env, stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
    const results = {};
    let buf = '';
    let stderr = '';
    let done = false;
    const send = (o) => child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', ...o })}\n`);
    const finish = (extra) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      try {
        child.stdin.end();
        child.kill();
      } catch {
        // already gone
      }
      resolve({ ...results, stderr, ...extra });
    };
    const timer = setTimeout(() => finish({ timeout: true }), timeoutMs);
    child.on('error', (e) => finish({ spawnError: e.message }));
    child.on('close', () => finish({}));
    child.stderr.on('data', (d) => (stderr += d));
    child.stdout.on('data', (d) => {
      buf += d;
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i).trim();
        buf = buf.slice(i + 1);
        if (!line) continue;
        let msg;
        try {
          msg = JSON.parse(line);
        } catch {
          continue;
        }
        if (msg.id === 1) {
          send({ method: 'initialized' });
          send({ id: 2, method: 'account/read', params: {} });
          send({ id: 3, method: 'account/rateLimits/read', params: { excludeResetCreditDetails: true } });
        } else if (msg.id === 2) {
          results.account = msg.error ? { error: msg.error } : msg.result;
        } else if (msg.id === 3) {
          results.rateLimits = msg.error ? { error: msg.error } : msg.result;
        }
        if (results.account && results.rateLimits) finish({});
      }
    });
    send({ id: 1, method: 'initialize', params: { clientInfo: { name: 'aims', title: 'aims', version: '0.1.0' } } });
  });
}

// In live-check replies (protocol errors, no model text) a 401 reliably means a dead login.
function classifyLive(message) {
  return classifyFailure(message) || (/unauthori[sz]ed|\b401\b/i.test(message || '') ? 'auth' : null);
}

async function liveCodex(cfg, name) {
  const bin = which(binFor(getTool('codex')));
  if (!bin) return { status: 'error', detail: 'codex not found on PATH' };
  const { env } = buildEnv(cfg, 'codex', name);
  const r = await codexAppServer(bin, env);
  if (r.spawnError) return { status: 'error', detail: r.spawnError };
  if (!r.account) return { status: 'error', detail: r.timeout ? 'codex app-server timed out' : 'no answer from codex app-server' };
  const acct = r.account.account;
  if (r.account.error) return { status: classifyLive(r.account.error.message) || 'error', detail: r.account.error.message };
  if (!acct && r.account.requiresOpenaiAuth) return { status: 'auth', detail: 'not logged in' };
  const out = { status: 'ok', email: acct && acct.email, plan: acct && (acct.planType || acct.type) };
  const rl = r.rateLimits;
  if (!rl) return out;
  if (rl.error) {
    const kind = classifyLive(rl.error.message);
    return { ...out, status: kind || 'ok', detail: rl.error.message };
  }
  const snap = rl.rateLimits || {};
  out.windows = [
    ['primary', '5h'],
    ['secondary', '7d'],
  ]
    .filter(([k]) => snap[k])
    .map(([k, fallback]) => ({
      label: windowLabel(snap[k].windowDurationMins, fallback),
      pct: snap[k].usedPercent,
      resetsAt: snap[k].resetsAt ? toDate(snap[k].resetsAt).toISOString() : null,
    }));
  if (snap.rateLimitReachedType || rl.ordinaryUsageAllowed === false) {
    out.status = 'limit';
    out.detail = snap.rateLimitReachedType || 'usage not allowed';
  }
  return out;
}

/** Run a live check and store what it learned. */
export async function liveCheck(cfg, id, name) {
  const r = id === 'claude' ? await liveClaude(cfg, name) : await liveCodex(cfg, name);
  if (r.windows) recordUsage(id, name, r.windows, 'live');
  if (r.status === 'ok') {
    clearProfile(id, name);
  } else if (r.status === 'auth') {
    markNeedsLogin(id, name, r.detail);
  } else if (r.status === 'limit') {
    const windows = r.windows || [];
    const full = windows.filter((w) => w.pct >= cfg.failover.threshold);
    const resets = (full.length ? full : windows)
      .map((w) => toDate(w.resetsAt))
      .filter(Boolean)
      .sort((a, b) => b - a)[0];
    markLimited(cfg, id, name, { reason: 'limit', resetsAt: resets });
  }
  if (r.email || r.plan) {
    updateProfileState(id, name, (s) => ({ ...s, account: { email: r.email || null, plan: r.plan || null } }));
  }
  return r;
}

export function profileExists(cfg, id, name) {
  return Boolean(cfg.tools[id].profiles[name]) && fs.existsSync(profileDir(cfg, id, name));
}
