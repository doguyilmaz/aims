// Builds the npm package from GoReleaser's output and optionally publishes it.
//
//   node scripts/npm/build.mjs v1.2.3 [--publish]
//
// @doguyilmaz/aims is one package: a small launcher and the binary for every
// supported platform, in bin/<platform>-<arch>/. One name on npm, no install
// scripts, and it works offline and with --ignore-scripts.
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
const name = '@doguyilmaz/aims';
const out = join(root, 'dist', 'npm', 'aims');

// GoReleaser's names for a platform -> Node's (process.platform, process.arch).
const nodeOS = { darwin: 'darwin', linux: 'linux', windows: 'win32' };
const nodeCPU = { amd64: 'x64', arm64: 'arm64' };

const artifacts = JSON.parse(readFileSync(join(root, 'dist', 'artifacts.json'), 'utf8'));
const binaries = artifacts.filter((a) => a.type === 'Binary' && a.extra?.ID === 'aims');
if (binaries.length === 0) throw new Error('no binaries in dist/artifacts.json; run goreleaser first');

rmSync(join(root, 'dist', 'npm'), { recursive: true, force: true });
const oses = new Set();
const cpus = new Set();
for (const b of binaries) {
  const os = nodeOS[b.goos];
  const cpu = nodeCPU[b.goarch];
  if (!os || !cpu) continue;
  const exe = os === 'win32' ? 'aims.exe' : 'aims';
  const dir = join(out, 'bin', `${os}-${cpu}`);
  mkdirSync(dir, { recursive: true });
  copyFileSync(join(root, b.path), join(dir, exe));
  chmodSync(join(dir, exe), 0o755);
  oses.add(os);
  cpus.add(cpu);
}

copyFileSync(join(root, 'scripts', 'npm', 'aims.js'), join(out, 'bin', 'aims.js'));
chmodSync(join(out, 'bin', 'aims.js'), 0o755);
// npm shows the README outside the repository: point relative images at the tag.
writeFileSync(
  join(out, 'README.md'),
  readFileSync(join(root, 'README.md'), 'utf8').replaceAll('src="assets/', `src="https://raw.githubusercontent.com/doguyilmaz/aims/${tag}/assets/`),
);
copyFileSync(join(root, 'LICENSE'), join(out, 'LICENSE'));
writeFileSync(
  join(out, 'package.json'),
  JSON.stringify(
    {
      name,
      version,
      description: 'Several Claude Code and Codex accounts on one machine: instant switching, shared history, failover.',
      license: 'MIT',
      homepage: 'https://doguyilmaz.github.io/aims/',
      repository: { type: 'git', url: 'git+https://github.com/doguyilmaz/aims.git' },
      keywords: ['claude', 'claude-code', 'codex', 'accounts', 'profiles', 'cli'],
      bin: { aims: 'bin/aims.js' },
      files: ['bin'],
      os: [...oses],
      cpu: [...cpus],
      engines: { node: '>=18' },
    },
    null,
    2,
  ) + '\n',
);
console.log(`built ${name}@${version} with ${binaries.length} binaries in ${out}`);

if (flag === '--publish') {
  // Publishing the same version twice fails; a re-run (of the release job, say) skips it.
  const published = (() => {
    try {
      execFileSync('npm', ['view', `${name}@${version}`, 'version'], { stdio: 'pipe' });
      return true;
    } catch {
      return false;
    }
  })();
  if (published) {
    console.log(`${name}@${version} is already on npm`);
  } else {
    const args = ['publish', '--access', 'public'];
    if (process.env.GITHUB_ACTIONS) args.push('--provenance');
    if (version.includes('-')) args.push('--tag', 'next');
    execFileSync('npm', args, { cwd: out, stdio: 'inherit' });
  }
}
