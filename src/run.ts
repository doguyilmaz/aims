import { analyzerFor } from './analyze.ts';
import { ensureHub, loadConfig, loadState, profileDir, updateProfileState } from './config.ts';
import { buildEnv, evaluate, markLimited, markNeedsLogin, pickProfile } from './health.ts';
import { detachLinks, ensureLinks } from './links.ts';
import { runAttached, runCaptured } from './proc.ts';
import { binFor, classifyFailure, getTool } from './tools.ts';
import type { Config, Evaluation, FailureKind, LinkReport, Sink, ToolId } from './types.ts';
import { info, readStdin, warn, which } from './util.ts';

/** Link a profile's shared entries into the hub; returns what happened. */
export function syncProfile(cfg: Config, id: ToolId, name: string, o: { fix?: boolean } = {}): LinkReport[] {
  const profile = cfg.tools[id].profiles[name];
  if (!profile || profile.existing) return [];
  const tool = getTool(id);
  const hub = ensureHub(cfg, id);
  const dir = profileDir(cfg, id, name);
  // `isolated` / `exclude` may have been added after the links were made.
  const exclude = profile.exclude ?? [];
  const detached = detachLinks({ hub, dir, names: profile.isolated ? null : exclude, history: tool.history });
  if (profile.isolated) return detached;
  return [
    ...detached,
    ...ensureLinks({ hub, dir, spec: tool.shared, sharedDirs: tool.sharedDirs, exclude, label: name, fix: o.fix ?? false }),
  ];
}

/** Terminal pin set by `aims env`. */
export function pinnedProfile(id: ToolId): string | null {
  return process.env[`AIMS_${id.toUpperCase()}_PROFILE`] || null;
}

/** Profile a new session would use: explicit > terminal pin > active > first. */
export function preferredProfile(cfg: Config, id: ToolId, explicit?: string | null): string | null {
  const t = cfg.tools[id];
  for (const n of [explicit, pinnedProfile(id), t.active, t.order[0]]) {
    if (n && t.profiles[n]) return n;
  }
  if (explicit) throw new Error(`no ${id} profile "${explicit}" (see: aims ls)`);
  return null;
}

export function resolveBin(id: ToolId): string {
  const tool = getTool(id);
  const bin = which(binFor(tool));
  if (!bin) throw new Error(`${tool.bin} not found on PATH (or set ${tool.binEnv})`);
  return bin;
}

const describeSkip = (ev: Evaluation) => `"${ev.name}" (${ev.reasons.join(', ')})`;
const kindLabel = (k: FailureKind) => (k === 'auth' ? 'login' : 'usage-limit');

export interface LaunchOptions {
  id: ToolId;
  /** Explicit profile; disables automatic failover. */
  profile?: string | null;
  args: string[];
  quiet?: boolean;
  /** stdin for headless runs (otherwise read from our own stdin). */
  input?: Uint8Array | null;
  sink?: Sink;
  errSink?: Sink;
  forceHeadless?: boolean;
  cwd?: string;
  timeoutMs?: number;
  /** Extra variables to drop from the child env. */
  stripEnv?: string[];
}

export interface LaunchResult {
  code: number;
  profile: string | null;
  failure?: FailureKind | null;
  /** Final answer, when the output format is structured (stream-json / --json). */
  finalText?: string | null;
  /** What the provider reported, for failed runs. */
  errorText?: string;
}

/** Launch `claude` / `codex` as a profile, with failover. */
export async function launch(o: LaunchOptions): Promise<LaunchResult> {
  const { id, args, quiet = false, stripEnv = [] } = o;
  const explicit = o.profile ?? null;
  const sink = o.sink ?? process.stdout;
  const errSink = o.errSink ?? process.stderr;
  const cfg = loadConfig();
  const tool = getTool(id);
  const bin = resolveBin(id);
  const t = cfg.tools[id];
  const say = quiet ? () => {} : info;

  const headless = Boolean(o.forceHeadless) || tool.isHeadless(args);
  const readInput = async () =>
    o.input !== undefined
      ? o.input
      : readStdin({
          // same rule as `claude -p`: wait 3s for the first byte
          onIdle: () => warn('no stdin data received in 3s, proceeding without it (use < /dev/null to skip the wait)'),
        });

  if (!Object.keys(t.profiles).length) {
    if (explicit) throw new Error(`no ${id} profile "${explicit}" (see: aims ls)`);
    // Nothing configured: behave exactly like the plain tool.
    const env: Record<string, string | undefined> = { ...process.env };
    for (const k of stripEnv) delete env[k];
    if (!headless) return { code: await runAttached(bin, args, env, o.cwd), profile: null };
    const analyzer = analyzerFor(id, args);
    const r = await runCaptured(bin, args, {
      env,
      cwd: o.cwd,
      input: await readInput(),
      sink,
      errSink,
      timeoutMs: o.timeoutMs,
      onLine: (stream, line) => analyzer.line(stream, line),
    });
    r.flush();
    return { code: r.code, profile: null, finalText: analyzer.finalText(), errorText: analyzer.errorText() };
  }

  const preferred = preferredProfile(cfg, id, explicit)!;
  const auto = cfg.failover.auto && !explicit;
  let name = preferred;
  if (auto) {
    const pick = pickProfile(cfg, loadState(), id, preferred);
    if (!pick.name) {
      warn(`no ${id} profile looks usable (${pick.skipped.map(describeSkip).join('; ')}); trying "${preferred}" anyway`);
    } else {
      if (pick.name !== preferred) say(`${id}: skipping ${pick.skipped.map(describeSkip).join(', ')} -> using "${pick.name}"`);
      name = pick.name;
    }
  } else {
    const ev = evaluate(cfg, loadState(), id, preferred);
    if (!ev.usable) warn(`${id} profile "${preferred}" may not work: ${ev.reasons.join(', ')}`);
  }

  const input = headless ? await readInput() : null;
  const tried: string[] = [];

  for (;;) {
    tried.push(name);
    for (const r of syncProfile(cfg, id, name)) {
      if (r.action === 'conflict' && !r.quiet) warn(`${id}/${name}: shared "${r.name}": ${r.detail} (aims doctor)`);
    }
    const { env, dropped } = buildEnv(cfg, id, name);
    for (const k of stripEnv) delete env[k];
    if (dropped.length) say(`ignoring ${dropped.join(', ')} from your shell so the "${name}" login is used`);
    // `aims failover` defaults to the profile that was actually used last.
    const usedAt = new Date().toISOString();
    updateProfileState(id, name, (s) => ({ ...s, lastUsedAt: usedAt }));

    if (!headless) return { code: await runAttached(bin, args, env, o.cwd), profile: name };

    const analyzer = analyzerFor(id, args);
    const r = await runCaptured(bin, args, {
      env,
      cwd: o.cwd,
      input,
      sink,
      errSink,
      timeoutMs: o.timeoutMs,
      onLine: (stream, line) => analyzer.line(stream, line),
    });
    const result = { profile: name, finalText: analyzer.finalText(), errorText: analyzer.errorText() };
    if (r.code === 0 && !analyzer.resultFailed()) {
      r.flush();
      return { ...result, code: r.code };
    }
    const kind = classifyFailure(result.errorText);
    if (kind === 'auth') markNeedsLogin(id, name, 'headless run reported a login problem');
    if (kind === 'limit') markLimited(cfg, id, name, { reason: 'limit' });
    const activity = analyzer.activity();
    // Retry only when the failed run provably changed nothing: no tool call seen in
    // structured output, or (output hidden) a login error, which stops the very first request.
    const safe = activity === 'none' || (activity === 'unknown' && kind === 'auth');
    const next = kind && auto && !r.committed && safe ? pickProfile(cfg, loadState(), id, null, { exclude: tried }) : null;
    if (!kind || !next?.name) {
      r.flush();
      if (kind) {
        const why = !auto
          ? ''
          : r.committed || !safe
            ? ' (not retried: the run may already have changed things)'
            : ' (no other usable profile)';
        say(`${id}/${name} hit a ${kindLabel(kind)} error; marked it so new runs use another profile${why}`);
      }
      return { ...result, code: r.code, failure: kind };
    }
    say(`${id}/${name} hit a ${kindLabel(kind)} error before doing anything -> retrying with "${next.name}"`);
    name = next.name;
  }
}
