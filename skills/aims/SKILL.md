---
name: aims
description: Manage several Claude Code and Codex accounts (personal, work) with the aims CLI. Use when the user asks which account is active, wants to switch accounts, hits a usage limit ("You've hit your limit", "usage limit reached"), sees a login error ("Please run /login", "refresh token was already used"), or wants to hand a task to another account.
---

# aims: several Claude Code and Codex accounts on one machine

aims keeps each account's login in its own config folder (`CLAUDE_CONFIG_DIR`
or `CODEX_HOME`) and links everything else (conversations, history, settings,
skills, MCP servers) to the shared `~/.claude` or `~/.codex`. Switching needs no
logout, and a conversation started on one account can continue on another.

## What a running session can and cannot do

- **This session's account is fixed.** A running `claude` or `codex` keeps the
  login it started with. Env `AIMS_SESSION_PROFILE` names it (`claude:work`,
  `codex:personal`); unset means the session was not started through aims.
- `aims use` and `aims failover` change the account of **new** sessions. To
  move this conversation, the user exits and runs `aims claude --continue` or
  `aims codex resume --last`; `aims failover claude --resume` does both.
- To use another account **right now**, run a self-contained task with it:
  the `aims_run` MCP tool (read-only unless you pass `write: true`), or
  `aims claude@work -p "..."` / `aims codex@work exec "..."`.
- A login opens a browser and needs the person. Never try to complete one;
  give the user the command: `aims login <tool> <profile>`.

## Tools

Prefer the MCP tools when they are available: `aims_status`, `aims_switch`,
`aims_failover`, `aims_clear`, `aims_run`, `aims_login_help`. Otherwise use the
CLI:

| Task | Command |
| --- | --- |
| Accounts, logins, plan usage | `aims status` (`--live` asks the providers, `--json` for scripts) |
| Switch the account for new sessions | `aims use work` (every tool) or `aims use claude work` |
| The current account hit its limit | `aims failover claude` (marks it; new sessions use the next one until it resets) |
| Undo a wrong mark | `aims clear claude [profile]` |
| Run something as a given account | `aims claude@work -p "prompt"`, `aims codex@personal exec "prompt"` |
| Log an account in again | tell the user: `aims login claude work` |
| Pin only this terminal | `eval "$(aims env work)"`; undo with `eval "$(aims env --reset)"` |
| Check everything | `aims doctor` |

## Limits and login errors

1. **Usage limit.** Call `aims_failover` (or `aims failover <tool>`), then tell
   the user which account new sessions use until the limit resets and how to
   continue (`aims claude --continue`). Once it resets, new sessions return to
   the active account on their own. Headless runs started through aims move to the
   next account on their own, but only when the failed attempt did nothing;
   otherwise the account is marked and the next run uses another one. Check
   what a failed run already changed before running it again.
2. **Dead login** ("Please run /login", "OAuth token revoked", "refresh token
   was already used"). Call `aims_failover` with `reason: "login"` so new
   sessions avoid the account, and give the user `aims login <tool> <profile>`.
3. **Never copy** `.credentials.json` or `auth.json` between profiles. Refresh
   tokens rotate, so a copied login logs both copies out.

Keep the user's accounts apart: do not move work data to a personal account,
or the reverse, unless the user asks for it.
