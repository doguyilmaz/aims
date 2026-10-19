---
title: Limits and failover
weight: 5
---

Plans have usage windows (five hours, a week) and logins expire. aims keeps track of both, steers new sessions away from accounts that cannot work right now, and repeats a headless run on another account when that is safe.

## When an account counts as unusable

| Mark | Set by | Cleared |
| --- | --- | --- |
| **Limited** until a time | a run that failed on a usage limit; `aims failover`; `f` in the dashboard; the AI through `aims_failover`; a live check | at that time, a run on it that succeeds, or `aims clear` |
| **Busy** for 2 minutes | a run that failed on a short rate limit (HTTP 429, "too many requests") | at that time |
| **Login expired** | a run that failed on a login error; `aims failover --reason login`; a live check | `aims login`, a run that succeeds, or `aims clear` |
| **Not logged in** | the account's folder has no login | `aims login` |
| **Near limit** | the last usage report has a window at or over the threshold (95% by default); `aims status` shows it as `limited` with the time left | when that window resets |

## What aims does

The policy is fixed. The accounts are tried in [order](#order), the active one first:

| Situation | What aims does |
| --- | --- |
| Nothing wrong | New and running sessions use the first usable account. |
| The first account's 5-hour limit is used up | Sessions use the next usable account. Once the first works again, they go back to it. |
| The first account's weekly limit is used up | Sessions stay on the next account until the weekly reset. |
| Every account is limited | The session uses the account that resets first, and continues on its own when it resets. |
| A login died | Next account, and a hint to run `aims login <tool> <profile>`. |
| A short 429 rate limit | Next account for about 2 minutes, then back. |

aims tells the 5-hour limit from the weekly one by the provider's message ("session limit" and "5-hour limit" are the 5-hour one) or, when the message does not say, by which window the last usage report shows used up.

A limit mark lasts until the reset time the provider gave ("resets 5pm", "try again in 2 hours") or the last usage report shows, and for 5 hours when neither says. `aims failover --minutes 90` sets it yourself.

Every mark remembers who set it and the provider's message. `aims status` and the dashboard show it, and the line aims prints when it skips an account names it, with the command that removes it:

```text
› claude: skipping "work" (5h limit for 4h53m, marked by a live check), using "personal"
  works again? aims clear claude work
```

## Before a session starts

Every `aims claude` or `aims codex` without `@profile` checks the preferred account first (the terminal's pin, then the active account). If it is unusable, aims takes the next usable account in failover order and tells you which one it skipped and why. If every account is limited, it starts the one that resets first and says when it is back (`claude: every account is limited; using work, the first back (in 30m)`). If no reset time is known, it starts the preferred one anyway, with a warning, so you still see the tool's own message.
`@profile` means "this account, no matter what": aims warns if it looks unusable but does not switch.

## During a headless run

A headless run (`claude -p`, `codex exec`, or the `aims_run` MCP tool) that fails with a limit or login error is marked as above. aims then runs it again on the next account **only when the failed attempt cannot have changed anything**:

| Output format | Error | Repeated? | Why |
| --- | --- | --- | --- |
| `claude -p --output-format stream-json`, `codex exec --json` | limit or login, no tool call seen | yes | the stream shows every tool call, and there was none |
| same | limit or login, after a tool call | no | the run may already have edited files or run commands |
| plain text or `json` | limit or login | no | the output does not show whether tools ran |

When it does not repeat a run, aims says so, and the next run goes to another account. Output of a failed attempt is held back for a few seconds, so a repeated run prints only the second attempt's answer. A run that succeeds clears its account's marks.

Errors are read only from the tool's own error output and final status, never from the model's answer or your prompt, so a prompt about rate limits does not trigger a failover.

## In an interactive session

A session started with `aims claude` or `aims codex` without `@profile` is supervised and moves by itself. Plain `claude`, `aims claude@work`, a Claude Code session in a worktree (`-w`) or tmux (`--tmux`), and tools started without aims are not: they keep the login they started with, because Claude Code and Codex read it once, when they start. A conversation only moves to accounts that share its history (the default); one with its own history stays put.

When the account of a supervised session hits a usage limit or its login dies, aims stops the tool and starts it again on the next usable account. It resumes the same conversation (`--resume <id>` for Claude Code, `codex resume <id>` for Codex) with the prompt "continue". Every option you passed is kept; your original prompt is not sent again. aims says what it did:

```text
› claude: personal hit its usage limit; continuing this conversation on work
› claude: personal works again; this conversation goes back to it
```

- **Going back.** Once a turn ends and the first account works again, aims moves the conversation back to it. No prompt is sent, and a turn you have already started is never cut short for it.
- **Before the limit.** If the account is at the threshold (95% by default) when a turn ends, the conversation moves on before the hard limit.
- **Every account limited.** aims waits for the first reset, a minute after it so the tool's own automatic continue wins, and continues on that account.

How aims knows: for Claude Code it passes `--settings` with UserPromptSubmit, Stop and StopFailure hooks to each session (your settings files are not changed, and a `--settings` you pass is merged). For Codex it follows the conversation's rollout file under `sessions/`, and starts `codex` with `-c features.daemon_auto_start=false` so the conversation lives in the process aims stops.

For the sessions that do not move, end the session (`esc`, then `/exit` or `ctrl+c` twice) and run:

```bash
aims failover claude --resume
```

It marks the account you used last and continues the same conversation on the next one. With history shared (the default) the new session reads the whole conversation, so the context is all there; only work that was cut off, such as a background agent that failed on the limit, has to be asked for again. The active account stays as it is: new sessions use the next one while the mark lasts and come back on their own when it lifts. `--to <profile>` makes another account active instead, as `aims use` would. Inside the session, the model can do the first part itself through the [MCP server](../ai-integration#mcp-server) and then tell you the command to continue.

With the [status line](../ai-integration#status-line) on, Claude Code shows the plan usage of the running account, and suggests `aims failover claude` with the next account's name once a window passes the threshold.

## Where usage numbers come from

| Tool | Source | Cost |
| --- | --- | --- |
| Claude Code | The status line: Claude Code passes the current account's usage to it while you work. | none |
| Claude Code | `aims status --live`: a one-word prompt to the smallest model, to verify the login | a few tokens |
| Codex | `aims status --live`: Codex's local app server reports the account and its rate limits | none |

`aims status` shows the last report and how old it is; `r` in the dashboard runs the live check.

## Order

`aims status` lists each tool's accounts in failover order, the active one first. While the active account is limited or logged out, new sessions use the next usable one in this order, tagged `standing in for <active>`, and return to the active one once it works again. To use personal and fall back on work:

```bash
aims order personal work            # every tool that has both
aims order claude personal work     # Claude Code only
aims order                          # asks, shows the order, saves it once you confirm
```

The first account named becomes the active one. `aims use` moves an account to the front the same way.

## Settings

In `~/.aims/config.json` (see [Configuration](../configuration)):

```json
{
  "failover": {
    "auto": true,
    "threshold": 95,
    "cooldownMinutes": 300
  }
}
```

| Key | Default | Meaning |
| --- | --- | --- |
| `auto` | `true` | Pick another account when the preferred one is unusable. With `false`, aims warns but starts the preferred one. |
| `threshold` | `95` | Usage percentage of a window that counts as "near limit". |
| `cooldownMinutes` | `300` | How long a limit mark lasts when the reset time is unknown. |

## Keeping logins alive

A login lasts while it is used: the tool renews its short-lived access token on the next request after it runs out. An account left alone for days can lose its login. `aims keepalive` renews every login whose access token has run out or is within 10 minutes of it, through the tool itself. A fresh login costs nothing.

```text
claude/work: renewed
claude/personal: fresh for 1h12m
codex/work: the login has lapsed (...)
```

Codex renews without a request to the model (its app server reads the account with a refresh). Claude Code has no such way, so aims sends a one-word prompt to its smallest model, the same check as `aims status --live`.

`aims keepalive on` runs it every hour in the background and `aims keepalive off` stops it:

| System | Uses |
| --- | --- |
| macOS | launchd: `~/Library/LaunchAgents/com.doguyilmaz.aims.keepalive.plist`, output in `~/.aims/keepalive.log` |
| Linux | a systemd user timer: `~/.config/systemd/user/aims-keepalive.timer`, output in the journal. It runs while you are logged in, or always after `loginctl enable-linger` |
| Windows | the Task Scheduler: task `aims keepalive` |

It records your `PATH` so the background run finds `claude` and `codex`. `aims setup --keepalive` and `aims init --integrations keepalive` do the same as `on`, `aims doctor` reports whether it is on, and `aims clean` removes it. A login that has lapsed needs `aims login <tool> <profile>`.

## Undoing a mark

```bash
aims clear claude work     # one account
aims clear claude          # every Claude Code account
aims clear --all           # every account
```
