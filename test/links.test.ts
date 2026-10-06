import { afterEach, beforeEach, expect, test } from 'bun:test';
import fs from 'node:fs';
import path from 'node:path';
import { type EnsureLinksOptions, ensureLinks, removeLinks } from '../src/links.ts';
import { TOOLS } from '../src/tools.ts';
import type { LinkReport } from '../src/types.ts';
import { type Sandbox, read, sandbox, write } from './helpers.ts';

let sb: Sandbox;
let hub: string;
let dir: string;
beforeEach(() => {
  sb = sandbox();
  hub = path.join(sb.root, 'hub');
  dir = path.join(sb.root, 'profile');
  fs.mkdirSync(hub);
});
afterEach(() => sb.restore());

const run = (extra: Partial<EnsureLinksOptions> = {}) =>
  ensureLinks({ hub, dir, spec: TOOLS.claude.shared, sharedDirs: TOOLS.claude.sharedDirs, label: 'work', ...extra });
const byName = (report: LinkReport[]) => Object.fromEntries(report.map((r) => [r.name, r]));
const isLink = (p: string) => fs.lstatSync(p).isSymbolicLink();

test('links existing hub entries and creates shared dirs', () => {
  write(path.join(hub, 'settings.json'), { a: 1 });
  write(path.join(hub, 'projects', 'p', 's.jsonl'), '{}');
  const r = byName(run());
  expect(r['settings.json']?.action).toBe('linked');
  expect(r.projects?.action).toBe('linked');
  expect(r.skills?.action).toBe('linked');
  expect(fs.existsSync(path.join(hub, 'skills'))).toBe(true);
  expect(r['CLAUDE.md']?.action).toBe('skipped');
  expect(isLink(path.join(dir, 'projects'))).toBe(true);
  expect(read(path.join(dir, 'projects', 'p', 's.jsonl'))).toBe('{}');
  // second run is a no-op
  expect(run().every((x) => x.action === 'ok' || x.action === 'skipped')).toBe(true);
});

test('never links credentials', () => {
  write(path.join(hub, '.credentials.json'), '{}');
  run();
  expect(fs.existsSync(path.join(dir, '.credentials.json'))).toBe(false);
});

test('adopts a file the tool created before it was linked', () => {
  fs.mkdirSync(dir);
  write(path.join(dir, 'CLAUDE.md'), 'mine');
  expect(byName(run())['CLAUDE.md']?.action).toBe('adopted');
  expect(isLink(path.join(dir, 'CLAUDE.md'))).toBe(true);
  expect(read(path.join(hub, 'CLAUDE.md'))).toBe('mine');
});

test('merges a real directory into the hub, keeping both versions of a conflict', () => {
  write(path.join(hub, 'projects', 'p', 'same.jsonl'), 'hub');
  write(path.join(hub, 'projects', 'p', 'equal.jsonl'), 'x');
  run();
  fs.unlinkSync(path.join(dir, 'projects'));
  write(path.join(dir, 'projects', 'p', 'new.jsonl'), 'new');
  write(path.join(dir, 'projects', 'p', 'same.jsonl'), 'profile');
  write(path.join(dir, 'projects', 'p', 'equal.jsonl'), 'x');
  expect(byName(run()).projects?.action).toBe('merged');
  expect(isLink(path.join(dir, 'projects'))).toBe(true);
  const files = fs.readdirSync(path.join(hub, 'projects', 'p'));
  expect(files).toContain('new.jsonl');
  expect(read(path.join(hub, 'projects', 'p', 'same.jsonl'))).toBe('hub');
  expect(files.some((f) => f.startsWith('same.jsonl.aims-work-'))).toBe(true);
  expect(files.filter((f) => f.startsWith('equal.jsonl'))).toHaveLength(1);
});

test('repairs a link replaced by an atomic write: newer content wins, other kept', () => {
  write(path.join(hub, 'settings.json'), { v: 'old' });
  run();
  fs.unlinkSync(path.join(dir, 'settings.json'));
  write(path.join(dir, 'settings.json'), { v: 'new' });
  const past = new Date(Date.now() - 60000);
  fs.utimesSync(path.join(hub, 'settings.json'), past, past);
  expect(byName(run())['settings.json']?.action).toBe('merged');
  expect(JSON.parse(read(path.join(hub, 'settings.json')))).toEqual({ v: 'new' });
  expect(fs.readdirSync(hub).some((f) => f.startsWith('settings.json.aims-previous-'))).toBe(true);
  expect(isLink(path.join(dir, 'settings.json'))).toBe(true);
});

test('appends prompt history instead of choosing one copy', () => {
  write(path.join(hub, 'history.jsonl'), 'a\n');
  fs.mkdirSync(dir);
  write(path.join(dir, 'history.jsonl'), 'b\n');
  run();
  expect(read(path.join(hub, 'history.jsonl'))).toBe('a\nb\n');
});

test('sqlite: adopts a closed database, refuses an open one', () => {
  const spec = TOOLS.codex.shared;
  fs.mkdirSync(dir);
  write(path.join(dir, 'state_5.sqlite'), 'db');
  write(path.join(dir, 'state_5.sqlite-wal'), 'pending');
  expect(byName(ensureLinks({ hub, dir, spec }))['state_5.sqlite']?.action).toBe('conflict');
  expect(fs.existsSync(path.join(hub, 'state_5.sqlite'))).toBe(false);
  expect(byName(ensureLinks({ hub, dir, spec, fix: true }))['state_5.sqlite']?.action).toBe('adopted');
  expect(read(path.join(hub, 'state_5.sqlite-wal'))).toBe('pending');
  expect(isLink(path.join(dir, 'state_5.sqlite'))).toBe(true);
  // the logs database stays private to the profile
  write(path.join(dir, 'logs_2.sqlite'), 'log');
  expect(byName(ensureLinks({ hub, dir, spec }))['logs_2.sqlite']).toBeUndefined();
});

test('exclude keeps an entry per profile', () => {
  write(path.join(hub, 'settings.json'), {});
  expect(byName(run({ exclude: ['settings.json'] }))['settings.json']).toBeUndefined();
  expect(fs.existsSync(path.join(dir, 'settings.json'))).toBe(false);
});

test('removeLinks leaves hub data intact', () => {
  write(path.join(hub, 'projects', 'p', 's.jsonl'), 'keep');
  run();
  removeLinks(dir);
  fs.rmSync(dir, { recursive: true, force: true });
  expect(read(path.join(hub, 'projects', 'p', 's.jsonl'))).toBe('keep');
});
