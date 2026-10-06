export type ToolId = 'claude' | 'codex';

export interface ProfileConfig {
  createdAt?: string;
  /** Uses the tool's normal home (~/.claude, ~/.codex) and the login already in it. */
  existing?: boolean;
  /** Shares nothing with the hub. */
  isolated?: boolean;
  /** Custom profile directory. */
  dir?: string;
  /** Shared entries to keep private for this profile. */
  exclude?: string[];
  /** Extra environment for this profile (e.g. an API key account). */
  env?: Record<string, string>;
}

export interface ToolConfig {
  /** The tool's normal home; shared entries of every profile point here. */
  hub: string | null;
  /** CLAUDE_CONFIG_DIR / CODEX_HOME exactly as the user had it, or null if unset. */
  hubEnv: string | null;
  active: string | null;
  /** Failover order. */
  order: string[];
  profiles: Record<string, ProfileConfig>;
}

export interface FailoverConfig {
  auto: boolean;
  cooldownMinutes: number;
  threshold: number;
}

export interface Config {
  version: 1;
  failover: FailoverConfig;
  statusline: { chain?: string };
  tools: Record<ToolId, ToolConfig>;
}

export interface UsageWindow {
  label: string;
  pct: number;
  resetsAt: string | null;
}

export interface ProfileState {
  until?: string;
  reason?: string;
  markedAt?: string;
  needsLogin?: boolean;
  loginDetail?: string | null;
  usage?: { windows: UsageWindow[]; source?: string; at?: string };
  account?: { email: string | null; plan: string | null };
  /** Last time aims launched the tool with this profile. */
  lastUsedAt?: string;
}

export type State = Partial<Record<ToolId, Record<string, ProfileState>>>;

export type FailureKind = 'auth' | 'limit';

export interface Credentials {
  /** null: cannot tell without asking the tool / provider. */
  present: boolean | null;
  method: string | null;
  email?: string | null;
  org?: string | null;
  plan?: string | null;
  hasRefresh?: boolean | null;
}

export interface Evaluation {
  name: string;
  usable: boolean;
  reasons: string[];
  cred: Credentials;
  state: ProfileState;
}

export interface LinkReport {
  name: string;
  action: 'ok' | 'linked' | 'adopted' | 'merged' | 'conflict' | 'skipped' | 'detached';
  detail?: string;
  /** Conflict nothing can fix automatically; only doctor/sync mention it. */
  quiet?: boolean;
}

/** Anything with a write() method: process.stdout, or a collector. */
export interface Sink {
  write(chunk: string | Uint8Array): unknown;
}
