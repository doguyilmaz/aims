#!/usr/bin/env node
// Runs the aims binary for this platform, shipped next to this file.
'use strict';
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const key = `${process.platform}-${process.arch}`;
const bin = path.join(__dirname, key, process.platform === 'win32' ? 'aims.exe' : 'aims');
if (!fs.existsSync(bin)) {
  console.error(
    `aims: there is no build for ${key} in this package.\n` +
      'See https://doguyilmaz.github.io/aims/docs/installation/ for other ways to install.',
  );
  process.exit(1);
}
if (process.platform !== 'win32') {
  // Some installers drop the executable bit of files outside "bin" entries.
  try {
    fs.accessSync(bin, fs.constants.X_OK);
  } catch {
    try {
      fs.chmodSync(bin, 0o755);
    } catch {}
  }
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
