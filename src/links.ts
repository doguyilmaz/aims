import fs from 'node:fs';
import path from 'node:path';
import type { LinkReport } from './types.ts';
import { IS_WIN, lstatSafe, statSafe } from './util.ts';

/**
 * Keep a profile directory's shared entries pointing at the hub.
 *
 * Tools sometimes replace a symlink with a real file (atomic "write temp +
 * rename"), or create an entry before aims linked it. Every run repairs that:
 * newer content is moved into the hub and the link is restored, so nothing
 * written under one account is lost or hidden from the others.
 */

function listNames(dir: string): string[] {
  try {
    return fs.readdirSync(dir);
  } catch {
    return [];
  }
}

/** Expand a tool's shared spec (strings + regexes) against what exists on disk. */
export function sharedNames(spec: Array<string | RegExp>, hub: string, dir: string, exclude: string[] = []): string[] {
  const names = new Set<string>();
  const present = [...listNames(hub), ...listNames(dir)];
  for (const s of spec) {
    if (typeof s === 'string') names.add(s);
    else for (const n of present) if (s.test(n)) names.add(n);
  }
  for (const n of exclude) names.delete(n);
  return [...names];
}

function sameTarget(linkPath: string, target: string): boolean {
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

function makeLink(target: string, linkPath: string, isDir: boolean): void {
  if (isDir) {
    // Junctions need no admin rights on Windows.
    fs.symlinkSync(target, linkPath, IS_WIN ? 'junction' : 'dir');
    return;
  }
  try {
    fs.symlinkSync(target, linkPath, 'file');
  } catch (err) {
    if (!IS_WIN) throw err;
    fs.linkSync(target, linkPath); // file symlinks need Developer Mode on Windows
  }
}

function moveSync(from: string, to: string): void {
  try {
    fs.renameSync(from, to);
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== 'EXDEV') throw err;
    fs.cpSync(from, to, { recursive: true, preserveTimestamps: true });
    fs.rmSync(from, { recursive: true, force: true });
  }
}

const stamp = () => new Date().toISOString().replace(/[:.]/g, '-');

function filesEqual(a: string, b: string): boolean {
  try {
    if (fs.statSync(a).size !== fs.statSync(b).size) return false;
    return fs.readFileSync(a).equals(fs.readFileSync(b));
  } catch {
    return false;
  }
}

/** Move the contents of `src` into `dst` without overwriting; returns names of kept duplicates. */
function mergeDir(src: string, dst: string, label: string): string[] {
  const conflicts: string[] = [];
  fs.mkdirSync(dst, { recursive: true });
  for (const name of listNames(src)) {
    const s = path.join(src, name);
    const d = path.join(dst, name);
    const sst = lstatSafe(s);
    const dstat = lstatSafe(d);
    if (!sst) continue;
    if (!dstat) {
      moveSync(s, d);
    } else if (sst.isDirectory() && !sst.isSymbolicLink() && dstat.isDirectory()) {
      conflicts.push(...mergeDir(s, d, label));
    } else if (sst.isFile() && dstat.isFile() && filesEqual(s, d)) {
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

const SQLITE_SIDECARS = ['-wal', '-shm', '-journal'];

export interface EnsureLinksOptions {
  hub: string;
  dir: string;
  spec: Array<string | RegExp>;
  sharedDirs?: string[];
  exclude?: string[];
  /** Used in names of kept duplicates. */
  label?: string;
  /** Also adopt databases that look open. */
  fix?: boolean;
}

export function ensureLinks(o: EnsureLinksOptions): LinkReport[] {
  const { hub, dir, spec, sharedDirs = [], exclude = [], label = 'profile', fix = false } = o;
  const report: LinkReport[] = [];
  if (path.resolve(hub) === path.resolve(dir)) return report;
  fs.mkdirSync(dir, { recursive: true });
  fs.mkdirSync(hub, { recursive: true });

  for (const name of sharedNames(spec, hub, dir, exclude)) {
    const target = path.join(hub, name);
    const link = path.join(dir, name);
    const lst = lstatSafe(link);
    const hst = statSafe(target);
    try {
      if (lst?.isSymbolicLink()) {
        if (sameTarget(link, target) || (!statSafe(link) && !hst)) {
          report.push({ name, action: 'ok' }); // dangling is harmless until the hub entry exists
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

      if (name.endsWith('.sqlite')) {
        // Never move a database that may be open. Only adopt a cleanly closed one.
        const busy = SQLITE_SIDECARS.some((s) => (statSafe(link + s)?.size ?? 0) > 0);
        if (!hst && (!busy || fix)) {
          moveSync(link, target);
          for (const s of SQLITE_SIDECARS) if (lstatSafe(link + s)) moveSync(link + s, target + s);
          makeLink(target, link, false);
          report.push({ name, action: 'adopted' });
        } else {
          report.push({
            name,
            action: 'conflict',
            quiet: Boolean(hst),
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
        // Append-only logs (prompt history): keep both.
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
      report.push({ name, action: 'conflict', detail: (err as Error).message });
    }
  }
  return report;
}

/**
 * Undo sharing for `names` (all shared entries when null): the link is
 * replaced by a private copy, or by nothing for conversation data (`history`).
 */
export function detachLinks(o: { hub: string; dir: string; names: string[] | null; history: Array<string | RegExp> }): LinkReport[] {
  const report: LinkReport[] = [];
  let hubReal: string;
  try {
    hubReal = fs.realpathSync(o.hub);
  } catch {
    return report;
  }
  const isHistory = (n: string) => o.history.some((h) => (typeof h === 'string' ? h === n : h.test(n)));
  for (const name of listNames(o.dir)) {
    if (o.names && !o.names.includes(name)) continue;
    const link = path.join(o.dir, name);
    if (!lstatSafe(link)?.isSymbolicLink()) continue;
    // aims links to <hub>/<name>; that entry may itself be a symlink (dotfiles), so
    // compare the link's own target, not its fully resolved path.
    const target = path.resolve(o.dir, fs.readlinkSync(link));
    const ours = [path.resolve(o.hub), hubReal].includes(path.dirname(target).replace(/^\\\\\?\\/, ''));
    if (!ours) continue;
    try {
      fs.unlinkSync(link);
    } catch {
      fs.rmdirSync(link); // Windows junction
    }
    const copy = !isHistory(name) && statSafe(target);
    if (copy) fs.cpSync(target, link, { recursive: true, preserveTimestamps: true, dereference: true });
    report.push({ name, action: 'detached', detail: copy ? 'now a private copy' : 'now private and empty' });
  }
  return report;
}

/** Remove the links aims created (never follows them into the hub). */
export function removeLinks(dir: string): void {
  for (const name of listNames(dir)) {
    const p = path.join(dir, name);
    if (!lstatSafe(p)?.isSymbolicLink()) continue;
    try {
      fs.unlinkSync(p);
    } catch {
      fs.rmdirSync(p); // Windows junction
    }
  }
}
