import fs from 'node:fs';
import path from 'node:path';
import { IS_WIN, lstatSafe, statSafe } from './util.js';

/**
 * Keep a profile directory's shared entries pointing at the hub.
 *
 * Tools sometimes replace a symlink with a real file (atomic "write temp +
 * rename"), or create an entry before aims linked it. Every run repairs that:
 * newer content is moved into the hub and the link is restored, so nothing
 * written under one account is lost or hidden from the others.
 */

function listNames(dir) {
  try {
    return fs.readdirSync(dir);
  } catch {
    return [];
  }
}

/** Expand a tool's shared spec (strings + regexes) against what exists on disk. */
export function sharedNames(spec, hub, dir, exclude = []) {
  const names = new Set();
  const present = [...listNames(hub), ...listNames(dir)];
  for (const s of spec) {
    if (typeof s === 'string') names.add(s);
    else for (const n of present) if (s.test(n)) names.add(n);
  }
  for (const n of exclude) names.delete(n);
  return [...names];
}

function sameTarget(linkPath, target) {
  try {
    return fs.realpathSync(linkPath) === fs.realpathSync(target);
  } catch {
    try {
      return path.resolve(path.dirname(linkPath), fs.readlinkSync(linkPath)) === path.resolve(target);
    } catch {
      return false;
    }
  }
}

function makeLink(target, linkPath, isDir) {
  if (isDir) {
    // Junctions need no admin rights on Windows.
    fs.symlinkSync(target, linkPath, IS_WIN ? 'junction' : 'dir');
    return 'symlink';
  }
  try {
    fs.symlinkSync(target, linkPath, 'file');
    return 'symlink';
  } catch (err) {
    if (!IS_WIN) throw err;
    fs.linkSync(target, linkPath); // file symlinks need Developer Mode on Windows
    return 'hardlink';
  }
}

function moveSync(from, to) {
  try {
    fs.renameSync(from, to);
  } catch (err) {
    if (err.code !== 'EXDEV') throw err;
    fs.cpSync(from, to, { recursive: true, preserveTimestamps: true });
    fs.rmSync(from, { recursive: true, force: true });
  }
}

function stamp() {
  return new Date().toISOString().replace(/[:.]/g, '-');
}

/** Move the contents of `src` into `dst` without overwriting; returns conflict names. */
function mergeDir(src, dst, label) {
  const conflicts = [];
  fs.mkdirSync(dst, { recursive: true });
  for (const name of listNames(src)) {
    const s = path.join(src, name);
    const d = path.join(dst, name);
    const sst = lstatSafe(s);
    const dst2 = lstatSafe(d);
    if (!dst2) {
      moveSync(s, d);
    } else if (sst.isDirectory() && !sst.isSymbolicLink() && dst2.isDirectory()) {
      conflicts.push(...mergeDir(s, d, label));
    } else if (sst.isFile() && dst2.isFile() && filesEqual(s, d)) {
      fs.rmSync(s, { force: true });
    } else {
      const alt = `${d}.aims-${label}-${stamp()}`;
      moveSync(s, alt);
      conflicts.push(path.basename(alt));
    }
  }
  fs.rmSync(src, { recursive: true, force: true });
  return conflicts;
}

function filesEqual(a, b) {
  try {
    const sa = fs.statSync(a);
    const sb = fs.statSync(b);
    if (sa.size !== sb.size) return false;
    return fs.readFileSync(a).equals(fs.readFileSync(b));
  } catch {
    return false;
  }
}

const SQLITE_SIDECARS = ['-wal', '-shm', '-journal'];

/**
 * @returns {Array<{name: string, action: string, detail?: string}>}
 *   action: ok | linked | adopted | merged | conflict | skipped
 */
export function ensureLinks({ hub, dir, spec, sharedDirs = [], exclude = [], label = 'profile', fix = false }) {
  const report = [];
  if (path.resolve(hub) === path.resolve(dir)) return report;
  fs.mkdirSync(dir, { recursive: true });
  fs.mkdirSync(hub, { recursive: true });

  for (const name of sharedNames(spec, hub, dir, exclude)) {
    const target = path.join(hub, name);
    const link = path.join(dir, name);
    const lst = lstatSafe(link);
    const hst = statSafe(target);
    const isSqlite = name.endsWith('.sqlite');
    try {
      if (lst && lst.isSymbolicLink()) {
        if (sameTarget(link, target)) {
          report.push({ name, action: 'ok' });
        } else if (!statSafe(link) && !hst) {
          report.push({ name, action: 'ok' }); // dangling but harmless until the hub entry exists
        } else {
          report.push({ name, action: 'conflict', detail: `points to ${fs.readlinkSync(link)}, not the hub` });
        }
        continue;
      }

      if (!lst) {
        if (hst) {
          makeLink(target, link, hst.isDirectory());
          report.push({ name, action: 'linked' });
        } else if (sharedDirs.includes(name)) {
          fs.mkdirSync(target, { recursive: true });
          makeLink(target, link, true);
          report.push({ name, action: 'linked' });
        } else {
          report.push({ name, action: 'skipped', detail: 'not created yet' });
        }
        continue;
      }

      // Hard link created on Windows: fine while it is the same file.
      if (hst && lst.isFile() && hst.isFile() && lst.ino && lst.ino === hst.ino && lst.dev === hst.dev) {
        report.push({ name, action: 'ok' });
        continue;
      }

      if (lst.isDirectory()) {
        if (!hst) {
          moveSync(link, target);
          makeLink(target, link, true);
          report.push({ name, action: 'adopted' });
        } else {
          const conflicts = mergeDir(link, target, label);
          makeLink(target, link, true);
          report.push({
            name,
            action: 'merged',
            detail: conflicts.length ? `kept both copies of: ${conflicts.join(', ')}` : undefined,
          });
        }
        continue;
      }

      if (isSqlite) {
        // Never move a database that may be open. Only adopt a cleanly closed one.
        const busy = SQLITE_SIDECARS.some((s) => {
          const st = statSafe(link + s);
          return st && st.size > 0;
        });
        if (!hst && (!busy || fix)) {
          moveSync(link, target);
          for (const s of SQLITE_SIDECARS) if (lstatSafe(link + s)) moveSync(link + s, target + s);
          makeLink(target, link, false);
          report.push({ name, action: 'adopted' });
        } else {
          report.push({
            name,
            action: 'conflict',
            quiet: Boolean(hst), // nothing to fix automatically; shown by doctor/sync only
            detail: hst
              ? 'profile and hub each have their own database; threads in this one stay profile-only'
              : 'database is in use; close the tool and run `aims doctor --fix`',
          });
        }
        continue;
      }

      // A regular file where the link should be.
      if (!hst) {
        moveSync(link, target);
        makeLink(target, link, false);
        report.push({ name, action: 'adopted' });
      } else if (filesEqual(link, target)) {
        fs.rmSync(link, { force: true });
        makeLink(target, link, false);
        report.push({ name, action: 'linked' });
      } else if (name.endsWith('.jsonl')) {
        // Append-only logs (prompt history): merge both.
        fs.appendFileSync(target, fs.readFileSync(link));
        fs.rmSync(link, { force: true });
        makeLink(target, link, false);
        report.push({ name, action: 'merged' });
      } else {
        // Newer copy wins; the other is kept next to it.
        const profileNewer = lst.mtimeMs > hst.mtimeMs;
        const backup = `${target}.aims-${profileNewer ? 'previous' : label}-${stamp()}`;
        if (profileNewer) {
          moveSync(target, backup);
          moveSync(link, target);
        } else {
          moveSync(link, backup);
        }
        makeLink(target, link, false);
        report.push({ name, action: 'merged', detail: `other version saved as ${path.basename(backup)}` });
      }
    } catch (err) {
      report.push({ name, action: 'conflict', detail: err.message });
    }
  }
  return report;
}

/** Remove the links aims created (never follows them into the hub). */
export function removeLinks(dir) {
  for (const name of listNames(dir)) {
    const p = path.join(dir, name);
    const st = lstatSafe(p);
    if (!st || !st.isSymbolicLink()) continue;
    try {
      fs.unlinkSync(p);
    } catch {
      fs.rmdirSync(p); // Windows junction
    }
  }
}
