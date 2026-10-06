import type { ToolStatus } from './ops.ts';
import type { UsageWindow } from './types.ts';
import { type Palette, c as color, fmtDuration, home, table, toDate } from './util.ts';

const plain: Palette = { bold: String, dim: String, red: String, green: String, yellow: String, cyan: String };

function tildify(p: string | null): string | null {
  const h = home();
  return p && p.startsWith(h) ? `~${p.slice(h.length)}` : p;
}

export function formatUsage(windows: UsageWindow[]): string {
  return windows
    .map((w) => {
      const r = toDate(w.resetsAt);
      const soon = r && r.getTime() > Date.now() && w.pct >= 80 ? ` (resets ${fmtDuration(r.getTime() - Date.now())})` : '';
      return `${w.label} ${Math.round(w.pct)}%${soon}`;
    })
    .join(' · ');
}

export function formatStatus(report: ToolStatus[], o: { color?: boolean } = {}): string {
  const c = o.color === false ? plain : color;
  const blocks: string[] = [];
  for (const t of report) {
    const head = `${c.bold(t.title)}  ${c.dim(`shared: ${tildify(t.hub) ?? '(not set up)'}`)}`;
    if (!t.profiles.length) {
      blocks.push(`${head}\n  ${c.dim(`no profiles yet -> aims add ${t.tool} <name> [--existing]`)}`);
      continue;
    }
    const rows = t.profiles.map((p) => {
      const mark = p.active ? c.green('*') : ' ';
      const tags = [p.existing && 'default login', p.isolated && 'isolated', p.pinned && 'pinned here']
        .filter(Boolean)
        .join(', ');
      const fallback =
        p.loggedIn === false
          ? 'not logged in'
          : p.method === 'api-key' || p.method === 'env'
            ? 'API key / token'
            : 'unknown account';
      const account = [p.email ?? c.dim(fallback), p.plan].filter(Boolean).join(' · ');
      const state = p.usable ? c.green(p.loggedIn === null ? 'ok?' : 'ok') : c.yellow(p.reasons.join(', '));
      return [`  ${mark} ${p.name}`, account + (tags ? c.dim(` (${tags})`) : ''), state, formatUsage(p.usage)];
    });
    blocks.push(`${head}\n${table(rows)}`);
  }
  return blocks.join('\n\n');
}
