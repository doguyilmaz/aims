---
name: aims
description: Manage multiple Claude Code / Codex accounts (e.g. personal and work) with the aims CLI. Use when the user asks which account is active, wants to switch accounts, hits a usage/rate limit ("You've hit your limit", "Usage limit reached"), sees a login error ("Please run /login", "refresh token was already used"), or wants to hand a task to another account or to the other tool.
---

# aims: several Claude Code / Codex accounts on one machine

`aims` keeps one login per account in its own config dir (`CLAUDE_CONFIG_DIR` /
`CODEX_HOME`) and links everything else (transcripts, history, settings, skills,
MCP config) to the shared `~/.claude` / `~/.codex`. Switching accounts is
therefore instant and never needs logout/login, and a conversation started on
one account can be continued on another.

## What you can and cannot do from inside a session

- **This session's account is fixed.** A running `claude`/`codex` process keeps
  the login it started with. Find it in env `AIMS_SESSION_PROFILE`
  (`claude:work`, `codex:personal`; unset = not started through aims).
- Switching (`aims use`, `aims failover`) changes the account for **new**
  sessions. To move *this* conversation to another account the user exits and
  runs `aims claude --continue` (or `aims codex resume --last`) -- or
  `aims failover claude --resume` does both steps.
- To use another account **right now**, run a self-contained headless task with
  it: MCP tool `aims_run`, or `aims claude@work -p "..."` / `aims codex@work exec "..."`.
- Logging in needs a browser and a human: never try to complete it yourself;
  give the user the command (`aims login <tool> <profile>`).

## Prefer the MCP tools when present

`aims_status`, `aims_switch`, `aims_failover`, `aims_clear`, `aims_run`,
`aims_login_help`. Otherwise use the CLI through the shell:

| Task | Command |
| --- | --- |
| Show accounts, health, usage | `aims status` (`--live` to verify with the providers, `--json`) |
| Switch active account | `aims use work` (both tools) / `aims use claude work` |
| Current account hit its limit | `aims failover claude` (marks it limited, activates the next) |
| Clear a wrong limited/login mark | `aims clear claude [profile]` |
| Run something as a given account | `aims claude@work -p "prompt"` / `aims codex@personal exec "prompt"` |
| Re-login an account | tell the user: `aims login claude work` |
| Pin only this terminal | `eval "$(aims env work)"`, undo with `eval "$(aims env --reset)"` |
| Diagnose | `aims doctor` |

## Handling limits and login errors

1. Usage limit on the current account: call `aims_failover` (or `aims failover <tool>`),
   then tell the user which account is now active and how to continue
   (`aims claude --continue`). Headless runs started via `aims` already retry on
   the next account by themselves.
2. "Please run /login", "OAuth token revoked", "refresh token was already used":
   the login is dead. Run `aims_failover` with `reason: "login"` so new sessions avoid it,
   and give the user `aims login <tool> <profile>`.
3. Never copy `.credentials.json` / `auth.json` between profiles: refresh tokens
   rotate, so a copied login logs both copies out.

Keep the user's personal/work separation in mind: do not move work data to a
personal account (or the reverse) unless the user asks for it.
