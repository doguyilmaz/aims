import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

export const IS_WIN = process.platform === 'win32';
export const IS_MAC = process.platform === 'darwin';

export function home() {
  return os.homedir();
}

export function expandHome(p) {
  if (!p) return p;
  if (p === '~') return home();
  if (p.startsWith('~/') || p.startsWith('~\\')) return path.join(home(), p.slice(2));
  return p;
}

export function lstatSafe(p) {
  try {
    return fs.lstatSync(p);
  } catch {
    return null;
  }
}

export function statSafe(p) {
  try {
    return fs.statSync(p);
  } catch {
    return null;
  }
}

export function readJson(p, fallback = null) {
  try {
    return JSON.parse(fs.readFileSync(p, 'utf8'));
  } catch {
    return fallback;
  }
}

/** Write via temp file + rename so readers never see a half-written file. */
export function writeFileAtomic(p, data, mode) {
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

export function writeJsonAtomic(p, obj, mode) {
  writeFileAtomic(p, `${JSON.stringify(obj, null, 2)}\n`, mode);
}

/** Resolve a command on PATH (honours PATHEXT on Windows). */
export function which(cmd) {
  if (!cmd) return null;
  if (cmd.includes('/') || (IS_WIN && cmd.includes('\\'))) return statSafe(cmd) ? cmd : null;
  const exts = IS_WIN ? (process.env.PATHEXT || '.EXE;.CMD;.BAT;.COM').split(';').filter(Boolean) : [''];
  for (const dir of (process.env.PATH || '').split(path.delimiter)) {
    if (!dir) continue;
    for (const ext of IS_WIN ? ['', ...exts] : exts) {
      const full = path.join(dir, cmd + ext);
      const st = statSafe(full);
      if (st && st.isFile()) {
        if (IS_WIN) {
          if (ext) return full;
          continue;
        }
        try {
          fs.accessSync(full, fs.constants.X_OK);
          return full;
        } catch {
          // not executable, keep looking
        }
      }
    }
  }
  return null;
}

export function fmtDuration(ms) {
  if (ms <= 0) return 'now';
  const m = Math.round(ms / 60000);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const rest = m % 60;
  if (h < 48) return rest ? `${h}h${rest}m` : `${h}h`;
  return `${Math.round(h / 24)}d`;
}

/** Accepts epoch seconds, epoch ms or an ISO string. */
export function toDate(v) {
  if (v === null || v === undefined || v === '') return null;
  if (typeof v === 'number') return new Date(v < 1e12 ? v * 1000 : v);
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
}

const useColor = () => !process.env.NO_COLOR && process.stderr.isTTY;
const paint = (code) => (s) => (useColor() ? `\x1b[${code}m${s}\x1b[0m` : String(s));
export const c = {
  bold: paint('1'),
  dim: paint('2'),
  red: paint('31'),
  green: paint('32'),
  yellow: paint('33'),
  cyan: paint('36'),
};

export function info(msg) {
  process.stderr.write(`${c.cyan('aims:')} ${msg}\n`);
}

export function warn(msg) {
  process.stderr.write(`${c.yellow('aims:')} ${msg}\n`);
}

export class UsageError extends Error {}

/** Minimal flag parser: --flag, --key value, --key=value. Stops at "--". */
export function parseFlags(argv, spec = {}) {
  const flags = {};
  const positional = [];
  const rest = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
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
  return { flags, positional, rest };
}

export function table(rows) {
  if (!rows.length) return '';
  const strip = (s) => String(s).replace(/\x1b\[[0-9;]*m/g, '');
  const widths = rows[0].map((_, i) => Math.max(...rows.map((r) => strip(r[i] ?? '').length)));
  return rows
    .map((r) =>
      r
        .map((cell, i) => {
          const s = String(cell ?? '');
          return i === r.length - 1 ? s : s + ' '.repeat(widths[i] - strip(s).length);
        })
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
export function readStdin({ idleMs = 3000, encoding = null, untilEnd = true, onIdle } = {}) {
  return new Promise((resolve) => {
    const stdin = process.stdin;
    if (stdin.isTTY) return resolve(null);
    const chunks = [];
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      stdin.removeAllListeners('data');
      stdin.removeAllListeners('end');
      stdin.removeAllListeners('error');
      stdin.pause();
      if (typeof stdin.unref === 'function') stdin.unref();
      const buf = Buffer.concat(chunks);
      resolve(chunks.length ? (encoding ? buf.toString(encoding) : buf) : null);
    };
    let timer = setTimeout(() => {
      if (!chunks.length && onIdle) onIdle();
      finish();
    }, idleMs);
    stdin.on('data', (d) => {
      clearTimeout(timer);
      // untilEnd: once input started, wait for EOF (a slow producer must not be cut off)
      if (!untilEnd) timer = setTimeout(finish, idleMs);
      chunks.push(Buffer.isBuffer(d) ? d : Buffer.from(d));
    });
    stdin.on('end', finish);
    stdin.on('error', finish);
  });
}
