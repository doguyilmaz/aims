import os from 'node:os';
import type { Sink } from './types.ts';
import { IS_WIN, errorMessage } from './util.ts';

type Env = Record<string, string | undefined>;

function winQuote(arg: string): string {
  if (arg === '') return '""';
  if (!/[\s"&|<>^%!()]/.test(arg)) return arg;
  return `"${arg.replace(/(\\*)"/g, '$1$1\\"').replace(/(\\+)$/, '$1$1')}"`;
}

/** argv to spawn; npm's .cmd shims on Windows have to go through cmd.exe. */
export function command(bin: string, args: string[]): string[] {
  if (IS_WIN && /\.(cmd|bat)$/i.test(bin)) {
    return ['cmd.exe', '/d', '/s', '/c', `"${[bin, ...args].map(winQuote).join(' ')}"`];
  }
  return [bin, ...args];
}

function signalExit(signal: string | null | undefined, code: number | null): number {
  if (signal) {
    const n = (os.constants.signals as Record<string, number>)[signal];
    return 128 + (n ?? 1);
  }
  return code ?? 1;
}

export interface SyncResult {
  code: number | null;
  stdout: string;
  stderr: string;
}

/** Short helper calls (`claude auth status`, `security`, `--version`). Never throws. */
export function runSync(
  bin: string,
  args: string[],
  o: { env?: Env; input?: string; timeoutMs?: number; cwd?: string } = {},
): SyncResult {
  try {
    const r = Bun.spawnSync(command(bin, args), {
      env: o.env ?? process.env,
      cwd: o.cwd,
      stdin: o.input !== undefined ? Buffer.from(o.input) : 'ignore',
      stdout: 'pipe',
      stderr: 'pipe',
      timeout: o.timeoutMs ?? 30000,
      windowsHide: true,
    });
    return { code: r.exitCode, stdout: r.stdout.toString(), stderr: r.stderr.toString() };
  } catch (err) {
    return { code: null, stdout: '', stderr: errorMessage(err) };
  }
}

type Child = { kill(signal?: number | NodeJS.Signals): void };
const active = new Set<Child>();

/** Stop every tool process aims started (MCP server shutdown). */
export function killActiveChildren(signal: NodeJS.Signals = 'SIGTERM'): void {
  for (const child of active) child.kill(signal);
}

/**
 * While `child` runs: SIGTERM/SIGHUP are passed on so it never outlives aims.
 * SIGINT is passed on only when `forwardInt` (a terminal already delivers
 * Ctrl+C to the whole process group; a second one would make an interactive
 * TUI quit).
 */
async function supervise(child: Child & { exited: Promise<number>; signalCode: string | null }, forwardInt: boolean) {
  const handlers: Array<[NodeJS.Signals, () => void]> = [
    ['SIGINT', forwardInt ? () => child.kill('SIGINT') : () => {}],
    ['SIGTERM', () => child.kill('SIGTERM')],
  ];
  if (!IS_WIN) handlers.push(['SIGHUP', () => child.kill('SIGHUP')], ['SIGQUIT', () => {}]);
  for (const [sig, h] of handlers) process.on(sig, h);
  active.add(child);
  try {
    const code = await child.exited;
    return signalExit(child.signalCode, code);
  } finally {
    active.delete(child);
    for (const [sig, h] of handlers) process.removeListener(sig, h);
  }
}

/** Run with the terminal attached. Ctrl+C belongs to the tool, not to aims. */
export async function runAttached(bin: string, args: string[], env: Env, cwd?: string): Promise<number> {
  const proc = Bun.spawn(command(bin, args), { env, cwd, stdin: 'inherit', stdout: 'inherit', stderr: 'inherit' });
  return supervise(proc, false);
}

/** Calls `fn` with each complete line; `end()` flushes a trailing partial line. */
function lineSplitter(fn: (line: string) => void) {
  const dec = new TextDecoder();
  let buf = '';
  return {
    push(chunk: Uint8Array) {
      buf += dec.decode(chunk, { stream: true });
      let i: number;
      while ((i = buf.indexOf('\n')) >= 0) {
        fn(buf.slice(0, i).replace(/\r$/, ''));
        buf = buf.slice(i + 1);
      }
    },
    end() {
      if (buf) fn(buf);
      buf = '';
    },
  };
}

export const COMMIT_AFTER_MS = 8000; // limit/login errors show up with the first request
const COMMIT_AFTER_BYTES = 256 * 1024;
const TAIL_CHARS = 64 * 1024;

export interface CapturedResult {
  code: number;
  /** Last ~64KB of stdout+stderr, for classifying failures. */
  tail: string;
  /** stdout was already passed through (too late to retry cleanly). */
  committed: boolean;
  /** Pass held-back stdout through. */
  flush: () => void;
}

/**
 * Run headless. stderr streams live; stdout is held back briefly so a
 * limit/login failure can be retried on another profile without mixing two
 * runs' output.
 */
export async function runCaptured(
  bin: string,
  args: string[],
  o: {
    env: Env;
    cwd?: string;
    input?: Uint8Array | null;
    sink: Sink;
    errSink: Sink;
    timeoutMs?: number;
    onLine?: (stream: 'out' | 'err', line: string) => void;
  },
): Promise<CapturedResult> {
  const proc = Bun.spawn(command(bin, args), {
    env: o.env,
    cwd: o.cwd,
    stdin: o.input && o.input.length ? o.input : 'ignore',
    stdout: 'pipe',
    stderr: 'pipe',
    windowsHide: true,
  });
  let held: Uint8Array[] = [];
  let heldBytes = 0;
  let committed = false;
  let tail = '';
  const outDec = new TextDecoder();
  const errDec = new TextDecoder();
  const keep = (dec: TextDecoder, chunk: Uint8Array) => {
    tail = (tail + dec.decode(chunk, { stream: true })).slice(-TAIL_CHARS);
  };
  const commit = () => {
    if (committed) return;
    committed = true;
    for (const chunk of held) o.sink.write(chunk);
    held = [];
  };
  const commitTimer = setTimeout(commit, COMMIT_AFTER_MS);
  const killer = o.timeoutMs
    ? setTimeout(() => {
        o.errSink.write(`aims: stopped after ${Math.round(o.timeoutMs! / 1000)}s\n`);
        proc.kill();
      }, o.timeoutMs)
    : undefined;

  const outLines = lineSplitter((l) => o.onLine?.('out', l));
  const errLines = lineSplitter((l) => o.onLine?.('err', l));
  const pumpOut = (async () => {
    for await (const chunk of proc.stdout) {
      keep(outDec, chunk);
      outLines.push(chunk);
      if (committed) {
        o.sink.write(chunk);
        continue;
      }
      held.push(chunk);
      heldBytes += chunk.length;
      if (heldBytes > COMMIT_AFTER_BYTES) commit();
    }
  })();
  const pumpErr = (async () => {
    for await (const chunk of proc.stderr) {
      keep(errDec, chunk);
      errLines.push(chunk);
      o.errSink.write(chunk);
    }
  })();

  const code = await supervise(proc, true);
  await Promise.allSettled([pumpOut, pumpErr]);
  outLines.end();
  errLines.end();
  clearTimeout(commitTimer);
  clearTimeout(killer);
  return { code, tail, committed, flush: commit };
}

/**
 * Minimal JSON-RPC-over-stdio client (newline-delimited), used for
 * `codex app-server`. Sends `initialize`, then each request; resolves with the
 * results keyed by method, or whatever arrived before the timeout.
 */
export async function jsonRpcSession(
  bin: string,
  args: string[],
  env: Env,
  requests: Array<{ method: string; params?: unknown }>,
  timeoutMs = 30000,
): Promise<{ results: Record<string, { result?: unknown; error?: { message: string } }>; error?: string }> {
  let proc: ReturnType<typeof Bun.spawn<'pipe', 'pipe', 'pipe'>>;
  try {
    proc = Bun.spawn(command(bin, args), { env, stdin: 'pipe', stdout: 'pipe', stderr: 'pipe', windowsHide: true });
  } catch (err) {
    return { results: {}, error: errorMessage(err) };
  }
  const results: Record<string, { result?: unknown; error?: { message: string } }> = {};
  const send = (o: object) => {
    proc.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', ...o })}\n`);
    proc.stdin.flush();
  };
  const byId = new Map<number, string>();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    proc.kill();
  }, timeoutMs);
  void (async () => {
    for await (const _ of proc.stderr) {
      // drained so the child never blocks on a full pipe
    }
  })();

  send({ id: 0, method: 'initialize', params: { clientInfo: { name: 'aims', title: 'aims', version: '1' } } });
  const decoder = new TextDecoder();
  let buf = '';
  try {
    outer: for await (const chunk of proc.stdout) {
      buf += decoder.decode(chunk, { stream: true });
      let i: number;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i).trim();
        buf = buf.slice(i + 1);
        if (!line) continue;
        let msg: { id?: number; result?: unknown; error?: { message: string } };
        try {
          msg = JSON.parse(line);
        } catch {
          continue;
        }
        if (msg.id === 0) {
          send({ method: 'initialized' });
          requests.forEach((r, n) => {
            byId.set(n + 1, r.method);
            send({ id: n + 1, method: r.method, params: r.params });
          });
        } else if (typeof msg.id === 'number' && byId.has(msg.id)) {
          results[byId.get(msg.id)!] = { result: msg.result, error: msg.error };
          if (Object.keys(results).length === requests.length) break outer;
        }
      }
    }
  } finally {
    clearTimeout(timer);
    try {
      proc.stdin.end();
    } catch {
      // already closed
    }
    proc.kill();
  }
  return timedOut && Object.keys(results).length < requests.length
    ? { results, error: `${bin} ${args.join(' ')} timed out` }
    : { results };
}
