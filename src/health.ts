import crypto from 'node:crypto';
import os from 'node:os';
import path from 'node:path';
import { getProfileState, profileDir, profileHomeEnv, updateProfileState } from './config.ts';
import { command, jsonRpcSession, runSync } from './proc.ts';
import { binFor, classifyFailure, getTool } from './tools.ts';
import type {
  Config,
  Credentials,
  Evaluation,
  FailureKind,
  ProfileState,
  State,
  ToolId,
  UsageWindow,
} from './types.ts';
import { IS_MAC, fmtDuration, home, readJson, toDate, which } from './util.ts';

type Env = Record<string, string | undefined>;

// ---------------------------------------------------------------------------
// Child environment

/** Env for running `id` as `name`: points the tool at the profile and drops overriding vars. */
export function buildEnv(cfg: Config, id: ToolId, name: string, base: Env = process.env): { env: Env; dropped: string[] } {
  const tool = getTool(id);
  const profile = cfg.tools[id].profiles[name];
  if (!profile) throw new Error(`no ${id} profile "${name}"`);
  const env: Env = { ...base };
  const dropped: string[] = [];
  for (const k of tool.conflictingEnv) {
    if (env[k] && !(profile.env && k in profile.env)) {
      delete env[k];
      dropped.push(k);
    }
  }
  const homeEnv = profileHomeEnv(cfg, id, name);
  if (homeEnv) env[tool.homeEnv] = homeEnv;
  else delete env[tool.homeEnv];
  Object.assign(env, profile.env ?? {});
  env.AIMS_SESSION_PROFILE = `${id}:${name}`;
  return { env, dropped };
}

// ---------------------------------------------------------------------------
// Offline credential inspection (no network, never refreshes a token)

function decodeJwt(token: string): Record<string, unknown> | null {
  try {
    const part = token.split('.')[1] ?? '';
    return JSON.parse(Buffer.from(part, 'base64url').toString('utf8'));
  } catch {
    return null;
  }
}

/** Keychain entry Claude Code uses for a given CLAUDE_CONFIG_DIR value (macOS). */
export function claudeKeychainService(homeEnvValue: string | null): string {
  const suffix = homeEnvValue
    ? `-${crypto.createHash('sha256').update(homeEnvValue.normalize('NFC')).digest('hex').slice(0, 8)}`
    : '';
  return `Claude Code-credentials${suffix}`;
}

function keychainAccount(): string {
  try {
    return process.env.USER || os.userInfo().username;
  } catch {
    return 'claude-code-user';
  }
}

interface ClaudeOauth {
  accessToken?: string;
  refreshToken?: string;
  subscriptionType?: string;
}

function readClaudeKeychain(homeEnvValue: string | null): { found: boolean; oauth: ClaudeOauth | null } {
  const r = runSync('security', ['find-generic-password', '-a', keychainAccount(), '-w', '-s', claudeKeychainService(homeEnvValue)], {
    timeoutMs: 5000,
  });
  if (r.code !== 0) return { found: false, oauth: null };
  const out = r.stdout.trim();
  for (const candidate of [out, /^[0-9a-f]+$/i.test(out) ? Buffer.from(out, 'hex').toString('utf8') : null]) {
    if (!candidate) continue;
    try {
      return { found: true, oauth: (JSON.parse(candidate) as { claudeAiOauth?: ClaudeOauth }).claudeAiOauth ?? null };
    } catch {
      // try the next encoding
    }
  }
  return { found: true, oauth: null };
}

export function claudeJsonPath(cfg: Config, name: string): string {
  const env = profileHomeEnv(cfg, 'claude', name);
  return env ? path.join(env, '.claude.json') : path.join(home(), '.claude.json');
}

/** What can be known offline about a profile's login. */
export function readCredentials(cfg: Config, id: ToolId, name: string): Credentials {
  const profile = cfg.tools[id].profiles[name]!;
  const dir = profileDir(cfg, id, name);
  const penv = profile.env ?? {};

  if (id === 'claude') {
    const acct = readJson<{ oauthAccount?: { emailAddress?: string; organizationName?: string } }>(claudeJsonPath(cfg, name), {})
      .oauthAccount ?? {};
    const base = { email: acct.emailAddress ?? null, org: acct.organizationName ?? null };
    if (penv.CLAUDE_CODE_OAUTH_TOKEN || penv.ANTHROPIC_API_KEY || penv.ANTHROPIC_AUTH_TOKEN) {
      return { ...base, present: true, method: 'env' };
    }
    let oauth: ClaudeOauth | null = null;
    let present = false;
    if (IS_MAC) {
      const kc = readClaudeKeychain(profileHomeEnv(cfg, id, name));
      present = kc.found;
      oauth = kc.oauth;
    }
    if (!present) {
      const file = readJson<{ claudeAiOauth?: ClaudeOauth } | null>(path.join(dir, '.credentials.json'), null);
      if (file) {
        present = true;
        oauth = file.claudeAiOauth ?? null;
      }
    }
    if (!present) return { ...base, present: false, method: null };
    return {
      ...base,
      present: true,
      method: oauth ? 'claude.ai' : 'credentials',
      plan: oauth?.subscriptionType ?? null,
      hasRefresh: oauth ? Boolean(oauth.refreshToken) : null,
    };
  }

  // codex
  if (penv.CODEX_API_KEY || penv.OPENAI_API_KEY) return { present: true, method: 'env' };
  const auth = readJson<{
    OPENAI_API_KEY?: string | null;
    tokens?: { id_token?: string; access_token?: string; refresh_token?: string };
  } | null>(path.join(dir, 'auth.json'), null);
  if (auth) {
    if (auth.OPENAI_API_KEY) return { present: true, method: 'api-key' };
    const tokens = auth.tokens ?? {};
    const claims = decodeJwt(tokens.id_token ?? '') ?? {};
    const oa = (claims['https://api.openai.com/auth'] ?? {}) as { chatgpt_plan_type?: string };
    return {
      present: Boolean(tokens.refresh_token || tokens.access_token),
      method: 'chatgpt',
      email: typeof claims.email === 'string' ? claims.email : null,
      plan: oa.chatgpt_plan_type ?? null,
      hasRefresh: Boolean(tokens.refresh_token),
    };
  }
  // Credentials may live in the OS keyring (cli_auth_credentials_store); ask codex.
  const bin = which(binFor(getTool('codex')));
  if (!bin) return { present: null, method: null };
  const r = runSync(bin, ['login', 'status'], { env: buildEnv(cfg, id, name).env, timeoutMs: 15000 });
  if (r.code === 0) return { present: true, method: 'keyring' };
  return { present: r.code === 1 ? false : null, method: null };
}

// ---------------------------------------------------------------------------
// Usability: can a new session use this profile right now?

export function usageWindows(st: ProfileState): UsageWindow[] {
  return st.usage?.windows ?? [];
}

export function evaluate(
  cfg: Config,
  state: State,
  id: ToolId,
  name: string,
  o: { now?: number; cred?: Credentials } = {},
): Evaluation {
  const now = o.now ?? Date.now();
  const st = getProfileState(state, id, name);
  const cred = o.cred ?? readCredentials(cfg, id, name);
  const reasons: string[] = [];
  if (cred.present === false) reasons.push('not logged in');
  else if (st.needsLogin) reasons.push('login expired');
  const until = toDate(st.until);
  if (until && until.getTime() > now) reasons.push(`${st.reason ?? 'limited'} for ${fmtDuration(until.getTime() - now)}`);
  for (const w of usageWindows(st)) {
    const resets = toDate(w.resetsAt);
    if (w.pct >= cfg.failover.threshold && (!resets || resets.getTime() > now)) {
      reasons.push(`${w.label} at ${Math.round(w.pct)}%${resets ? ` for ${fmtDuration(resets.getTime() - now)}` : ''}`);
    }
  }
  return { name, usable: reasons.length === 0, reasons, cred, state: st };
}

export interface Pick {
  name: string | null;
  usable: boolean;
  reasons: string[];
  skipped: Evaluation[];
}

/** First usable profile, starting with `preferred`, then the configured order. */
export function pickProfile(
  cfg: Config,
  state: State,
  id: ToolId,
  preferred: string | null,
  o: { exclude?: string[] } = {},
): Pick {
  const t = cfg.tools[id];
  const names = [preferred, ...t.order].filter(
    (n, i, a): n is string => Boolean(n && t.profiles[n]) && a.indexOf(n) === i,
  );
  const skipped: Evaluation[] = [];
  for (const n of names) {
    if (o.exclude?.includes(n)) continue;
    const ev = evaluate(cfg, state, id, n);
    if (ev.usable) return { name: n, usable: true, reasons: [], skipped };
    skipped.push(ev);
  }
  return { name: null, usable: false, reasons: [], skipped };
}

// ---------------------------------------------------------------------------
// State changes

export function markLimited(
  cfg: Config,
  id: ToolId,
  name: string,
  o: { minutes?: number; reason?: string; resetsAt?: Date | string | null } = {},
): ProfileState {
  return updateProfileState(id, name, (s) => {
    let until = toDate(o.resetsAt);
    const known = toDate(s.until);
    // Without new information, never stretch a reset time that is already known.
    if (!until && !o.minutes && known && known.getTime() > Date.now()) return s;
    if (!until && !o.minutes) {
      // Prefer the reset time the status line / live check last reported.
      until =
        usageWindows(s)
          .filter((w) => w.pct >= cfg.failover.threshold)
          .map((w) => toDate(w.resetsAt))
          .filter((d): d is Date => d !== null && d.getTime() > Date.now())
          .sort((a, b) => a.getTime() - b.getTime())[0] ?? null;
    }
    until ??= new Date(Date.now() + (o.minutes ?? cfg.failover.cooldownMinutes) * 60000);
    return { ...s, until: until.toISOString(), reason: o.reason ?? 'limited', markedAt: new Date().toISOString() };
  });
}

export function markNeedsLogin(id: ToolId, name: string, detail?: string): ProfileState {
  return updateProfileState(id, name, (s) => ({ ...s, needsLogin: true, loginDetail: detail ?? null }));
}

export function clearProfile(id: ToolId, name: string): ProfileState {
  return updateProfileState(id, name, ({ until, reason, markedAt, needsLogin, loginDetail, ...rest }) => rest);
}

export function recordUsage(id: ToolId, name: string, windows: UsageWindow[], source: string): ProfileState {
  return updateProfileState(id, name, (s) => ({ ...s, usage: { windows, source, at: new Date().toISOString() } }));
}

// ---------------------------------------------------------------------------
// Live checks (network). Claude: one tiny Haiku prompt. Codex: app-server account APIs (no tokens).

export interface LiveResult {
  status: 'ok' | FailureKind | 'error';
  detail?: string;
  email?: string | null;
  plan?: string | null;
  windows?: UsageWindow[];
}

async function liveClaude(cfg: Config, name: string): Promise<LiveResult> {
  const bin = which(binFor(getTool('claude')));
  if (!bin) return { status: 'error', detail: 'claude not found on PATH' };
  const proc = Bun.spawn(
    command(bin, ['-p', 'Reply with just: OK', '--model', 'haiku', '--output-format', 'json', '--no-session-persistence']),
    { env: buildEnv(cfg, 'claude', name).env, stdin: 'ignore', stdout: 'pipe', stderr: 'pipe', timeout: 120000 },
  );
  const [out, err, code] = await Promise.all([new Response(proc.stdout).text(), new Response(proc.stderr).text(), proc.exited]);
  let parsed: { is_error?: boolean; result?: string } | null = null;
  try {
    parsed = JSON.parse(out.trim().split('\n').pop() ?? '');
  } catch {
    // not JSON: an old CLI or a hard failure
  }
  if (code === 0 && parsed && !parsed.is_error) return { status: 'ok' };
  const text = `${parsed ? (parsed.result ?? '') : out}\n${err}`;
  return {
    status: classifyFailure(text) ?? 'error',
    detail: text.trim().split('\n').filter(Boolean).pop() ?? `exit ${code}`,
  };
}

function windowLabel(mins: number | null | undefined, fallback: string): string {
  if (!mins) return fallback;
  if (mins % 1440 === 0) return `${mins / 1440}d`;
  if (mins % 60 === 0) return `${mins / 60}h`;
  return `${mins}m`;
}

interface CodexWindow {
  usedPercent: number;
  resetsAt?: number | null;
  windowDurationMins?: number | null;
}

async function liveCodex(cfg: Config, name: string): Promise<LiveResult> {
  const bin = which(binFor(getTool('codex')));
  if (!bin) return { status: 'error', detail: 'codex not found on PATH' };
  const { results, error } = await jsonRpcSession(bin, ['app-server'], buildEnv(cfg, 'codex', name).env, [
    { method: 'account/read', params: {} },
    { method: 'account/rateLimits/read', params: { excludeResetCreditDetails: true } },
  ]);
  const acctRes = results['account/read'];
  if (!acctRes) return { status: 'error', detail: error ?? 'no answer from codex app-server' };
  if (acctRes.error) return { status: classifyFailure(acctRes.error.message) ?? 'error', detail: acctRes.error.message };
  const acct = acctRes.result as {
    account: { type: string; email?: string | null; planType?: string } | null;
    requiresOpenaiAuth: boolean;
  };
  if (!acct.account && acct.requiresOpenaiAuth) return { status: 'auth', detail: 'not logged in' };
  const out: LiveResult = {
    status: 'ok',
    email: acct.account?.email ?? null,
    plan: acct.account?.planType ?? acct.account?.type ?? null,
  };
  const rl = results['account/rateLimits/read'];
  if (!rl) return out;
  if (rl.error) return { ...out, status: classifyFailure(rl.error.message) ?? 'ok', detail: rl.error.message };
  const body = rl.result as {
    ordinaryUsageAllowed?: boolean | null;
    rateLimits?: { primary?: CodexWindow | null; secondary?: CodexWindow | null; rateLimitReachedType?: string | null };
  };
  const snap = body.rateLimits ?? {};
  out.windows = (
    [
      ['primary', '5h'],
      ['secondary', '7d'],
    ] as const
  ).flatMap(([k, fallback]) => {
    const w = snap[k];
    if (!w) return [];
    return [{ label: windowLabel(w.windowDurationMins, fallback), pct: w.usedPercent, resetsAt: toDate(w.resetsAt)?.toISOString() ?? null }];
  });
  if (snap.rateLimitReachedType || body.ordinaryUsageAllowed === false) {
    out.status = 'limit';
    out.detail = snap.rateLimitReachedType ?? 'usage not allowed';
  }
  return out;
}

/** Run a live check and store what it learned. */
export async function liveCheck(cfg: Config, id: ToolId, name: string): Promise<LiveResult> {
  const r = id === 'claude' ? await liveClaude(cfg, name) : await liveCodex(cfg, name);
  if (r.windows) recordUsage(id, name, r.windows, 'live');
  if (r.status === 'ok') {
    clearProfile(id, name);
  } else if (r.status === 'auth') {
    markNeedsLogin(id, name, r.detail);
  } else if (r.status === 'limit') {
    const windows = r.windows ?? [];
    const full = windows.filter((w) => w.pct >= cfg.failover.threshold);
    const resets = (full.length ? full : windows)
      .map((w) => toDate(w.resetsAt))
      .filter((d): d is Date => d !== null)
      .sort((a, b) => b.getTime() - a.getTime())[0];
    markLimited(cfg, id, name, { reason: 'limit', resetsAt: resets ?? null });
  }
  if (r.email || r.plan) {
    updateProfileState(id, name, (s) => ({ ...s, account: { email: r.email ?? null, plan: r.plan ?? null } }));
  }
  return r;
}
