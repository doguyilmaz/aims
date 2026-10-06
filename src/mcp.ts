import { loadConfig } from './config.ts';
import { formatStatus } from './format.ts';
import { liveCheck } from './health.ts';
import { VERSION, clear, failover, statusReport, useProfile } from './ops.ts';
import { killActiveChildren } from './proc.ts';
import { launch } from './run.ts';
import { TOOL_IDS, getTool, isToolId } from './tools.ts';
import type { Sink, ToolId } from './types.ts';
import { errorMessage } from './util.ts';

const PROTOCOLS = ['2024-11-05', '2025-03-26', '2025-06-18', '2025-11-25'];
const MAX_OUTPUT = 60000;

const toolEnum = { type: 'string', enum: TOOL_IDS };

/**
 * A running session cannot change its own account: switching affects
 * sessions started afterwards; `aims_run` uses another account right now for
 * a self-contained headless task.
 */
export const TOOL_DEFS = [
  {
    name: 'aims_status',
    description:
      'List the Claude Code / Codex accounts (profiles) managed by aims: which is active, login state, plan usage, cooldowns. ' +
      'The account of the current session is in env AIMS_SESSION_PROFILE (tool:profile). ' +
      'live=true checks with the providers (Codex: free account API; Claude: a one-word Haiku prompt).',
    inputSchema: {
      type: 'object',
      properties: { tool: toolEnum, live: { type: 'boolean', description: 'Contact the provider to verify login and usage.' } },
    },
  },
  {
    name: 'aims_switch',
    description:
      'Make a profile the active account for NEW sessions of a tool (or of both tools when tool is omitted). ' +
      'Does not change the account of an already running session.',
    inputSchema: { type: 'object', properties: { tool: toolEnum, profile: { type: 'string' } }, required: ['profile'] },
  },
  {
    name: 'aims_failover',
    description:
      'The current account hit its usage limit or its login died: mark it (until its reset time, or `minutes`) ' +
      'and make the next usable account active. Tell the user they can continue the same conversation with ' +
      '`aims claude --continue` / `aims codex resume --last`.',
    inputSchema: {
      type: 'object',
      properties: {
        tool: toolEnum,
        from: { type: 'string', description: 'Profile to mark (default: the account of this session, else the active one).' },
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
    inputSchema: { type: 'object', properties: { tool: toolEnum, profile: { type: 'string' } }, required: ['tool'] },
  },
  {
    name: 'aims_run',
    description:
      'Run a self-contained task headlessly with another account right now (`claude -p` or `codex exec`), ' +
      'failing over to the next account on limit/login errors. Returns the final output. ' +
      "Uses the tool's default permissions. Use it to delegate work or get a second opinion from the other account/tool.",
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
      'Get the terminal command a human must run to (re)log a profile in. Logins open a browser, so the model cannot complete them.',
    inputSchema: { type: 'object', properties: { tool: toolEnum, profile: { type: 'string' } }, required: ['tool', 'profile'] },
  },
];

interface ToolResult {
  content: Array<{ type: 'text'; text: string }>;
  isError?: boolean;
  structuredContent?: unknown;
}

function text(t: string, isError = false, structured?: object): ToolResult {
  return { content: [{ type: 'text', text: t }], ...(isError && { isError }), ...(structured && { structuredContent: structured }) };
}

class Collector implements Sink {
  private chunks: string[] = [];
  private size = 0;
  private decoder = new TextDecoder();
  write(d: string | Uint8Array): boolean {
    const s = typeof d === 'string' ? d : this.decoder.decode(d, { stream: true });
    this.chunks.push(s);
    this.size += s.length;
    while (this.size > MAX_OUTPUT * 2 && this.chunks.length > 1) this.size -= this.chunks.shift()!.length;
    return true;
  }
  toString(): string {
    const s = this.chunks.join('');
    return s.length > MAX_OUTPUT ? `[...truncated...]\n${s.slice(-MAX_OUTPUT)}` : s;
  }
}

class InvalidParams extends Error {}

type Args = Record<string, unknown>;
const str = (v: unknown) => (typeof v === 'string' && v ? v : undefined);

function toolArg(a: Args, required: true): ToolId;
function toolArg(a: Args, required?: false): ToolId | undefined;
function toolArg(a: Args, required = false): ToolId | undefined {
  if (a.tool === undefined && !required) return undefined;
  if (!isToolId(a.tool)) throw new InvalidParams(`tool must be one of: ${TOOL_IDS.join(', ')}`);
  return a.tool;
}

/** The account this MCP server's own session runs on, if started through aims. */
function sessionProfile(): { tool: string; profile: string } | null {
  const [tool, profile] = (process.env.AIMS_SESSION_PROFILE ?? '').split(':');
  return tool && profile ? { tool, profile } : null;
}

async function callTool(name: string, a: Args): Promise<ToolResult> {
  switch (name) {
    case 'aims_status': {
      const tool = toolArg(a);
      const ids = tool ? [tool] : TOOL_IDS;
      const live: string[] = [];
      if (a.live === true) {
        const cfg = loadConfig();
        for (const id of ids) {
          for (const p of cfg.tools[id].order) {
            const r = await liveCheck(cfg, id, p);
            live.push(`${id}/${p}: ${r.status}${r.detail ? ` (${r.detail})` : ''}`);
          }
        }
      }
      const report = statusReport({ ids });
      const session = process.env.AIMS_SESSION_PROFILE ?? null;
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
      const profile = str(a.profile);
      if (!profile) throw new InvalidParams('profile is required');
      const ids = useProfile(toolArg(a) ?? null, profile);
      return text(`Active ${ids.join(' + ')} profile is now "${profile}". New sessions use it; this session keeps its current account.`);
    }
    case 'aims_failover': {
      const tool = toolArg(a, true);
      const sess = sessionProfile();
      const r = failover(tool, {
        from: str(a.from) ?? (sess?.tool === tool ? sess.profile : undefined),
        to: str(a.to),
        minutes: typeof a.minutes === 'number' ? a.minutes : undefined,
        reason: str(a.reason) ?? 'limit',
      });
      const resume = tool === 'claude' ? 'aims claude --continue' : 'aims codex resume --last';
      const pinNote = r.pinned
        ? `\nNote: this terminal pins ${tool} to "${r.pinned}" (AIMS_${tool.toUpperCase()}_PROFILE); aims still skips it while it cools down.`
        : '';
      return text(
        `${tool}: "${r.from}" is cooling down${r.until ? ` until ${r.until}` : ''}; active profile is now "${r.to}". ` +
          `To continue this conversation on "${r.to}", exit this session and run: ${resume}${pinNote}`,
        false,
        r,
      );
    }
    case 'aims_clear': {
      const tool = toolArg(a, true);
      const names = clear(tool, str(a.profile));
      return text(`Cleared ${tool}: ${names.join(', ') || '(none)'}`);
    }
    case 'aims_run': {
      const tool = toolArg(a, true);
      const prompt = str(a.prompt);
      if (!prompt) throw new InvalidParams('prompt is required');
      const out = new Collector();
      const err = new Collector();
      const r = await launch({
        id: tool,
        profile: str(a.profile),
        args: getTool(tool).headlessArgs({ prompt, model: str(a.model), resume: a.continue === true ? 'continue' : null }),
        quiet: true,
        input: null,
        sink: out,
        errSink: err,
        forceHeadless: true,
        cwd: str(a.cwd),
        timeoutMs: (typeof a.timeoutSeconds === 'number' ? a.timeoutSeconds : 600) * 1000,
        // don't let the child think it is nested in (or attached to the IDE of) this session
        stripEnv: ['CLAUDECODE', 'CLAUDE_CODE_SSE_PORT', 'CLAUDE_CODE_ENTRYPOINT'],
      });
      // Structured output (stream-json / --json): return the answer, not the event stream.
      const output = (r.finalText ?? (r.code === 0 ? out.toString() : '')).trim();
      const problem = r.code !== 0 ? (r.errorText || err.toString().trim().split('\n').slice(-15).join('\n')) : '';
      const body = [
        `[${tool}/${r.profile ?? 'default'} exit ${r.code}${r.failure ? `, ${r.failure} error` : ''}]`,
        output || (problem ? '' : '(no output)'),
        problem ? `--- error ---\n${problem}` : '',
      ]
        .filter(Boolean)
        .join('\n');
      return text(body, r.code !== 0, { profile: r.profile, exitCode: r.code, failure: r.failure ?? null });
    }
    case 'aims_login_help': {
      const tool = toolArg(a, true);
      const profile = str(a.profile) ?? '<profile>';
      return text(
        `Ask the user to run this in a terminal (it opens a browser):\n\n  aims login ${tool} ${profile}\n\n${getTool(tool).loginTip}`,
      );
    }
    default:
      throw new InvalidParams(`unknown tool ${name}`);
  }
}

interface RpcMessage {
  jsonrpc?: string;
  id?: string | number | null;
  method?: string;
  params?: { protocolVersion?: string; name?: string; arguments?: Args };
}

export async function handle(msg: RpcMessage): Promise<object | null> {
  const { id, method, params } = msg;
  const reply = (result: unknown) => ({ jsonrpc: '2.0', id, result });
  const fail = (code: number, message: string) => ({ jsonrpc: '2.0', id, error: { code, message } });
  switch (method) {
    case 'initialize': {
      const requested = params?.protocolVersion;
      return reply({
        protocolVersion: requested && PROTOCOLS.includes(requested) ? requested : PROTOCOLS.at(-1),
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
    case 'tools/call':
      try {
        return reply(await callTool(params?.name ?? '', params?.arguments ?? {}));
      } catch (err) {
        if (err instanceof InvalidParams) return fail(-32602, err.message);
        return reply(text(`Error: ${errorMessage(err)}`, true));
      }
    default:
      if (id === undefined || id === null) return null; // notification
      return fail(-32601, `method not found: ${method}`);
  }
}

/** Newline-delimited JSON-RPC over stdio. stdout carries protocol messages only. */
export async function serveMcp(): Promise<number> {
  const write = (o: object) => process.stdout.write(`${JSON.stringify(o)}\n`);
  const pending = new Set<Promise<void>>();
  // The client went away: stop any aims_run children instead of orphaning them.
  for (const sig of ['SIGTERM', 'SIGHUP'] as const) {
    process.on(sig, () => {
      killActiveChildren('SIGTERM');
      process.exit(128 + (sig === 'SIGTERM' ? 15 : 1));
    });
  }
  for await (const line of console) {
    if (!line.trim()) continue;
    let msg: RpcMessage;
    try {
      msg = JSON.parse(line);
    } catch {
      write({ jsonrpc: '2.0', id: null, error: { code: -32700, message: 'parse error' } });
      continue;
    }
    // Requests run concurrently: a long aims_run must not block ping or status.
    const p: Promise<void> = handle(msg)
      .then((res) => {
        if (res) write(res);
      })
      .catch((err) => {
        if (msg.id !== undefined) write({ jsonrpc: '2.0', id: msg.id, error: { code: -32603, message: errorMessage(err) } });
      })
      .finally(() => pending.delete(p));
    pending.add(p);
  }
  // stdin closed: nobody is left to read results. Give quick calls a moment, then stop.
  await Promise.race([Promise.allSettled([...pending]), Bun.sleep(1500)]);
  killActiveChildren('SIGTERM');
  return 0;
}
