---
title: Commands
weight: 9
aliases: ["/docs/commands/"]
---

`aims help <command>` shows the same with examples. `<tool>` is `claude` or `codex`; `<profile>` is an account name you chose.

Arguments in `[brackets]` are optional. Leave them out and aims asks on a terminal: it lists your accounts with who they are and how they are doing, and checks before anything it cannot undo. A profile name alone means the tool that has it (`aims logout work`). In a script, a missing argument is an error that shows an example.

## Get started

### `aims init`

Sets up your accounts step by step: finds the tools and your current logins, names the accounts, sets what they share, connects the integrations and walks you through the logins. Run it again to add an account or reconnect the tools.

| Flag | Default | |
| --- | --- | --- |
| `-y`, `--yes` | | No questions; logins are left for later. Tools that already have profiles are skipped, so it is safe to repeat. |
| `--tools` | every installed tool | Comma-separated, e.g. `claude` |
| `--current` | `personal` | Name for the login you already have |
| `--second` | `work` | Name for the other account |
| `--share` | `all` | `all`, `settings` or `none`. See [What accounts share](../sharing). |
| `--integrations` | `all` | `skill`, `mcp`, `statusline`, `shell`, `keepalive`, `trusthooks` (comma-separated), `all` (all but `trusthooks`) or `none` |

### `aims login [tool] [profile] [-- tool flags]`

Runs the tool's own login for the profile, creating the profile if it is new. Without a profile, aims asks which account to log in, starting on one that needs it, or sets up a new one. Flags after `--` go to the tool:

```bash
aims login claude work -- --email me@company.com
aims login codex work -- --device-auth
```

`--share` sets the sharing mode of a new profile. When the login turns out to be an account another profile already uses, aims says so.

## Every day

### `aims status [tool]`

What is going on, in a few lines. Alias: `aims ls`.

- **Each account:** the plan, its state (`ready`, `limited 2h10m`, `login expired`, `not logged in`) and the usage window that decides that state. `●` marks the account sessions start on. A tag says when an account is `standing in for` the active one, `pinned in this terminal`, `isolated` or keeps its `own history`.
- **Under each tool,** when an account is out: what a session does now (`new sessions start on work`, `a session at its limit has nowhere to move`, `no account works now; sessions wait 1h23m`).
- **Open sessions:** the sessions aims started that still run, with their account, folder and age; `pinned` or `auto off` when they will not move at a limit.
- **Next:** what is left to set up, each with the command that does it. Empty once aims is fully set up. It can say:
  - an account is not logged in, is logged in to the same account as another, or has out-of-date details: `aims login <tool> <profile>`
  - a tool has one account, so there is nothing to move to: `aims login <tool> <name>`
  - plain `claude` and `codex` skip aims: `aims setup --shell <shell>`
  - a session is pinned to an account: the command that restarts it on auto
  - the login keepalive is gone: `aims keepalive on`
  - auto moves are off: the setting that turns them on
  - an account takes no part in moves until no instance of the tool uses it: `aims doctor`
  - an account keeps unused database copies: `aims doctor --fix`

| Flag | |
| --- | --- |
| `--live` | Ask the providers first. Free for Codex and the other tools; costs a few tokens for Claude Code. |
| `--json` | Everything, as JSON: emails, every usage window, marks and who set them |

### `aims usage [provider]`

Every usage window of every account with its bar and when it resets, and under them the limits of installed tools aims does not run sessions on: Antigravity (`agy`), Grok, Kiro and OpenCode. Columns line up across all of them, and on a narrow terminal the windows wrap under each other.

aims asks each other tool's own CLI (`agy -p /usage`, Grok's ACP agent, OpenCode's server), and for Kiro the API behind `kiro-cli`'s `/usage`, using the login kiro-cli keeps. aims never logs in to these tools itself. Filter by `claude`, `codex`, `kiro`, `grok`, `agy` or `opencode`. The numbers are cached until `aims keepalive`, `--refresh` or `--live` updates them.

Scheduled resets, banked reset inventory and user reminders are separate. Banked reset counts come from Codex's account API when available; detail lists can be absent or capped. aims never redeems or buys a reset. A reset time in the past means the reading needs refreshing, not that usage is now zero. Failed refreshes retain the previous numbers **and their original observation time**.

| Flag | |
| --- | --- |
| `--live` | Ask the providers first. Free for Codex and the other tools; costs a few tokens for Claude Code. |
| `--refresh` | Refresh Codex and the other providers without a model prompt or clearing manual routing/login marks. Claude uses its cached native status line. Mutually exclusive with `--live`. |
| `--json` | Versioned usage snapshot: accounts/groups, windows, reset timestamps, banked resets, freshness, errors and custom reminders. Omits credential fields, account email, tenant and profile paths. Reads caches without provider/keychain calls unless refreshing. |

`aims usage reset add <provider> <row-id> --at <RFC3339> --label <text>` adds a custom reminder. Take the row ID from `aims usage --json`; `--at` must include a timezone. `aims usage reset remove <id>` removes it. Both accept `--json` to return the updated snapshot. The reminders live in `AIMS_HOME/usage-reminders.json`, separately from provider data, credentials and session history. Malformed or newer reminder formats are left unchanged.

For the optional menu bar and pinned panel, see [macOS usage app](../macos-usage).

### `aims use [tool] <profile>`

Makes a profile the account new sessions start on in this project, and puts it first in the project's failover order. A project is the git repository you are in, or the folder itself outside one; its order is kept in `~/.aims/config.json`, never in the project. aims says what changed and what other folders use. Without a tool, every tool that has a profile with that name switches.

| Flag | |
| --- | --- |
| `--global` | Change every folder without an order of its own instead. aims names the projects that keep theirs. |

### `aims claude [args]`, `aims codex [args]`

Run the tool as the active account (or the next usable one). All arguments go to the tool. `aims claude@work` and `aims codex@work` pick the account.

Without `@profile`, an interactive session is watched and moves to the next usable account by itself when its account hits a limit or its login dies; see [In an interactive session](../failover#in-an-interactive-session).

### `aims run <tool>[@profile] [args]`

The same as the line above, spelled out.

### `aims env [tool] [profile]`

Prints the exports that pin the current terminal to a profile. Use with `eval "$(aims env work)"`.

| Flag | |
| --- | --- |
| `--reset` | Unpin |
| `--shell` | `bash`, `zsh`, `fish` or `powershell` (default: detected) |

### `aims exec [--claude profile] [--codex profile] -- <program> [args...]`

Starts another application with the preferred CLI accounts in this project. Explicit flags pin them. The child receives aims' home, database and memory routing, without the enclosing session's move hooks or CLI session identifiers. Its arguments and exit code pass through. `--dry-run` prints the selected tools and profile names without running it. No config, links, logins or account order are changed by aims. The application owns its session lifecycle and may override the inherited environment. See [Other coding apps](../other-apps).

`--launchers` puts installed aims launchers first on this child's PATH only. Run `aims bridge install` first. Bundled SDK runtimes and explicit executable overrides are preserved.

## Accounts

### `aims add [tool] [profile]`

Adds a profile without logging in. Without a profile, aims asks for the tool, a name and what it shares, and offers to log it in.

| Flag | |
| --- | --- |
| `--existing` | Adopt the login already in the tool's own folder |
| `--share` | `all`, `settings` or `none` |
| `--dir` | Use this folder instead of `~/.aims/profiles/<tool>/<profile>`. It must not overlap your home folder, `~/.aims`, the tool's folder or another profile. |
| `--login` | Log in right away |

### `aims failover [tool]`

Marks the account you used last as limited. New sessions use the next usable account until the mark lifts, then return to the active one.

Without a tool, aims uses the only tool with a second account, or asks. On a terminal it also asks which account hit its limit (starting on the one used last) and what happened, then offers to continue your last conversation on the next account.

| Flag | |
| --- | --- |
| `--from` | The profile to mark (default: the one used last) |
| `--to` | Make this profile first in this project instead, as `aims use` would |
| `--global` | With `--to`: first in every folder without an order of its own |
| `--minutes` | How long the mark lasts when the reset time is unknown |
| `--reason` | `limit` (default) or `login`. A login mark lasts until `aims login`. |
| `--resume` | Continue the last conversation on the new account |

Run inside a session that [moves by itself](../failover#in-an-interactive-session), it also moves that session's conversation to the next account.

### `aims move [profile]`

Run inside a session started with `aims claude` or `aims codex` (no `@profile`), for example as `!aims move work` in Claude Code. aims restarts the session on that account and resumes the conversation: right away when the session is idle, once the current turn ends otherwise. The conversation then stays there while that account works, instead of going back to the first one. Without a profile, the next usable account takes it. The model can do the same with the `aims_move` MCP tool.

### `aims clear [tool] [profile]`

Removes limit and login marks. Without arguments, aims lists the marked accounts, all picked, and clears the ones you keep picked. A script names the account, or passes `--all` to clear every mark.

### `aims order [tool] [profile]...`

Sets which account comes first and which stand in for it. The first becomes the active account; while it is limited or logged out, new sessions use the next usable one, in this order. `aims order claude personal work` means "personal, and work when personal is out".

Every argument is optional. Without a tool, every tool that has the named profiles changes (`aims order personal work`). Without profiles, aims asks which tool (or both, when they have the same accounts), and which account comes first, shows the order and saves it once you confirm. What happens at a limit is fixed; see [What aims does](../failover#what-aims-does).

Like `aims use`, it sets the order of this project (the git repository, or the folder outside one). Profiles it does not name follow, in the order of every other folder.

| Flag | |
| --- | --- |
| `--global` | Set the order of every folder without one of its own |
| `--reset` | Drop this project's own order, so it follows the order of every folder again |

### `aims logout [tool] [profile]`

Runs the tool's logout for the profile. Without a profile, aims asks which logged-in account, then checks first (No is the default).

### `aims rm [tool] [profile]`

Forgets a profile. Alias: `aims remove`. Without a profile, aims asks which account and what should happen to it: keep it (the default), forget it, or log it out and delete its folder.

| Flag | |
| --- | --- |
| `--purge` | Also log it out and delete its folder. Anything shared that was written there moves to the hub first; shared history and settings are never deleted. aims only deletes folders it created. |
| `--force` | Purge even if some shared entries could not be moved; does not bypass a failed logout |
| `-y`, `--yes` | Do not ask |

Purge records its intent before logging out or deleting files. The native tool must complete logout even if the folder is already missing, because a keychain/keyring login can outlive it. A completed logout is checkpointed; an interrupted purge can then resume without running logout again. Pending profiles cannot start new work or renew their login. Run `aims doctor --check` to see the explicit retry command, then repeat `aims rm <tool> <profile> --purge`. Doctor repair keeps these files and never completes a destructive purge automatically. Removing without `--purge` forgets the label and keeps remaining files; it cannot restore a login already revoked by an earlier purge attempt.

## Integrations

### `aims bridge`

Read-only comparison of the current process' CLI homes, aims profiles and actual transcript link targets. It never invokes a provider, reads credentials, repairs links or changes setup. `--json` prints routing metadata without secrets.

### `aims bridge install`, `aims bridge uninstall`

Create or remove dedicated executable links under `~/.aims/bridge/bin` (or `$AIMS_HOME/bridge/bin`). Clients that accept one executable path can use these for their CLI/SDK connection. Original CLIs, global PATH, app settings, logins and conversations stay. Changed launchers and unrelated files are never overwritten or deleted. Installation requires a persistent aims binary, not `go run`.

### `aims bridge commands`

Print generic CLI, app-server and ACP integration commands. `--json` returns a versioned catalog of executable paths, argument arrays and environment maps, and an SDK executable path after launcher installation. No arbitrary profile environment or credentials are exported. Use it to configure custom agents in editors without depending on an app-specific exporter.

### `aims bridge run <tool>[@profile] [--] [args...]`

Start a native CLI with the preferred or explicit account. stdin, stdout, stderr and arguments pass through without analysis, buffering, retries or authentication probes. Only the named tool is routed; files and sharing modes are not migrated. The native process replaces aims on Unix, and Windows uses attached streams. The client owns active sessions and account switching.

### `aims bridge acp <tool>[@profile] [--] [args...]`

Start an installed upstream `claude-agent-acp` or `codex-acp` with the selected aims account environment. The adapter keeps its bundled native runtime and explicit executable overrides. On Windows, standard npm command shims run through their recorded Node entrypoint or native executable, with arguments passed directly. Custom batch scripts require an explicit native/runtime command (for example `aims exec -- node <adapter-entrypoint>`). No protocol translation, adapter downloads or preview upgrades happen in aims. See [ACP clients](../other-apps#acp-clients).

### `aims bridge t3`

Previews additive T3 Code existing-CLI connections for aims profiles. Existing providers, defaults and unknown settings are retained; managed T3 logins remain separate. Use T3's import flow to add available CLI conversations to its own thread list.

| Flag | |
| --- | --- |
| `--settings` | Read another T3 server settings file instead of `$T3CODE_HOME/userdata/settings.json` or `~/.t3/userdata/settings.json` |
| `--json` | Print new connections and diagnostics; omit existing settings and credentials |
| `--output` | Write merged settings to a **new**, private export file. Refuses existing paths, symlinks and live settings. |

Close T3 before installing a reviewed export, generate it from the latest settings and retain a backup. The exporter does not modify running apps, copy credentials or change sharing modes. See [T3 Code setup](../other-apps#t3-code) for activation, keychain identity and import limits.

### `aims setup`

Connects aims to the tools. On a terminal without flags it lists what aims can add (MCP server and skill, status line, shell integration, login keepalive, Codex hooks on restarts), one line each on what it does, with what is set up now selected; it adds or removes what you change. Flags do the same in a script. See [AI integration](../ai-integration).

When a new version of aims can add something you were never asked about, the next command you run says so once, and `aims setup` is where you choose.

| Flag | |
| --- | --- |
| `--all` | Skill, MCP server, status line, shell integration and keepalive |
| `--no-skill`, `--no-mcp` | Skip one |
| `--statusline` | Show the account and plan usage in Claude Code's status line |
| `--shell [name]` | Add the shell integration to your shell's startup file |
| `--keepalive` | Renew logins in the background; see [`aims keepalive`](#aims-keepalive-onoff) |
| `--trust-hooks` | Run Codex's hooks without its review when aims restarts a session; see [Failover](../failover#codex-hooks) |
| `--tools` | Only these tools |

### `aims sync [tool]`

Reconciles shared links for every profile (each launch does this for the profile it starts) and shares Claude Code MCP servers and project trust between your accounts. It keeps conflicting versions and returns a failure when an entry still needs attention. Close affected tools before reconciling independent content. Open databases, recovery sidecars, unrelated symlinks and independent data databases are kept. For conservative migration repairs, start with `aims doctor --check`, then `aims doctor --fix`.

### `aims shell-init [shell]`

Prints the shell integration: `claude` and `codex` functions and completion for aims.

### `aims mcp`

The MCP server. Claude Code and Codex start it; you do not run it yourself.

## Maintenance

### `aims doctor`

Read-only checks of shared folders, account routing, existing bridge launchers, installed skills and MCP settings. It never starts a provider, reads login files or writes repairs by default. Exits with `1` when a problem remains; informational setup suggestions alone do not fail the check.

| Flag | Behavior |
| --- | --- |
| `--check` | Explicit spelling of the default read-only check |
| `--json` | Versioned report (`schemaVersion: "1.0.0"`) with `checks` and `repairs`; no credential values |
| `--fix` | Create missing links, apply explicit sharing-policy changes and repair an existing owned launcher installation; keep independent settings, histories and data databases |
| `--logins` | Inspect local login status and provider versions, account sharing and MCP secret warnings; may invoke installed CLIs, read login files/keychain and update aims' cached account details |

`--fix --json` reports attempted repairs and then checks the resulting state. Repeating a successful fix makes no further changes. Use `--logins` separately from the other flags.

Fix never forces an open database to move. Verified-unused index/log copies and their SQLite sidecars are archived together in a private `.aims-recovery-*` folder in the profile. The report gives the recovery path. A missing open-file inspection capability leaves them in place. Independent content requires an explicit `aims sync` after closing affected tools; two independent data databases require deliberate SQLite reconciliation. Existing symlinks managed by other tools remain untouched. See [Troubleshooting](../troubleshooting).

### `aims keepalive [on|off]`

Renews every login whose access token has run out or is within 10 minutes of it, through the tool itself. A fresh login costs nothing. Codex renews without a request to the model; Claude Code gets a one-word prompt to its smallest model, the same check as `aims usage --live`.

`on` runs it every hour in the background (launchd, a systemd user timer or the Windows Task Scheduler); `off` removes that. See [Keeping logins alive](../failover#keeping-logins-alive).

### `aims clean`

Takes aims out of your tools (skill, MCP server, status line, shell line, keepalive) and forgets its settings and marks. Every login and account folder stays, so `aims init` brings them back without logging in again. If an integration cannot be removed, configuration and marks are retained for retry. Recovery copies and user additions stay. Alias: `aims uninstall`. `-y` skips the question.

### `aims wipe`

Logs out every account aims created and deletes its folder, then cleans. Anything shared that was written in a folder moves to `~/.claude` or `~/.codex` first. Your conversations, sessions, skills, settings and main logins are never touched, and custom `--dir` folders are forgotten, not deleted. Recovery copies are retained, so the aims home can remain after wipe even though its account configuration and profile folders have been removed.

If a native logout fails or its executable is unavailable, wipe stops with that profile and its registration retained. `--force` does not bypass logout; earlier completed removals remain completed.

| Flag | |
| --- | --- |
| `-y`, `--yes` | Do not ask |
| `--force` | Delete a folder even if something in it is not shared |

### `aims upgrade`

Upgrades aims with the tool that installed it. Alias: `aims update`. `--check` only prints the command.

### `aims completion <shell>`

Prints the completion script for `bash`, `zsh`, `fish` or `powershell`.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | An error, or `doctor` found problems |
| `2` | A command-line mistake, or an unknown profile |
| `130` | Cancelled with Ctrl+C |
| other | `aims claude` and `aims codex` exit with the tool's own code |
