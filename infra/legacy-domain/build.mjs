import { mkdir, readFile, writeFile, copyFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const output = `${root}.cloudflare/output/v0`;
const config = JSON.parse(await readFile(`${root}worker.json`, 'utf8'));
await mkdir(`${output}/workers/default/bundle`, { recursive: true });
await copyFile(`${root}index.mjs`, `${output}/workers/default/bundle/index.mjs`);
await writeFile(`${output}/config.json`, JSON.stringify({ buildContext: { mode: 'production', isPreview: false } }));
await writeFile(`${output}/workers/default/worker.config.json`, JSON.stringify({
  ...config,
  manifest: { type: 'complete', mainModule: 'index.mjs', modules: { 'index.mjs': { type: 'esm' } } },
}));
