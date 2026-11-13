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

- Env `AIMS_SESSION_PROFILE` names this session's account (`claude:work`,
  `codex:personal`); unset means the session was not started through aims.
- **A session started with `aims claude` or `aims codex` (no `@profile`) moves
  on its own** (env `AIMS_RUN` is set): when its account hits a limit or its login
  dies, aims restarts it on the next usable account and continues the
  conversation; once the first account works again, aims moves it back after a
  turn. When every account is limited, it waits for the first reset and goes
  on. Nothing to do; tell the user what happened if they ask.
- **To move such a session to another account on purpose**, call `aims_move`
  (or run `aims move [profile]`), then end your turn: aims restarts the session
  on that account once the turn ends and resumes the conversation, which stays
  there while it works. The user can do the same with `!aims move work`.
- Any other running session (`aims claude@work`, plain `claude` or `codex`) keeps
  the login it started with. `aims use` and `aims failover` change the account
  of **new** sessions, in this project unless `--global`. To move such a conversation, the user exits and runs
  `aims claude --continue` or `aims codex resume --last`;
  `aims failover claude --resume` does both.
- To use another account **right now**, run a self-contained task with it:
  the `aims_run` MCP tool (read-only unless you pass `write: true`), or
  `aims claude@work -p "..."` / `aims codex@work exec "..."`.
- A login opens a browser and needs the person. Never try to complete one;
  give the user the command: `aims login <tool> <profile>`.

## Tools

Prefer the MCP tools when they are available: `aims_status`, `aims_switch`,
`aims_failover`, `aims_move`, `aims_clear`, `aims_run`, `aims_login_help`. Codex may not list
them up front: look for `mcp__aims__aims_status` and the rest among its deferred
tools (in `exec`, `ALL_TOOLS`; call `await tools.mcp__aims__aims_status({})`).
Otherwise use the CLI:

| Task | Command |
| --- | --- |
| Accounts, logins, plan usage | `aims status` (`--live` asks the providers, `--json` for scripts) |
| Switch the account for new sessions in this project | `aims use work` (every tool) or `aims use claude work`; `--global` for every folder |
| The current account hit its limit | `aims failover claude` (marks it; new sessions use the next one) |
| Move this session's conversation now | `aims move work` (only in a session aims moves) |
| Undo a wrong mark | `aims clear claude [profile]` |
| Run something as a given account | `aims claude@work -p "prompt"`, `aims codex@personal exec "prompt"` |
| Log an account in again | tell the user: `aims login claude work` |
| Pin only this terminal | `eval "$(aims env work)"`; undo with `eval "$(aims env --reset)"` |
| Check everything | `aims doctor` |

## Limits and login errors

1. **Usage limit.** In a session aims moves on its own (above), do nothing.
   Otherwise call `aims_failover` (or `aims failover <tool>`), then tell the
   user which account new sessions use now, as its answer says, and how to
   continue (`aims claude --continue`). Once the limit resets, new sessions
   return to the active account on their own. Headless runs started through aims move
   to the next account on their own, but only when the failed attempt did
   nothing; otherwise the account is marked and the next run uses another one.
   Check what a failed run already changed before running it again.
2. **Dead login** ("Please run /login", "OAuth token revoked", "refresh token
   was already used"). Call `aims_failover` with `reason: "login"` so new
   sessions avoid the account, and give the user `aims login <tool> <profile>`.
3. **Never copy** `.credentials.json` or `auth.json` between profiles. Refresh
   tokens rotate, so a copied login logs both copies out.

Keep the user's accounts apart: do not move work data to a personal account,
or the reverse, unless the user asks for it.
