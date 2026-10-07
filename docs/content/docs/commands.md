---
title: Commands
weight: 9
---

`aims help <command>` shows the same with examples. `<tool>` is `claude` or `codex`; `<profile>` is an account name you chose.

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
| `--integrations` | `all` | `skill`, `mcp`, `statusline`, `shell` (comma-separated), `all` or `none` |

### `aims login <tool> <profile> [-- tool flags]`

Runs the tool's own login for the profile, creating the profile if it is new. Flags after `--` go to the tool:

```bash
aims login claude work -- --email me@company.com
aims login codex work -- --device-auth
```

`--share` sets the sharing mode of a new profile. When the login turns out to be an account another profile already uses, aims says so.

## Every day

### `aims status [tool]`

Every account with its login, plan usage and marks. Alias: `aims ls`.

| Flag | |
| --- | --- |
| `--live` | Ask the providers first. Free for Codex; costs a few tokens for Claude Code. |
| `--json` | Machine-readable output |

### `aims use [tool] <profile>`

Makes a profile the active account for new sessions. Without a tool, every tool that has a profile with that name switches.

### `aims claude [args]`, `aims codex [args]`

Run the tool as the active account (or the next usable one). All arguments go to the tool. `aims claude@work` and `aims codex@work` pick the account.

### `aims run <tool>[@profile] [args]`

The same as the line above, spelled out.

### `aims env [tool] [profile]`

Prints the exports that pin the current terminal to a profile. Use with `eval "$(aims env work)"`.

| Flag | |
| --- | --- |
| `--reset` | Unpin |
| `--shell` | `bash`, `zsh`, `fish` or `powershell` (default: detected) |

## Accounts

### `aims add <tool> <profile>`

Adds a profile without logging in.

| Flag | |
| --- | --- |
| `--existing` | Adopt the login already in the tool's own folder |
| `--share` | `all`, `settings` or `none` |
| `--dir` | Use this folder instead of `~/.aims/profiles/<tool>/<profile>`. It must not overlap your home folder, `~/.aims`, the tool's folder or another profile. |
| `--login` | Log in right away |

### `aims failover <tool>`

Marks the account you used last as limited and makes the next usable one active.

| Flag | |
| --- | --- |
| `--from` | The profile to mark (default: the one used last) |
| `--to` | The profile to switch to (default: the next usable one) |
| `--minutes` | How long the mark lasts when the reset time is unknown |
| `--reason` | `limit` (default) or `login`. A login mark lasts until `aims login`. |
| `--resume` | Continue the last conversation on the new account |

### `aims clear <tool> [profile]`

Removes limit and login marks.

### `aims order <tool> <profile>...`

Sets the order failover tries profiles in.

### `aims logout <tool> <profile>`

Runs the tool's logout for the profile.

### `aims rm <tool> <profile>`

Forgets a profile. Alias: `aims remove`.

| Flag | |
| --- | --- |
| `--purge` | Also log it out and delete its folder. Anything shared that was written there moves to the hub first; shared history and settings are never deleted. aims only deletes folders it created. |
| `--force` | Purge even if some shared entries could not be moved |
| `-y`, `--yes` | Do not ask |

## Integrations

### `aims setup`

Connects aims to the tools. See [AI integration](../ai-integration).

| Flag | |
| --- | --- |
| `--all` | Skill, MCP server, status line and shell integration |
| `--no-skill`, `--no-mcp` | Skip one |
| `--statusline` | Show the account and plan usage in Claude Code's status line |
| `--shell [name]` | Add the shell integration to your shell's startup file |
| `--tools` | Only these tools |

### `aims uninstall`

Undoes `aims setup`. Profiles and logins stay. `-y` skips the question.

### `aims sync [tool]`

Repairs shared links for every profile (each launch does this for the profile it starts) and copies MCP servers added to your main Claude Code login into the other logins.

### `aims shell-init [shell]`

Prints the shell integration: `claude` and `codex` functions and completion for aims.

### `aims mcp`

The MCP server. Claude Code and Codex start it; you do not run it yourself.

## Maintenance

### `aims doctor`

Checks the tools, each profile's login and shared folders, and the integrations, and prints the command that fixes each problem. It repairs shared links as it goes; `--fix` also adopts databases that looked open (close the tools first). Exits with 1 when something needs attention.

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
