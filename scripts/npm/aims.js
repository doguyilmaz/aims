#!/usr/bin/env node
// Runs the aims binary from the platform package npm installed next to this one.
'use strict';
const { spawn } = require('node:child_process');

const key = `${process.platform}-${process.arch}`;
const exe = process.platform === 'win32' ? 'aims.exe' : 'aims';
let bin;
try {
  bin = require.resolve(`@doguyilmaz/aims-${key}/bin/${exe}`);
} catch {
  console.error(
    `aims: there is no build for ${key}, or optional dependencies were skipped.\n` +
      'See https://doguyilmaz.github.io/aims/docs/installation/ for other ways to install.',
  );
  process.exit(1);
}

const child = spawn(bin, process.argv.slice(2), { stdio: 'inherit' });
// Ctrl+C reaches aims directly (same process group); this process only waits.
// Signals sent to it alone are passed on.
for (const sig of ['SIGINT', 'SIGQUIT', 'SIGTERM', 'SIGHUP']) {
  process.on(sig, () => {
    if (sig === 'SIGTERM' || sig === 'SIGHUP') child.kill(sig);
  });
}
child.on('error', (err) => {
  console.error(`aims: ${err.message}`);
  process.exit(1);
});
child.on('exit', (code, signal) => {
  if (signal) {
    process.removeAllListeners(signal);
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});
