import readline from 'node:readline';
import { loadConfig } from './config.js';
import { liveCheck } from './health.js';
import { clear, failover, statusReport, useProfile } from './ops.js';
import { launch } from './run.js';
import { TOOL_IDS, getTool } from './tools.js';
import { formatStatus } from './format.js';

const VERSION = '0.1.0';
const PROTOCOLS = ['2024-11-05', '2025-03-26', '2025-06-18', '2025-11-25'];
const MAX_OUTPUT = 60000;

const toolEnum = { type: 'string', enum: TOOL_IDS };

/**
 * MCP tools. Note for the model: a running session cannot change its own
 * account. Switching affects sessions started afterwards; `aims_run` lets you
 * use another account right now for a self-contained headless task.
 */
const TOOL_DEFS = [
  {
    name: 'aims_status',
    description:
      'List the Claude Code / Codex accounts (profiles) managed by aims: which is active, login state, plan usage, cooldowns. ' +
      'The account of the current session is in env AIMS_SESSION_PROFILE (tool:profile). ' +
      'live=true checks with the providers (Codex: free account API; Claude: a one-word Haiku prompt).',
    inputSchema: {
      type: 'object',
      properties: {
        tool: toolEnum,
        live: { type: 'boolean', description: 'Contact the provider to verify login and usage.' },
      },
    },
  },
  {
    name: 'aims_switch',
    description:
      'Make a profile the active account for NEW sessions of a tool (or of both tools when tool is omitted). ' +
      'Does not change the account of an already running session.',
    inputSchema: {
      type: 'object',
      properties: { tool: toolEnum, profile: { type: 'string' } },
      required: ['profile'],
    },
  },
  {
    name: 'aims_failover',
    description:
      'The current account hit its usage limit or needs a break: mark it as limited (until its reset time, or `minutes`) ' +
      'and make the next usable account active. Tell the user they can continue the same conversation with ' +
      '`aims claude --continue` / `aims codex resume --last`.',
    inputSchema: {
      type: 'object',
      properties: {
        tool: toolEnum,
        from: { type: 'string', description: 'Profile to mark (default: the active one).' },
        to: { type: 'string', description: 'Profile to switch to (default: next usable).' },
        minutes: { type: 'number', description: 'Cooldown length if the reset time is unknown.' },
        reason: { type: 'string' },
      },
      required: ['tool'],
    },
  },
  {
    name: 'aims_clear',
    description: 'Clear the cooldown / needs-login mark of one profile (or all profiles of a tool).',
    inputSchema: {
      type: 'object',
      properties: { tool: toolEnum, profile: { type: 'string' } },
      required: ['tool'],
    },
  },
  {
    name: 'aims_run',
    description:
      'Run a self-contained task headlessly with another account right now (`claude -p` or `codex exec`), ' +
      'failing over to the next account on limit/login errors. Returns the final output. ' +
      'Uses the tool\'s default permissions. Use it to delegate work or get a second opinion from the other account/tool.',
    inputSchema: {
      type: 'object',
      properties: {
        tool: toolEnum,
        prompt: { type: 'string' },
        profile: { type: 'string', description: 'Pin a profile (disables failover). Default: active profile.' },
        cwd: { type: 'string', description: 'Working directory (default: where the MCP server runs).' },
        model: { type: 'string' },
        continue: { type: 'boolean', description: 'Continue the most recent conversation in cwd.' },
        timeoutSeconds: { type: 'number', description: 'Default 600.' },
      },
      required: ['tool', 'prompt'],
    },
  },
  {
    name: 'aims_login_help',
    description:
      'Get the terminal command a human must run to (re)log a profile in. Logins open a browser, so they cannot be completed by the model.',
    inputSchema: {
      type: 'object',
      properties: { tool: toolEnum, profile: { type: 'string' } },
      required: ['tool', 'profile'],
    },
  },
];

function text(t, isError = false, structured) {
  const res = { content: [{ type: 'text', text: t }] };
  if (isError) res.isError = true;
  if (structured) res.structuredContent = structured;
  return res;
}

class Collector {
  constructor() {
    this.chunks = [];
    this.size = 0;
  }
  write(d) {
    const s = d.toString();
    this.chunks.push(s);
    this.size += s.length;
    while (this.size > MAX_OUTPUT * 2 && this.chunks.length > 1) this.size -= this.chunks.shift().length;
    return true;
  }
  toString() {
    const s = this.chunks.join('');
    return s.length > MAX_OUTPUT ? `[...truncated...]\n${s.slice(-MAX_OUTPUT)}` : s;
  }
}

async function callTool(name, a = {}) {
  switch (name) {
    case 'aims_status': {
      const ids = a.tool ? [a.tool] : TOOL_IDS;
      const live = [];
      if (a.live) {
        const cfg = loadConfig();
        for (const id of ids) {
          for (const p of cfg.tools[id].order) {
            const r = await liveCheck(cfg, id, p);
            live.push(`${id}/${p}: ${r.status}${r.detail ? ` (${r.detail})` : ''}`);
          }
        }
      }
      const report = statusReport({ ids });
      const session = process.env.AIMS_SESSION_PROFILE || null;
      const body = [
        session ? `This session runs as ${session}.` : 'This session was not started through aims (account unknown).',
        formatStatus(report, { color: false }),
        live.length ? `Live check:\n${live.join('\n')}` : '',
      ]
        .filter(Boolean)
        .join('\n\n');
      return text(body, false, { session, tools: report });
    }
    case 'aims_switch': {
      const ids = useProfile(a.tool || null, a.profile);
      return text(
        `Active ${ids.join(' + ')} profile is now "${a.profile}". New sessions use it; this session keeps its current account.`,
      );
    }
    case 'aims_failover': {
      getTool(a.tool);
      // Default to the account this very session runs on, when it was started through aims.
      const [sessTool, sessProfile] = (process.env.AIMS_SESSION_PROFILE || '').split(':');
      const from = a.from || (sessTool === a.tool ? sessProfile : undefined);
      const r = failover(a.tool, { from, to: a.to, minutes: a.minutes, reason: a.reason || 'limit' });
      const resume = a.tool === 'claude' ? 'aims claude --continue' : 'aims codex resume --last';
      return text(
        `${a.tool}: "${r.from}" is cooling down${r.until ? ` until ${r.until}` : ''}; active profile is now "${r.to}". ` +
          `To continue this conversation on "${r.to}", exit this session and run: ${resume}` +
          (r.pinned ? `\nNote: this terminal pins ${a.tool} to "${r.pinned}" via AIMS_${a.tool.toUpperCase()}_PROFILE; aims will still skip it while it cools down.` : ''),
        false,
        r,
      );
    }
    case 'aims_clear': {
      const names = clear(a.tool, a.profile);
      return text(`Cleared ${a.tool}: ${names.join(', ') || '(none)'}`);
    }
    case 'aims_run': {
      const tool = getTool(a.tool);
      const out = new Collector();
      const err = new Collector();
      const r = await launch({
        id: a.tool,
        profile: a.profile,
        args: tool.headlessArgs({ prompt: a.prompt, model: a.model, resume: a.continue ? 'continue' : null }),
        quiet: true,
        input: Buffer.alloc(0),
        sink: out,
        errSink: err,
        forceHeadless: true,
        cwd: a.cwd,
        timeoutMs: (a.timeoutSeconds || 600) * 1000,
        // don't let the child think it is nested in (or attached to the IDE of) this session
        stripEnv: ['CLAUDECODE', 'CLAUDE_CODE_SSE_PORT', 'CLAUDE_CODE_ENTRYPOINT'],
      });
      const output = out.toString().trim();
      const tail = err.toString().trim().split('\n').slice(-15).join('\n');
      const body = [
        `[${a.tool}/${r.profile} exit ${r.code}${r.failure ? `, ${r.failure} error` : ''}]`,
        output || '(no output)',
        r.code !== 0 && tail ? `--- stderr (tail) ---\n${tail}` : '',
      ]
        .filter(Boolean)
        .join('\n');
      return text(body, r.code !== 0, { profile: r.profile, exitCode: r.code, failure: r.failure || null });
    }
    case 'aims_login_help': {
      const tool = getTool(a.tool);
      return text(
        `Ask the user to run this in a terminal (it opens a browser):\n\n  aims login ${a.tool} ${a.profile}\n\n${tool.loginTip}`,
      );
    }
    default:
      throw Object.assign(new Error(`unknown tool ${name}`), { code: -32602 });
  }
}

export async function handle(msg) {
  const { id, method, params } = msg;
  const reply = (result) => ({ jsonrpc: '2.0', id, result });
  switch (method) {
    case 'initialize': {
      const requested = params && params.protocolVersion;
      return reply({
        protocolVersion: PROTOCOLS.includes(requested) ? requested : PROTOCOLS[PROTOCOLS.length - 1],
        capabilities: { tools: { listChanged: false } },
        serverInfo: { name: 'aims', title: 'AI multi-session', version: VERSION },
        instructions:
          'aims manages several Claude Code / Codex accounts. A running session cannot switch its own account: ' +
          'aims_switch/aims_failover affect new sessions, aims_run uses another account immediately for a headless task.',
      });
    }
    case 'ping':
      return reply({});
    case 'tools/list':
      return reply({ tools: TOOL_DEFS });
    case 'tools/call': {
      try {
        return reply(await callTool(params.name, params.arguments || {}));
      } catch (err) {
        if (err.code === -32602) return { jsonrpc: '2.0', id, error: { code: -32602, message: err.message } };
        return reply(text(`Error: ${err.message}`, true));
      }
    }
    default:
      if (id === undefined || id === null) return null; // notification
      return { jsonrpc: '2.0', id, error: { code: -32601, message: `method not found: ${method}` } };
  }
}

export function serveMcp() {
  const rl = readline.createInterface({ input: process.stdin, terminal: false });
  const write = (o) => process.stdout.write(`${JSON.stringify(o)}\n`);
  let pending = 0;
  let closed = false;
  rl.on('line', async (line) => {
    if (!line.trim()) return;
    let msg;
    try {
      msg = JSON.parse(line);
    } catch {
      return write({ jsonrpc: '2.0', id: null, error: { code: -32700, message: 'parse error' } });
    }
    pending++;
    try {
      const res = await handle(msg);
      if (res) write(res);
    } catch (err) {
      if (msg.id !== undefined) write({ jsonrpc: '2.0', id: msg.id, error: { code: -32603, message: err.message } });
    } finally {
      pending--;
      if (closed && !pending) process.exit(0);
    }
  });
  rl.on('close', () => {
    closed = true;
    if (!pending) process.exit(0);
  });
  return new Promise(() => {});
}

