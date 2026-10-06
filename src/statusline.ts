import { getProfileState, loadConfig, loadState, profileForHomeEnv } from './config.ts';
import { pickProfile, recordUsage } from './health.ts';
import { runSync } from './proc.ts';
import type { ProfileState, UsageWindow } from './types.ts';
import { readStdin, toDate } from './util.ts';

interface StatusWindow {
  used_percentage?: number;
  utilization?: number;
  used_percent?: number;
  remaining?: number;
  limit?: number;
  resets_at?: number | string;
}

interface StatusInput {
  rate_limits?: { five_hour?: StatusWindow; seven_day?: StatusWindow };
}

/** Claude Code's status line JSON -> usage windows. */
export function windowsFromStatus(input: StatusInput | null | undefined): UsageWindow[] {
  const rl = input?.rate_limits;
  if (!rl) return [];
  const out: UsageWindow[] = [];
  for (const [key, label] of [
    ['five_hour', '5h'],
    ['seven_day', '7d'],
  ] as const) {
    const w = rl[key];
    if (!w) continue;
    let pct = w.used_percentage ?? w.utilization ?? w.used_percent;
    if (pct === undefined && typeof w.remaining === 'number' && typeof w.limit === 'number' && w.limit > 0) {
      pct = 100 - (w.remaining / w.limit) * 100;
    }
    if (typeof pct !== 'number') continue;
    out.push({ label, pct: Math.round(pct * 10) / 10, resetsAt: toDate(w.resets_at)?.toISOString() ?? null });
  }
  return out;
}

function changed(prev: ProfileState['usage'], next: UsageWindow[]): boolean {
  if (!prev?.windows) return true;
  if (Date.now() - Date.parse(prev.at ?? '') > 60000) return true;
  return next.some((w) => {
    const p = prev.windows.find((x) => x.label === w.label);
    return !p || Math.abs(p.pct - w.pct) >= 1 || p.resetsAt !== w.resetsAt;
  });
}

/**
 * `aims statusline` -- Claude Code's statusLine command. Shows which account
 * this session runs on and records its plan usage, so aims can fail over
 * *before* the limit is hit.
 */
export async function statusline(): Promise<void> {
  const raw = (await readStdin({ idleMs: 300, untilEnd: false }))?.toString('utf8') ?? '';
  let input: StatusInput = {};
  try {
    input = JSON.parse(raw || '{}');
  } catch {
    input = {};
  }
  const cfg = loadConfig();
  const name = profileForHomeEnv(cfg, 'claude', process.env.CLAUDE_CONFIG_DIR);
  const windows = windowsFromStatus(input);
  let ours = '';

  if (name) {
    if (windows.length && changed(getProfileState(loadState(), 'claude', name).usage, windows)) {
      try {
        recordUsage('claude', name, windows, 'statusline');
      } catch {
        // never break the status line over a state write
      }
    }
    ours = `⎇ ${name}`;
    const shown = windows.filter((w) => w.label === '5h' || w.pct >= 50);
    if (shown.length) ours += ` ${shown.map((w) => `${w.label} ${Math.round(w.pct)}%`).join(' ')}`;
    if (windows.some((w) => w.pct >= cfg.failover.threshold)) {
      const next = pickProfile(cfg, loadState(), 'claude', null, { exclude: [name] });
      if (next.name) ours += ` → aims failover claude (${next.name})`;
    }
  }

  let chained = '';
  if (cfg.statusline.chain) {
    const shell = process.platform === 'win32' ? ['cmd.exe', '/d', '/s', '/c'] : ['/bin/sh', '-c'];
    const r = runSync(shell[0]!, [...shell.slice(1), cfg.statusline.chain], { input: raw, timeoutMs: 3000 });
    chained = r.stdout.replace(/\s+$/, '');
  }
  process.stdout.write(`${[chained, ours].filter(Boolean).join('  ')}\n`);
}
