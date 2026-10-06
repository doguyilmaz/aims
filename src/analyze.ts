import type { ToolId } from './types.ts';

/**
 * Watches a headless run's output and answers two questions without ever
 * looking at the prompt or the model's own text:
 *  - errorText(): what the provider/CLI reported as the error
 *  - activity():  did the run demonstrably do something with side effects
 *                 (a tool call)? 'unknown' when the output format hides it.
 */
export interface RunAnalyzer {
  line(stream: 'out' | 'err', line: string): void;
  /** A structured final result reported failure (exit code may still be 0). */
  resultFailed(): boolean;
  errorText(): string;
  activity(): 'none' | 'some' | 'unknown';
  /** The final answer, for structured formats. */
  finalText(): string | null;
}

function optionValue(args: string[], name: string): string | null {
  for (let i = 0; i < args.length; i++) {
    const a = args[i]!;
    if (a === name) return args[i + 1] ?? null;
    if (a.startsWith(`${name}=`)) return a.slice(name.length + 1);
  }
  return null;
}

function parse(line: string): Record<string, any> | null {
  if (!line.startsWith('{')) return null;
  try {
    return JSON.parse(line);
  } catch {
    return null;
  }
}

const keepLast = (arr: string[], line: string, n: number) => {
  if (!line.trim()) return;
  arr.push(line);
  if (arr.length > n) arr.shift();
};

/** `claude -p`: text | json | stream-json */
function claudeAnalyzer(args: string[]): RunAnalyzer {
  const format = optionValue(args, '--output-format') ?? 'text';
  const lastOut: string[] = [];
  const errLines: string[] = [];
  let result: Record<string, any> | null = null;
  let toolUse = false;
  return {
    line(stream, line) {
      if (stream === 'err') return keepLast(errLines, line, 20);
      if (format === 'text') return keepLast(lastOut, line, 5);
      const ev = parse(line);
      if (!ev) return;
      if (ev.type === 'result') result = ev;
      if (ev.type === 'assistant' && Array.isArray(ev.message?.content)) {
        if (ev.message.content.some((c: { type?: string }) => c?.type === 'tool_use')) toolUse = true;
      }
    },
    resultFailed: () => Boolean(result?.is_error),
    errorText() {
      // text mode prints only the final result, so a failed run's stdout is the error itself
      const res = result ? [result.is_error ? String(result.result ?? result.subtype ?? '') : ''] : lastOut;
      return [...res, ...errLines].join('\n');
    },
    activity: () => (format === 'stream-json' ? (toolUse ? 'some' : 'none') : 'unknown'),
    finalText: () => (result && !result.is_error ? String(result.result ?? '') : null),
  };
}

const CODEX_TOOL_ITEMS = new Set(['command_execution', 'file_change', 'mcp_tool_call', 'web_search']);
// `ERROR: ...`, `2026-...Z ERROR module: ...`, `warning: ...` -- never the echoed prompt
const CODEX_ERROR_LINE = /^(?:ERROR:|warning:|\d{4}-\d\d-\d\dT\S+\s+ERROR\s)/;

/** `codex exec` / `codex review`: human output or --json events. */
function codexAnalyzer(args: string[]): RunAnalyzer {
  const json = args.includes('--json');
  const errors: string[] = [];
  let tool = false;
  let failed = false;
  let lastMessage: string | null = null;
  return {
    line(stream, line) {
      if (json && stream === 'out') {
        const ev = parse(line);
        if (!ev) return;
        if (ev.type === 'error') keepLast(errors, String(ev.message ?? ''), 20);
        if (ev.type === 'turn.failed') {
          failed = true;
          keepLast(errors, String(ev.error?.message ?? 'turn failed'), 20);
        }
        if ((ev.type === 'item.started' || ev.type === 'item.completed') && CODEX_TOOL_ITEMS.has(ev.item?.type)) tool = true;
        if (ev.type === 'item.completed' && ev.item?.type === 'agent_message') lastMessage = String(ev.item.text ?? '');
        return;
      }
      if (stream === 'err' && CODEX_ERROR_LINE.test(line)) keepLast(errors, line, 20);
    },
    resultFailed: () => failed,
    errorText: () => errors.join('\n'),
    activity: () => (json ? (tool ? 'some' : 'none') : 'unknown'),
    finalText: () => lastMessage,
  };
}

export function analyzerFor(id: ToolId, args: string[]): RunAnalyzer {
  return id === 'claude' ? claudeAnalyzer(args) : codexAnalyzer(args);
}
