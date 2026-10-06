import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { afterEach, beforeEach, test } from 'node:test';
import { ensureLinks, removeLinks } from '../src/links.js';
import { TOOLS } from '../src/tools.js';
import { sandbox, write } from './helpers.js';

let sb;
let hub;
let dir;
beforeEach(() => {
  sb = sandbox();
  hub = path.join(sb.root, 'hub');
  dir = path.join(sb.root, 'profile');
  fs.mkdirSync(hub);
});
afterEach(() => sb.restore());

const run = (extra = {}) =>
  ensureLinks({ hub, dir, spec: TOOLS.claude.shared, sharedDirs: TOOLS.claude.sharedDirs, label: 'work', ...extra });
const byName = (report) => Object.fromEntries(report.map((r) => [r.name, r]));
const isLink = (p) => fs.lstatSync(p).isSymbolicLink();

test('links existing hub entries and creates shared dirs', () => {
  write(path.join(hub, 'settings.json'), { a: 1 });
  write(path.join(hub, 'projects', 'p', 's.jsonl'), '{}');
  const r = byName(run());
  assert.equal(r['settings.json'].action, 'linked');
  assert.equal(r.projects.action, 'linked');
  assert.equal(r.skills.action, 'linked');
  assert.ok(fs.existsSync(path.join(hub, 'skills')), 'shared dir created in hub');
  assert.equal(r['CLAUDE.md'].action, 'skipped');
  assert.ok(isLink(path.join(dir, 'projects')));
  assert.equal(fs.readFileSync(path.join(dir, 'projects', 'p', 's.jsonl'), 'utf8'), '{}');
  // second run is a no-op
  assert.ok(run().every((x) => ['ok', 'skipped'].includes(x.action)));
});

test('never links credentials', () => {
  write(path.join(hub, '.credentials.json'), '{}');
  run();
  assert.equal(fs.existsSync(path.join(dir, '.credentials.json')), false);
});

test('adopts a file the tool created before it was linked', () => {
  fs.mkdirSync(dir);
  write(path.join(dir, 'CLAUDE.md'), 'mine');
  assert.equal(byName(run())['CLAUDE.md'].action, 'adopted');
  assert.ok(isLink(path.join(dir, 'CLAUDE.md')));
  assert.equal(fs.readFileSync(path.join(hub, 'CLAUDE.md'), 'utf8'), 'mine');
});

test('merges a real directory into the hub, keeping both versions of a conflict', () => {
  write(path.join(hub, 'projects', 'p', 'same.jsonl'), 'hub');
  write(path.join(hub, 'projects', 'p', 'equal.jsonl'), 'x');
  run();
  fs.unlinkSync(path.join(dir, 'projects'));
  write(path.join(dir, 'projects', 'p', 'new.jsonl'), 'new');
  write(path.join(dir, 'projects', 'p', 'same.jsonl'), 'profile');
  write(path.join(dir, 'projects', 'p', 'equal.jsonl'), 'x');
  const r = byName(run());
  assert.equal(r.projects.action, 'merged');
  assert.ok(isLink(path.join(dir, 'projects')));
  const files = fs.readdirSync(path.join(hub, 'projects', 'p')).sort();
  assert.ok(files.includes('new.jsonl'));
  assert.equal(fs.readFileSync(path.join(hub, 'projects', 'p', 'same.jsonl'), 'utf8'), 'hub');
  assert.ok(files.some((f) => f.startsWith('same.jsonl.aims-work-')), 'conflicting copy kept');
  assert.equal(files.filter((f) => f.startsWith('equal.jsonl')).length, 1, 'identical files deduplicated');
});

test('repairs a link replaced by an atomic write: newer content wins, other kept', () => {
  write(path.join(hub, 'settings.json'), { v: 'old' });
  run();
  fs.unlinkSync(path.join(dir, 'settings.json'));
  write(path.join(dir, 'settings.json'), { v: 'new' });
  const past = new Date(Date.now() - 60000);
  fs.utimesSync(path.join(hub, 'settings.json'), past, past);
  const r = byName(run());
  assert.equal(r['settings.json'].action, 'merged');
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(hub, 'settings.json'), 'utf8')), { v: 'new' });
  assert.ok(fs.readdirSync(hub).some((f) => f.startsWith('settings.json.aims-previous-')));
  assert.ok(isLink(path.join(dir, 'settings.json')));
});

test('appends prompt history instead of choosing one copy', () => {
  write(path.join(hub, 'history.jsonl'), 'a\n');
  fs.mkdirSync(dir);
  write(path.join(dir, 'history.jsonl'), 'b\n');
  run();
  assert.equal(fs.readFileSync(path.join(hub, 'history.jsonl'), 'utf8'), 'a\nb\n');
});

test('sqlite: adopts a closed database, refuses an open one', () => {
  const spec = TOOLS.codex.shared;
  fs.mkdirSync(dir);
  write(path.join(dir, 'state_5.sqlite'), 'db');
  write(path.join(dir, 'state_5.sqlite-wal'), 'pending');
  let r = byName(ensureLinks({ hub, dir, spec }));
  assert.equal(r['state_5.sqlite'].action, 'conflict');
  assert.equal(fs.existsSync(path.join(hub, 'state_5.sqlite')), false);
  r = byName(ensureLinks({ hub, dir, spec, fix: true }));
  assert.equal(r['state_5.sqlite'].action, 'adopted');
  assert.equal(fs.readFileSync(path.join(hub, 'state_5.sqlite-wal'), 'utf8'), 'pending');
  assert.ok(isLink(path.join(dir, 'state_5.sqlite')));
  // logs db is private to the profile
  write(path.join(dir, 'logs_2.sqlite'), 'log');
  assert.equal(byName(ensureLinks({ hub, dir, spec }))['logs_2.sqlite'], undefined);
});

test('exclude keeps an entry per profile', () => {
  write(path.join(hub, 'settings.json'), {});
  const r = byName(run({ exclude: ['settings.json'] }));
  assert.equal(r['settings.json'], undefined);
  assert.equal(fs.existsSync(path.join(dir, 'settings.json')), false);
});

test('removeLinks leaves hub data intact', () => {
  write(path.join(hub, 'projects', 'p', 's.jsonl'), 'keep');
  run();
  removeLinks(dir);
  fs.rmSync(dir, { recursive: true, force: true });
  assert.equal(fs.readFileSync(path.join(hub, 'projects', 'p', 's.jsonl'), 'utf8'), 'keep');
});
