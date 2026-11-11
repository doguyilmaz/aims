---
title: Other coding apps
weight: 12
---

aims can share CLI accounts and conversation files with editors, desktop apps and orchestrators. Three things need to agree: the login home, the shared transcript path, and the app's own conversation list. Pointing two apps at the same history does not make them use the same login or automatically populate both lists.

## Choose the connection the client supports

| Client surface | Use | What the client keeps |
| --- | --- | --- |
| A program that inherits CLI environment | `aims exec -- <program>` | Its own runtimes and session lifecycle |
| One executable path, as in a CLI provider or SDK override | `aims bridge install`, then select the reported launcher | Native CLI arguments and protocol |
| A command plus prefix arguments | `aims bridge run claude --` or `aims bridge run codex --` | Native stdin/stdout, model and permission options |
| An ACP editor or IDE | `aims bridge acp claude` or `aims bridge acp codex` | The upstream ACP implementation and its bundled CLI version |
| An app's own API, OAuth or managed provider | Configure the app's native provider or its existing-CLI mode | Its own authentication and conversation database |

This is a connection layer for clients of Claude Code and Codex, not a replacement runtime for every agent. It does not turn one application's OAuth token into another application's credential. See the [compatibility survey](../compatibility) for current sources and the level of validation.

## Check paths without changing anything

```bash
aims bridge
aims bridge --json
```

The report compares the current process' `CODEX_HOME` and `CLAUDE_CONFIG_DIR` with aims' hub and profiles. It follows transcript directory links, so a T3 `shadow` home linked to `~/.codex/sessions` is correctly reported as sharing transcripts. It does not assume the shadow home shares the hub's login.

Inspection never starts a provider, checks or refreshes a login, repairs links, installs the keepalive, or writes config. JSON contains routing paths and environment variable **names**, never credentials or arbitrary profile environment values. This is the first check to use in a running app; `aims doctor` performs read-only checks, `aims doctor --fix` applies conservative repairs, and `aims doctor --logins` explicitly inspects local logins.

## Apps that inherit the CLI environment

```bash
aims exec -- my-orchestrator
aims exec --claude work --codex personal -- my-orchestrator --help
aims exec --dry-run -- my-orchestrator
```

`aims exec` passes each tool's preferred account in the current project to the program. Terminal pins from `aims env` still apply; the explicit flags override them. Both tools are configured, including one for which no explicit flag was given. A tool with no aims profiles keeps its inherited environment.

The selected profile's `CODEX_HOME` or `CLAUDE_CONFIG_DIR`, database and memory routing, and configured `env` reach the child. Conflicting login variables are removed as in a normal aims launch. The parent's aims move hooks and CLI session identifiers are removed, so a new application cannot accidentally report turns to the enclosing session. Cross-provider environment overrides that would misroute a selected account are rejected.

aims starts the program once, passes through its arguments and terminal, and returns its exit code. It does not choose accounts using live usage checks, restart the orchestrator, change account order, refresh logins, or synchronize files. The application manages its own session lifecycle. `--dry-run` prints only selected tools and profile names; it does not print the complete environment or command arguments, which may contain secrets.

An already running desktop application keeps its old environment. A launcher that forwards to an existing process cannot change that process' accounts. An app's explicit provider settings can also take precedence over inherited variables.

## One executable path for IDEs and SDKs

Run once from a persistent installation of aims:

```bash
aims bridge install
aims bridge commands --json
```

The installer reports paths such as `~/.aims/bridge/bin/claude` and `~/.aims/bridge/bin/codex` (`.exe` on Windows). Select that path in the app's custom CLI executable field. For example, Cline's **Claude Code** provider accepts a Claude CLI path. The Claude Agent SDK accepts `pathToClaudeCodeExecutable`; the Codex Python SDK accepts `CodexConfig(codex_bin=...)`. Use the SDK's documented override only when intentionally selecting a compatible installed CLI; otherwise keep its packaged runtime and use environment routing.

Each new launcher process resolves the preferred account in **its working directory**, with an explicit terminal pin taking precedence. `aims use` and project orders therefore apply to future workers without updating every editor's configuration. The installation remembers its aims home and native executable locations, so it also works when an IDE starts with a minimal PATH or does not load shell functions. Existing processes keep the account they started with.

Set the worker process' working directory to its project. If a client chooses a workspace only in a later protocol request, or through a native `--cd` option, give that provider connection an explicit profile pin instead: aims does not reinterpret provider arguments or change an active server's account per request.

The paths are executable links to the single installed aims binary: symlinks where available, otherwise hard links on Windows. No shell script quoting or binary copies are needed. aims and `AIMS_HOME` must be on the same volume when Windows requires hard links. After replacing an aims executable on Windows, rerun `aims bridge install` to refresh its links. Building with `go run` cannot create a persistent installation; use `go install` or a normal persistent binary first.

For an application that looks for CLI executables on PATH, opt in for that child only:

```bash
aims exec --launchers -- my-orchestrator
```

This leaves global PATH and the original CLIs untouched. `aims exec` pins the selected accounts for the child; configure launcher paths directly when an orchestrator should resolve each worker's own project order instead. SDKs that use a bundled CLI keep it: aims does not inject `CODEX_PATH`, `CLAUDE_CODE_EXECUTABLE` or other runtime overrides.

To undo the executable integration:

```bash
aims bridge uninstall
```

Only aims' launcher links and manifest are removed. Changed launchers and unrelated files are retained. Account folders, conversations, native CLIs, shell configuration and app settings stay. Point clients back to their original executable after uninstalling.

On Windows, hard links can remain locked by the running aims executable. In that case uninstall reports queued removal: a bounded system PowerShell helper waits for this aims process to exit, reacquires the installation lock, and verifies the unchanged manifest and launcher hashes before every deletion attempt, including after a busy-file retry. It stops if another program changes the installation. Close active clients first. Use `aims bridge --json` to verify completion; a changed or still-busy installation remains available for retry. No executable copy, script file or scheduled cleanup task is installed.

## Native protocols without another supervisor

If the client accepts a command and arguments, installation of executable links is optional:

```bash
aims bridge run codex -- app-server
aims bridge run claude@work -- -p --input-format stream-json --output-format stream-json --verbose
```

Append the client's own arguments, including its prompt, model, sandbox and permission options. aims does not parse provider output, consume stdin, add content to stdout, retry a request, probe a login or migrate data. It routes only the named tool and preserves the other provider's environment. Shell API keys that would override the chosen login are removed; keys deliberately configured in that profile's `env` are passed to the child without being printed.

On macOS and Linux the native process replaces aims, keeping the PID, streams, signals and exit code, with no resident protocol proxy. Windows attaches the same streams and waits for the child, forwarding termination where supported. Running sessions belong to the client; changing aims' preference does not restart them. Use ordinary `aims claude` and `aims codex` when aims itself should supervise and move a terminal session between accounts.

Launchers do not run file migrations or change sharing modes. Existing profiles already receive shared links during aims setup and normal launches. If `aims bridge` reports a broken link, finish the affected sessions before deliberately running `aims sync`.

## ACP clients

Use the maintained upstream adapters, installed separately, with aims supplying the account environment:

```bash
aims bridge acp claude
aims bridge acp codex@work
```

These start `claude-agent-acp` or `codex-acp` already installed on PATH, or an absolute adapter path remembered by `aims bridge install`. aims does not download an adapter, implement its protocol, reinterpret capabilities or force a newer system CLI over the adapter's compatible bundled runtime. Explicit upstream executable overrides are preserved.

For Zed's Custom Agent configuration, use the absolute path to a persistent aims binary, an argument array, and the aims home from `aims bridge commands --json`:

```json
{
  "agent_servers": {
    "aims-claude": {
      "type": "custom",
      "command": "/absolute/path/to/aims",
      "args": ["bridge", "acp", "claude"],
      "env": { "AIMS_HOME": "/home/me/.aims" }
    }
  }
}
```

Use `codex` in the argument array for its ACP adapter. JetBrains and other ACP clients can use the same command, arguments and environment in their custom-agent setup. ACP and Codex app-server are different protocols: do not configure `app-server` as an ACP agent. Session discovery/import remains a client and adapter capability; shared files alone do not create a sidebar entry.

## Client and SDK authors

`aims bridge commands --json` returns a versioned, credential-free catalog of `command`, `args` and `env` values for CLI, Codex app-server and ACP connections, plus a `sdkExecutable` when dedicated launchers are installed. The environment includes the selected aims configuration root, including a custom `AIMS_HOME`; it never includes profile API keys or token files. Pass arguments as arrays instead of composing shell strings. `profileVariable` names the optional environment pin for a particular account.

Keep one provider process per account connection when its home is fixed for the lifetime of that process. Use a client-side import/resume API to expose existing conversations; do not scan or rewrite token files to discover sessions. Keep runtime sockets and live writer locks under their provider's control.

Run aims on the machine or in the container that actually executes the CLI. A local launcher does not mount a host home into a sandbox, copy accounts to a remote worker, or authenticate a cloud runtime.

## T3 Code

T3 supports multiple provider instances. Its **managed** Codex connections manage authentication themselves, outside a normal CLI `auth.json`. An existing-CLI connection can use an aims account's actual home instead. Keep managed connections if you want them: they do not need to be logged out, removed, or converted.

```bash
aims bridge t3
aims bridge t3 --json
aims bridge t3 --output ./t3-settings.aims.json
```

The first command previews an additive plan. `--json` prints new provider entries and diagnostics, without disclosing existing settings. `--output` creates a **new**, owner-readable file containing the original settings plus those entries. It refuses existing files, symlinks and the live settings path. The live application and its settings stay untouched.

The merged export retains any inline secrets already present in the original T3 settings. Keep that file private and out of version control; use `--json` when you only need the credential-free additions. aims never reads or copies the CLI credential files or keychain entries.

The exporter:

- Creates separate existing-CLI connections with names such as `aims · Codex / personal`.
- Reuses an existing direct connection when its configured login matches exactly; a managed connection or matching history path is insufficient.
- Preserves all existing provider instances, model selections and unknown settings. A reserved instance ID with different settings is an error, never overwritten.
- Sets database and memory routing consistently with aims. Profiles with their own history stay private; independent databases are retained.
- Preserves the unset `CLAUDE_CONFIG_DIR` for an adopted default login. Explicitly setting it to `~/.claude` would select another macOS keychain entry and another `.claude.json`.
- Refuses profiles with custom `env`, rather than exporting API keys or silently dropping account settings. Use `aims exec` for those profiles, or configure the connection manually. Relative or tilde Claude homes are also refused because T3 expands them and could change their keychain identity.

T3 server settings normally live at `~/.t3/userdata/settings.json`, or `$T3CODE_HOME/userdata/settings.json`. For another installation:

```bash
aims bridge t3 --settings /path/to/settings.json --output ./t3-settings.aims.json
```

The file format is T3's existing-CLI `providerInstances` configuration; aims does not patch T3 or its database. T3 versions that change that format may need an updated exporter.

### Activate the connections

You can enter the generated connection settings in T3's existing-CLI provider setup. If installing a merged settings file instead, finish running work and close T3 and its server completely first. Generate a fresh export after closing it, review the changes, and keep a backup of the current settings before replacing them. Regenerate the export if the live settings changed: an older full snapshot could hide newer edits. Keep the backup until you have verified the new connections; removing only the newly added `*_aims_*` instances also reverses this integration.

After reopening T3, select the desired existing-CLI connection for new threads. The export deliberately leaves defaults, existing threads and managed logins alone. T3 owns account switching for its threads; aims' interactive failover supervises only sessions started through `aims claude` or `aims codex`.

T3 also derives continuation groups from configured home paths. Distinct aims profile homes can appear as different groups even when their transcript directories link to the same hub. This exporter does not change T3's grouping or promise automatic moves between those instances. Do not set an aims profile as a T3 `shadowHomePath` to bypass that restriction: T3 materializes its own links there and can conflict with the profile's private runtime files. Fully automatic moves across these instances require T3 to recognize separate login and shared-history homes.

### Bring CLI conversations into T3

Shared files make conversations available to T3's scanner. They do **not** automatically create entries in T3's own thread database. Use T3's session/project import flow, select the existing project and conversations, then resume the imported thread. The location and labels of that flow depend on the T3 version.

If a conversation is missing:

1. Run `aims bridge` from the affected environment and compare the actual transcript paths.
2. Check that the selected T3 connection scans the intended CLI home and is enabled.
3. Check the profile's sharing mode. `settings`, `none`, and excluded history intentionally keep conversations apart.
4. Refresh T3's import scan. Its scanner has file/size/time budgets and may omit worktree projects, archived sessions or large transcripts. An app's import limits cannot be fixed by copying logins or redirecting all its state.
5. Finish and close the conversation in the other app before resuming it; keep writer locks intact.

Do not copy `auth.json`, `.credentials.json`, keychain entries, or managed T3 tokens between homes. Refresh tokens rotate; duplicated credentials can invalidate each other. Share the **same login home** when both applications should use a CLI account, or use separate independently authenticated logins. Do not link a managed shadow's auth or daemon sockets into an aims profile.

## Disk use and cleanup

Inspection reads directory metadata and link targets, not the conversation tree or database contents. Launchers add links and one small manifest under aims' own directory. They do not duplicate executables, conversations or databases. Optional exports create only an explicitly requested new file; repeated export to the same path is refused. After installing and checking a configuration, remove its export and deliberate settings backup when no longer needed. The bridge creates no worktrees, sandboxes, scheduled jobs or global PATH wrappers. Its short-lived Windows uninstall helper exits after cleanup or its deadline.

On Windows, aims places protocol launchers in a native job before starting the provider, so forcibly ending the launcher also ends its process tree. See [Windows job objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects). Standard npm `.cmd` adapters are resolved to their runtime and entrypoint and receive arguments directly; custom batch programs are rejected rather than interpreted as npm shims. The supported format follows [npm cmd-shim](https://github.com/npm/cmd-shim/blob/main/lib/index.js).
