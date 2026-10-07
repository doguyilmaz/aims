---
title: AI integration
weight: 7
---

aims can tell Claude Code and Codex which accounts exist and let them act on it: switch the account for new sessions, fail over when this one hits its limit, or hand a self-contained task to another account right now.

```bash
aims setup --all          # everything below
aims setup                # skill and MCP server only
aims uninstall            # take it all back out
```

`aims init` offers the same choices. Restart running sessions afterwards.

## MCP server

`aims mcp` is an [MCP](https://modelcontextprotocol.io) server over stdio. `aims setup` registers it as a user-level server named `aims`: with `claude mcp add` in every Claude Code login (each login keeps its own list), and in the shared `config.toml` for Codex.

| Tool | What it does |
| --- | --- |
| `aims_status` | Every account with its login, plan usage and marks, and which account this session runs as. `live: true` asks the providers first. |
| `aims_switch` | Make a profile active for new sessions of one tool, or of every tool that has it. |
| `aims_failover` | This account hit its limit or lost its login: mark it and activate the next one. Answers with the command that continues the conversation there. |
| `aims_clear` | Remove marks from one profile or all of a tool's profiles. |
| `aims_run` | Run a prompt now with another account (`claude -p` or `codex exec`), with the same [retry rules](../failover#during-a-headless-run) as the CLI. Returns the final answer. Read-only unless the call sets `write`. |
| `aims_login_help` | The command a person must run to log a profile in. The model cannot complete a browser login, and does not try. |

A session cannot change its own login. The server's instructions say so, so the model tells you to restart (`aims claude --continue`) instead of pretending it switched. Each session started through aims carries `AIMS_SESSION_PROFILE` (for example `claude:work`), which `aims_status` reports and `aims_failover` uses as the account to mark.

`aims_run` is read-only by default: Claude Code runs in plan mode and Codex in its read-only sandbox, whatever your settings allow. With `write: true` the run gets your normal settings, which for a headless Claude Code run still excludes anything that would ask for approval. Unless you approved the tool for good, your client shows the call, `write` included, before it runs. A run started this way cannot start another `aims_run`, and when the MCP client disconnects, aims stops any run it started.

## Skill

`aims setup` writes `skills/aims/SKILL.md` into `~/.claude` and `~/.codex` (and into any profile that shares nothing). It teaches the model when to use the MCP tools (a limit error, a dead login, "which account am I on?") and the rules around them: never copy login files, and never move one account's data to another unasked.

## Claude Code plugin

Instead of `aims setup`, Claude Code users can install the MCP server and skill as a plugin. aims itself must be installed and on your `PATH`:

```text
/plugin marketplace add doguyilmaz/aims
/plugin install aims@aims
```

Use one or the other. With both, the model sees the tools twice.

## Status line

`aims setup --statusline` sets aims as Claude Code's [status line](https://docs.anthropic.com/en/docs/claude-code/statusline) command. It shows the account of the running session and its plan usage in a few characters:

```text
~/src/app main  work ▰▱▱▱▱  11% · 7d 47%
~/src/app main  work ▰▰▰▰▱  82% ↻1h20m · 7d 61%
~/src/app main  work ▰▰▰▰▰  97% ↻54m · 7d 61% → personal
```

The bar is the five-hour window, then the weekly one. Numbers turn yellow at 70% and red at the [threshold](../failover#settings); from 70% on, `↻` says when the window resets. Past the threshold, or when the account is marked as limited, `→` names the account aims would move to.

The usage numbers come from Claude Code itself, which passes them to the status line command. aims records them, which is how it knows to skip an account before the limit hits, at no cost.

If you already had a status line command, aims keeps it: your line is shown first, followed by aims' part. `aims uninstall` puts your command back.
