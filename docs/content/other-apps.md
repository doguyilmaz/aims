---
title: Other coding apps
weight: 12
---

aims can share CLI accounts and conversation files with editors, desktop apps and orchestrators. Three things need to agree: the login home, the shared transcript path, and the app's own conversation list. Pointing two apps at the same history does not make them use the same login or automatically populate both lists.

## Check paths without changing anything

```bash
aims bridge
aims bridge --json
```

The report compares the current process' `CODEX_HOME` and `CLAUDE_CONFIG_DIR` with aims' hub and profiles. It follows transcript directory links, so a T3 `shadow` home linked to `~/.codex/sessions` is correctly reported as sharing transcripts. It does not assume the shadow home shares the hub's login.

Inspection never starts a provider, checks or refreshes a login, repairs links, installs the keepalive, or writes config. JSON contains routing paths and environment variable **names**, never credentials or arbitrary profile environment values. This is the first check to use in a running app; `aims doctor` also repairs links and may run login-status commands.

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

Inspection reads directory metadata and link targets, not the conversation tree or database contents. Exports do not copy conversations or databases. Only an explicitly requested export file is created; repeated export to the same path is refused. After installing and checking a configuration, remove that export and its deliberate settings backup when you no longer need them. These commands create no worktrees, sandboxes, background jobs or global PATH wrappers.
