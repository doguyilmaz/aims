#!/usr/bin/env node
import { main } from '../src/cli.js';

main(process.argv.slice(2)).then(
  (code) => {
    if (typeof code === 'number') process.exitCode = code;
  },
  (err) => {
    process.stderr.write(`aims: ${err && err.message ? err.message : err}\n`);
    if (process.env.AIMS_DEBUG) process.stderr.write(`${err && err.stack}\n`);
    process.exitCode = 1;
  },
);
