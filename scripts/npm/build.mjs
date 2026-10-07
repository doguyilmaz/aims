// Builds the npm packages from GoReleaser's output and optionally publishes them.
//
//   node scripts/npm/build.mjs v1.2.3 [--publish]
//
// @doguyilmaz/aims is a small launcher; the binary for each platform is in its
// own package (@doguyilmaz/aims-darwin-arm64 and so on), which npm installs
// only on matching machines through optionalDependencies.
import { execFileSync } from 'node:child_process';
import { chmodSync, copyFileSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const [tag, flag] = process.argv.slice(2);
if (!/^v\d+\.\d+\.\d+(-[\w.]+)?$/.test(tag ?? '')) {
  console.error('usage: node scripts/npm/build.mjs vX.Y.Z [--publish]');
  process.exit(2);
}
const version = tag.slice(1);
const out = join(root, 'dist', 'npm');
const scope = '@doguyilmaz';
const meta = {
  version,
  license: 'MIT',
  homepage: 'https://doguyilmaz.github.io/aims/',
  repository: { type: 'git', url: 'git+https://github.com/doguyilmaz/aims.git' },
};

// GoReleaser's names for a platform -> Node's.
const nodeOS = { darwin: 'darwin', linux: 'linux', windows: 'win32' };
const nodeCPU = { amd64: 'x64', arm64: 'arm64' };

const artifacts = JSON.parse(readFileSync(join(root, 'dist', 'artifacts.json'), 'utf8'));
const binaries = artifacts.filter((a) => a.type === 'Binary' && a.extra?.ID === 'aims');
if (binaries.length === 0) throw new Error('no binaries in dist/artifacts.json; run goreleaser first');

rmSync(out, { recursive: true, force: true });
const platforms = [];
for (const b of binaries) {
  const os = nodeOS[b.goos];
  const cpu = nodeCPU[b.goarch];
  if (!os || !cpu) continue;
  const name = `${scope}/aims-${os}-${cpu}`;
  const dir = join(out, `aims-${os}-${cpu}`);
  const exe = os === 'win32' ? 'aims.exe' : 'aims';
  mkdirSync(join(dir, 'bin'), { recursive: true });
  copyFileSync(join(root, b.path), join(dir, 'bin', exe));
  chmodSync(join(dir, 'bin', exe), 0o755);
  writeFileSync(
    join(dir, 'package.json'),
    JSON.stringify({ name, description: `The aims binary for ${os} ${cpu}.`, ...meta, os: [os], cpu: [cpu], files: ['bin'] }, null, 2) + '\n',
  );
  writeFileSync(join(dir, 'README.md'), `The aims binary for ${os} ${cpu}. Install [${scope}/aims](https://www.npmjs.com/package/${scope}/aims) instead.\n`);
  platforms.push({ name, dir });
}

const main = join(out, 'aims');
mkdirSync(join(main, 'bin'), { recursive: true });
copyFileSync(join(root, 'scripts', 'npm', 'aims.js'), join(main, 'bin', 'aims.js'));
chmodSync(join(main, 'bin', 'aims.js'), 0o755);
// npm shows the README outside the repository: point relative images at the tag.
writeFileSync(
  join(main, 'README.md'),
  readFileSync(join(root, 'README.md'), 'utf8').replaceAll('src="assets/', `src="https://raw.githubusercontent.com/doguyilmaz/aims/${tag}/assets/`),
);
copyFileSync(join(root, 'LICENSE'), join(main, 'LICENSE'));
writeFileSync(
  join(main, 'package.json'),
  JSON.stringify(
    {
      name: `${scope}/aims`,
      description: 'Several Claude Code and Codex accounts on one machine: instant switching, shared history, failover.',
      ...meta,
      keywords: ['claude', 'claude-code', 'codex', 'accounts', 'profiles', 'cli'],
      bin: { aims: 'bin/aims.js' },
      files: ['bin'],
      engines: { node: '>=18' },
      optionalDependencies: Object.fromEntries(platforms.map((p) => [p.name, version])),
    },
    null,
    2,
  ) + '\n',
);
console.log(`built ${platforms.length + 1} packages in ${out}`);

if (flag === '--publish') {
  const args = ['publish', '--access', 'public'];
  if (process.env.GITHUB_ACTIONS) args.push('--provenance');
  if (version.includes('-')) args.push('--tag', 'next');
  // Platform packages first, so the launcher never points at missing versions.
  for (const p of [...platforms, { name: `${scope}/aims`, dir: main }]) {
    console.log(`publishing ${p.name}@${version}`);
    execFileSync('npm', args, { cwd: p.dir, stdio: 'inherit' });
  }
}
