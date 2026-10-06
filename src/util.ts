import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

export const IS_WIN = process.platform === 'win32';
export const IS_MAC = process.platform === 'darwin';

/** Home directory. Reads HOME / USERPROFILE on every call (Bun caches os.homedir()). */
export function home(): string {
  return (IS_WIN ? process.env.USERPROFILE : process.env.HOME) || os.homedir();
}

export function expandHome(p: string): string {
  if (p === '~') return home();
  if (p.startsWith('~/') || p.startsWith('~\\')) return path.join(home(), p.slice(2));
  return p;
}

export function lstatSafe(p: string): fs.Stats | null {
  try {
    return fs.lstatSync(p);
  } catch {
    return null;
  }
}

export function statSafe(p: string): fs.Stats | null {
  try {
    return fs.statSync(p);
  } catch {
    return null;
  }
}

export function readJson<T>(p: string, fallback: T): T {
  try {
    return JSON.parse(fs.readFileSync(p, 'utf8')) as T;
  } catch {
    return fallback;
  }
}

/**
 * Write via temp file + rename so readers never see a half-written file.
 * A symlinked target (dotfiles managers) is written through, not replaced.
 */
export function writeFileAtomic(file: string, data: string, mode?: number): void {
  let p = file;
  try {
    p = fs.realpathSync(file);
  } catch {
    // does not exist yet
  }
  fs.mkdirSync(path.dirname(p), { recursive: true });
  const tmp = `${p}.${process.pid}.${Date.now()}.tmp`;
  fs.writeFileSync(tmp, data, mode ? { mode } : undefined);
  try {
    fs.renameSync(tmp, p);
  } catch (err) {
    fs.rmSync(tmp, { force: true });
    throw err;
  }
}

/** JSON file contents, null if missing; throws on unreadable / invalid JSON. */
export function readJsonStrict<T>(p: string): T | null {
  let text: string;
  try {
    text = fs.readFileSync(p, 'utf8');
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === 'ENOENT') return null;
    throw err;
  }
  try {
    return JSON.parse(text) as T;
  } catch (err) {
    throw new Error(`${p} is not valid JSON (${(err as Error).message}). Fix it, or move it away to start over.`);
  }
}

/** `child` is `parent` or somewhere below it. */
export function isWithin(child: string, parent: string): boolean {
  const rel = path.relative(path.resolve(parent), path.resolve(child));
  return rel === '' || (!rel.startsWith('..') && !path.isAbsolute(rel));
}

export function writeJsonAtomic(p: string, obj: unknown, mode?: number): void {
  writeFileAtomic(p, `${JSON.stringify(obj, null, 2)}\n`, mode);
}

/** Resolve a command on PATH (PATHEXT-aware on Windows). */
export function which(cmd: string): string | null {
  if (!cmd) return null;
  if (cmd.includes('/') || (IS_WIN && cmd.includes('\\'))) return statSafe(cmd) ? cmd : null;
  return Bun.which(cmd, { PATH: process.env.PATH ?? '' });
}

export function fmtDuration(ms: number): string {
  if (ms <= 0) return 'now';
  const m = Math.round(ms / 60000);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const rest = m % 60;
  if (h < 48) return rest ? `${h}h${rest}m` : `${h}h`;
  return `${Math.round(h / 24)}d`;
}

/** Accepts epoch seconds, epoch ms or an ISO string. */
export function toDate(v: unknown): Date | null {
  if (v === null || v === undefined || v === '') return null;
  if (typeof v === 'number') return new Date(v < 1e12 ? v * 1000 : v);
  const d = new Date(String(v));
  return Number.isNaN(d.getTime()) ? null : d;
}

type Paint = (s: unknown) => string;
const useColor = () => !process.env.NO_COLOR && Boolean(process.stderr.isTTY);
const paint =
  (code: string): Paint =>
  (s) =>
    useColor() ? `\x1b[${code}m${s}\x1b[0m` : String(s);
export const c = {
  bold: paint('1'),
  dim: paint('2'),
  red: paint('31'),
  green: paint('32'),
  yellow: paint('33'),
  cyan: paint('36'),
};
export type Palette = typeof c;

export function info(msg: string): void {
  process.stderr.write(`${c.cyan('aims:')} ${msg}\n`);
}

export function warn(msg: string): void {
  process.stderr.write(`${c.yellow('aims:')} ${msg}\n`);
}

export class UsageError extends Error {}

type FlagSpec = Record<string, 'string' | 'bool'>;
type Flags<S extends FlagSpec> = { [K in keyof S]?: S[K] extends 'string' ? string : boolean };

/** Minimal flag parser: --flag, --key value, --key=value. Stops at "--". */
export function parseFlags<S extends FlagSpec>(
  argv: string[],
  spec: S = {} as S,
): { flags: Flags<S>; positional: string[]; rest: string[] } {
  const flags: Record<string, string | boolean> = {};
  const positional: string[] = [];
  const rest: string[] = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]!;
    if (a === '--') {
      rest.push(...argv.slice(i + 1));
      break;
    }
    if (a.startsWith('--')) {
      const eq = a.indexOf('=');
      const key = eq > 0 ? a.slice(2, eq) : a.slice(2);
      if (spec[key] === 'string') {
        const val = eq > 0 ? a.slice(eq + 1) : argv[++i];
        if (val === undefined) throw new UsageError(`--${key} needs a value`);
        flags[key] = val;
      } else if (spec[key] === 'bool') {
        flags[key] = true;
      } else {
        throw new UsageError(`unknown option --${key}`);
      }
    } else {
      positional.push(a);
    }
  }
  return { flags: flags as Flags<S>, positional, rest };
}

const ANSI = /\x1b\[[0-9;]*m/g;

export function table(rows: string[][]): string {
  if (!rows.length) return '';
  const width = (s: string) => s.replace(ANSI, '').length;
  const widths = rows[0]!.map((_, i) => Math.max(...rows.map((r) => width(r[i] ?? ''))));
  return rows
    .map((r) =>
      r
        .map((cell, i) => (i === r.length - 1 ? cell : cell + ' '.repeat(widths[i]! - width(cell))))
        .join('  ')
        .trimEnd(),
    )
    .join('\n');
}

/**
 * Read piped stdin. Resolves null for a TTY, or when nothing arrives within
 * `idleMs` (an inherited pipe that never closes must not hang us).
 * With untilEnd=false it also stops `idleMs` after the last chunk.
 */
export function readStdin(opts: { idleMs?: number; untilEnd?: boolean; onIdle?: () => void } = {}): Promise<Buffer | null> {
  const { idleMs = 3000, untilEnd = true, onIdle } = opts;
  return new Promise((resolve) => {
    const stdin = process.stdin;
    if (stdin.isTTY) return resolve(null);
    const chunks: Buffer[] = [];
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      stdin.removeAllListeners('data');
      stdin.removeAllListeners('end');
      stdin.removeAllListeners('error');
      stdin.pause();
      stdin.unref?.();
      resolve(chunks.length ? Buffer.concat(chunks) : null);
    };
    let timer = setTimeout(() => {
      if (!chunks.length) onIdle?.();
      finish();
    }, idleMs);
    stdin.on('data', (d: Buffer | string) => {
      clearTimeout(timer);
      // untilEnd: once input started, wait for EOF (a slow producer must not be cut off)
      if (!untilEnd) timer = setTimeout(finish, idleMs);
      chunks.push(typeof d === 'string' ? Buffer.from(d) : d);
    });
    stdin.on('end', finish);
    stdin.on('error', finish);
  });
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
