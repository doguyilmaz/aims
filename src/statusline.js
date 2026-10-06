import { spawnSync } from 'node:child_process';
import { getProfileState, loadConfig, loadState, profileForHomeEnv } from './config.js';
import { pickProfile, recordUsage } from './health.js';
import { readStdin, toDate } from './util.js';

/** Claude Code's status line JSON -> [{label, pct, resetsAt}] */
export function windowsFromStatus(input) {
  const rl = input && input.rate_limits;
  if (!rl) return [];
  const out = [];
  for (const [key, label] of [
    ['five_hour', '5h'],
    ['seven_day', '7d'],
  ]) {
    const w = rl[key];
    if (!w) continue;
    let pct = w.used_percentage ?? w.utilization ?? w.used_percent;
    if (pct === undefined && typeof w.remaining === 'number' && typeof w.limit === 'number' && w.limit > 0) {
      pct = 100 - (w.remaining / w.limit) * 100;
    }
    if (typeof pct !== 'number') continue;
    const resets = toDate(w.resets_at);
    out.push({ label, pct: Math.round(pct * 10) / 10, resetsAt: resets ? resets.toISOString() : null });
  }
  return out;
}

function changed(prev, next) {
  if (!prev || !prev.windows) return true;
  if (Date.now() - Date.parse(prev.at || 0) > 60000) return true;
  return next.some((w) => {
    const p = prev.windows.find((x) => x.label === w.label);
    return !p || Math.abs(p.pct - w.pct) >= 1 || p.resetsAt !== w.resetsAt;
  });
}

/**
 * `aims statusline` -- configured as Claude Code's statusLine command.
 * Shows which account this session runs on and records its plan usage so
 * aims can fail over *before* the limit is hit.
 */
export async function statusline() {
  const raw = (await readStdin({ idleMs: 300, encoding: 'utf8', untilEnd: false })) || '';
  let input = {};
  try {
    input = JSON.parse(raw || '{}');
  } catch {
    input = {};
  }
  const cfg = loadConfig();
  const name = profileForHomeEnv(cfg, 'claude', process.env.CLAUDE_CONFIG_DIR);
  const windows = windowsFromStatus(input);
  const parts = [];

  if (name) {
    const state = loadState();
    const prev = getProfileState(state, 'claude', name).usage;
    if (windows.length && changed(prev, windows)) {
      try {
        recordUsage('claude', name, windows, 'statusline');
      } catch {
        // never break the status line over a state write
      }
    }
    let seg = `⎇ ${name}`;
    const shown = windows.filter((w) => w.label === '5h' || w.pct >= 50);
    if (shown.length) seg += ` ${shown.map((w) => `${w.label} ${Math.round(w.pct)}%`).join(' ')}`;
    if (windows.some((w) => w.pct >= cfg.failover.threshold)) {
      const next = pickProfile(cfg, loadState(), 'claude', null, { exclude: [name] });
      if (next.name) seg += ` → aims failover claude (${next.name})`;
    }
    parts.push(seg);
  }

  let chained = '';
  if (cfg.statusline && cfg.statusline.chain) {
    const r = spawnSync(cfg.statusline.chain, {
      input: raw,
      shell: true,
      encoding: 'utf8',
      timeout: 3000,
    });
    chained = (r.stdout || '').replace(/\s+$/, '');
  }
  const ours = parts.join(' ');
  process.stdout.write([chained, ours].filter(Boolean).join('  ') + '\n');
}
