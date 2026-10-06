import { spawn } from 'node:child_process';
import { ensureHub, loadConfig, loadState, profileDir } from './config.js';
import { buildEnv, evaluate, markLimited, markNeedsLogin, pickProfile } from './health.js';
import { ensureLinks } from './links.js';
import { binFor, classifyFailure, getTool } from './tools.js';
import { IS_WIN, info, readStdin, warn, which } from './util.js';

/** Link shared entries for a profile; returns the problems worth showing. */
export function syncProfile(cfg, id, name, { fix = false } = {}) {
  const tool = getTool(id);
  const profile = cfg.tools[id].profiles[name];
  if (!profile || profile.existing || profile.isolated) return [];
  const report = ensureLinks({
    hub: ensureHub(cfg, id),
    dir: profileDir(cfg, id, name),
    spec: tool.shared,
    sharedDirs: tool.sharedDirs,
    exclude: profile.exclude || [],
    label: name,
    fix,
  });
  return report;
}

export function pinnedProfile(id) {
  return process.env[`AIMS_${id.toUpperCase()}_PROFILE`] || null;
}

/** Profile a new session would use (explicit > terminal pin > active > first). */
export function preferredProfile(cfg, id, explicit) {
  const t = cfg.tools[id];
  for (const n of [explicit, pinnedProfile(id), t.active, t.order[0]]) {
    if (n && t.profiles[n]) return n;
  }
  if (explicit) throw new Error(`no ${id} profile "${explicit}" (see: aims ls)`);
  return null;
}

export function resolveBin(id) {
  const tool = getTool(id);
  const bin = which(binFor(tool));
  if (!bin) throw new Error(`${tool.bin} not found on PATH (or set ${tool.binEnv})`);
  return bin;
}

function winQuote(arg) {
  if (arg === '') return '""';
  if (!/[\s"&|<>^%!()]/.test(arg)) return arg;
  return `"${arg.replace(/(\\*)"/g, '$1$1\\"').replace(/(\\+)$/, '$1$1')}"`;
}

/** spawn() that also works for npm's .cmd shims on Windows. */
export function spawnTool(bin, args, opts) {
  if (IS_WIN && /\.(cmd|bat)$/i.test(bin)) {
    return spawn([winQuote(bin), ...args.map(winQuote)].join(' '), [], { ...opts, shell: true });
  }
  return spawn(bin, args, opts);
}

/** Run with the terminal attached; Ctrl+C belongs to the tool, not to aims. */
function runAttached(bin, args, env, cwd) {
  return new Promise((resolve, reject) => {
    const child = spawnTool(bin, args, { env, cwd, stdio: 'inherit' });
    const ignore = () => {};
    const forward = (sig) => () => child.kill(sig);
    const handlers = {
      SIGINT: ignore,
      SIGQUIT: ignore,
      SIGTERM: forward('SIGTERM'),
      SIGHUP: forward('SIGHUP'),
    };
    for (const [sig, h] of Object.entries(handlers)) process.on(sig, h);
    const cleanup = () => {
      for (const [sig, h] of Object.entries(handlers)) process.removeListener(sig, h);
    };
    child.on('error', (e) => {
      cleanup();
      reject(e);
    });
    child.on('exit', (code, signal) => {
      cleanup();
      resolve(signal ? 128 + (signalNumber(signal) || 1) : code ?? 1);
    });
  });
}

function signalNumber(sig) {
  return { SIGHUP: 1, SIGINT: 2, SIGQUIT: 3, SIGKILL: 9, SIGTERM: 15 }[sig];
}

const COMMIT_AFTER_MS = 8000; // limit/login errors show up within the first request
const COMMIT_AFTER_BYTES = 256 * 1024;
const TAIL_BYTES = 64 * 1024;

/**
 * Run headless, holding stdout back briefly so a limit/login failure can be
 * retried on another profile without mixing two runs' output.
 */
function runCaptured(bin, args, env, input, { sink = process.stdout, errSink = process.stderr, cwd, timeoutMs } = {}) {
  return new Promise((resolve, reject) => {
    const child = spawnTool(bin, args, { env, cwd, stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
    const killer = timeoutMs
      ? setTimeout(() => {
          errSink.write(`aims: stopped after ${Math.round(timeoutMs / 1000)}s\n`);
          child.kill();
        }, timeoutMs)
      : null;
    let held = [];
    let heldBytes = 0;
    let committed = false;
    let tail = '';
    const keep = (d) => {
      tail = (tail + d.toString()).slice(-TAIL_BYTES);
    };
    const commit = () => {
      if (committed) return;
      committed = true;
      for (const chunk of held) sink.write(chunk);
      held = [];
    };
    const timer = setTimeout(commit, COMMIT_AFTER_MS);
    child.stdout.on('data', (d) => {
      keep(d);
      if (committed) return sink.write(d);
      held.push(d);
      heldBytes += d.length;
      if (heldBytes > COMMIT_AFTER_BYTES) commit();
    });
    child.stderr.on('data', (d) => {
      keep(d);
      errSink.write(d);
    });
    child.on('error', (e) => {
      clearTimeout(timer);
      clearTimeout(killer);
      reject(e);
    });
    child.on('close', (code, signal) => {
      clearTimeout(timer);
      clearTimeout(killer);
      resolve({
        code: signal ? 128 + (signalNumber(signal) || 1) : code ?? 1,
        tail,
        committed,
        flush: commit,
      });
    });
    child.stdin.on('error', () => {});
    child.stdin.end(input || undefined);
  });
}

// `--output-format json|stream-json` can exit 0 with a failed final result.
// Only the final {"type":"result"} record counts, not individual tool errors.
const FAILED_RESULT_RE = /"type"\s*:\s*"result"[^\n]*"is_error"\s*:\s*true|"is_error"\s*:\s*true[^\n]*"type"\s*:\s*"result"/;

function looksFailed(code, tail) {
  return code !== 0 || FAILED_RESULT_RE.test(tail.slice(-8000));
}

function describeSkip(ev) {
  return `"${ev.name}" (${ev.reasons.join(', ')})`;
}

/**
 * Launch `claude` / `codex` under a profile.
 * @param {object} o
 * @param {string} o.id           tool id
 * @param {string} [o.profile]    explicit profile (disables automatic failover)
 * @param {string[]} o.args       arguments for the tool
 * @param {boolean} [o.quiet]
 * @param {Buffer} [o.input]      stdin for headless runs (read from our stdin otherwise)
 * @param {object} [o.sink]       where headless stdout goes (default process.stdout)
 * @param {string[]} [o.stripEnv] extra variables to drop from the child env
 */
export async function launch({
  id,
  profile: explicit,
  args,
  quiet = false,
  input,
  sink,
  errSink,
  forceHeadless,
  cwd,
  timeoutMs,
  stripEnv = [],
}) {
  const cfg = loadConfig();
  const tool = getTool(id);
  const bin = resolveBin(id);
  const t = cfg.tools[id];
  const say = quiet ? () => {} : info;

  if (!Object.keys(t.profiles).length) {
    if (explicit) throw new Error(`no ${id} profile "${explicit}" (see: aims ls)`);
    // Nothing configured: behave exactly like the plain tool.
    return { code: await runAttached(bin, args, process.env, cwd), profile: null };
  }

  const preferred = preferredProfile(cfg, id, explicit);
  const auto = cfg.failover.auto && !explicit;
  let state = loadState();
  let pick;
  if (auto) {
    pick = pickProfile(cfg, state, id, preferred);
    if (!pick.name) {
      warn(`no ${id} profile looks usable (${pick.skipped.map(describeSkip).join('; ')}); trying "${preferred}" anyway`);
      pick = { name: preferred };
    } else if (pick.name !== preferred) {
      say(`${id}: skipping ${pick.skipped.map(describeSkip).join(', ')} -> using "${pick.name}"`);
    }
  } else {
    pick = { name: preferred };
    const ev = evaluate(cfg, state, id, preferred);
    if (!ev.usable) warn(`${id} profile "${preferred}" may not work: ${ev.reasons.join(', ')}`);
  }

  const headless = forceHeadless || tool.isHeadless(args);
  const tried = [];
  let name = pick.name;
  const stdinBuf = !headless
    ? null
    : input !== undefined
      ? input
      : await readStdin({
          // same rule as `claude -p`: wait 3s for the first byte
          onIdle: () => warn('no stdin data received in 3s, proceeding without it (use < /dev/null to skip the wait)'),
        });

  for (;;) {
    tried.push(name);
    for (const r of syncProfile(cfg, id, name)) {
      if (r.action === 'conflict' && !r.quiet) warn(`${id}/${name}: shared "${r.name}": ${r.detail} (aims doctor)`);
    }
    const { env, dropped } = buildEnv(cfg, id, name);
    for (const k of stripEnv) delete env[k];
    if (dropped.length && !quiet) {
      say(`ignoring ${dropped.join(', ')} from your shell so the "${name}" login is used`);
    }

    if (!headless) return { code: await runAttached(bin, args, env, cwd), profile: name };

    const r = await runCaptured(bin, args, env, stdinBuf, { sink, errSink, cwd, timeoutMs });
    if (!looksFailed(r.code, r.tail)) {
      r.flush();
      return { code: r.code, profile: name };
    }
    // Errors are printed last; the rest of the tail may be model output.
    const kind = classifyFailure(r.tail.slice(-4000));
    if (kind === 'auth') markNeedsLogin(id, name, 'headless run reported a login problem');
    if (kind === 'limit') markLimited(cfg, id, name, { reason: 'limit' });
    if (!kind || r.committed || !auto) {
      r.flush();
      if (kind) say(`${id}/${name} hit a ${kind === 'auth' ? 'login' : 'usage-limit'} error; marked it so the next run uses another profile`);
      return { code: r.code, profile: name, failure: kind };
    }
    state = loadState();
    const next = pickProfile(cfg, state, id, null, { exclude: tried });
    if (!next.name) {
      r.flush();
      warn(`${id}/${name} hit a ${kind} error and no other profile is usable`);
      return { code: r.code, profile: name, failure: kind };
    }
    say(`${id}/${name} hit a ${kind === 'auth' ? 'login' : 'usage-limit'} error -> retrying with "${next.name}"`);
    name = next.name;
  }
}
