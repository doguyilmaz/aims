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
| **Near limit** | the last usage report has a window at or over the threshold (95% by default) | when that window resets |

A limit of the five-hour window counts only while `fiveHour` is on, the default; see [The five-hour limit](#the-five-hour-limit).

A limit mark lasts until the reset time the provider gave ("resets 5pm", "try again in 2 hours") or the last usage report shows, and for 5 hours when neither says. `aims failover --minutes 90` sets it yourself.

Every mark remembers who set it and the provider's message. `aims status` and the dashboard show it, and the line aims prints when it skips an account names it, with the command that removes it:

```text
› claude: skipping "work" (5h limit for 4h53m, marked by a live check), using "personal"
  works again? aims clear claude work
```

## Before a session starts

Every `aims claude` or `aims codex` without `@profile` checks the preferred account first (the terminal's pin, then the active account). If it is unusable, aims takes the next usable account in failover order and tells you which one it skipped and why. If none is usable it starts the preferred one anyway, with a warning, so you still see the tool's own message. A five-hour limit you chose to [wait out](#the-five-hour-limit) does not make an account unusable: aims starts it and says it is waiting.

`@profile` means "this account, no matter what": aims warns if it looks unusable but does not switch.

## During a headless run

A headless run (`claude -p`, `codex exec`, or the `aims_run` MCP tool) that fails with a limit or login error is marked as above. aims then runs it again on the next account **only when the failed attempt cannot have changed anything**:

| Output format | Error | Repeated? | Why |
| --- | --- | --- | --- |
| `claude -p --output-format stream-json`, `codex exec --json` | limit or login, no tool call seen | yes | the stream shows every tool call, and there was none |
| same | limit or login, after a tool call | no | the run may already have edited files or run commands |
| plain text or `json` | limit or login | no | the output does not show whether tools ran |
| any | a five-hour limit you chose to [wait out](#the-five-hour-limit) | no | the account is waiting for its reset, not out |

When it does not repeat a run, aims says so, and the next run goes to another account (or, for a five-hour limit that is waited out, back to the same one). Output of a failed attempt is held back for a few seconds, so a repeated run prints only the second attempt's answer. A run that succeeds clears its account's marks.

Errors are read only from the tool's own error output and final status, never from the model's answer or your prompt, so a prompt about rate limits does not trigger a failover.

## In an interactive session

A running session cannot change its login. Claude Code and Codex read the login once, when they start, and aims only starts them: it is not a proxy between the tool and the provider, so it cannot move a request that is already under way to another account. When Claude Code or Codex reports a limit in the middle of a conversation, end the session (`esc`, then `/exit` or `ctrl+c` twice) and run:

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

## The five-hour limit

Claude Code and Codex plans have a five-hour window and a weekly one. `aims order` asks what to do when the first account uses up its five-hour window:

```text
◆  When personal uses up its 5-hour limit?
   1. Switch to work        new sessions go on with work
   2. Wait for the reset    only the weekly limit or a login problem switches
```

| Answer | What new sessions do |
| --- | --- |
| Switch (the default) | use work while personal's five-hour window is used up, and personal again once it resets |
| Wait | stay on personal until its five-hour window resets. Only the weekly limit or a login problem moves them to work. |

In a script, `aims order --five-hour switch|wait`, with or without an order. It applies to every tool, and is `fiveHour` in the [settings](#settings).

While personal waits for the reset, `aims status` and the status line show it as `waiting`, and a new session says so and names the account it would otherwise use:

```text
› claude: "personal" waits for its 5-hour reset (5h at 100% for 3h20m) instead of failing over
  another account now: aims claude@work
```

A headless run that fails on the five-hour limit is then not repeated on another account. `aims failover` (or `f` in the dashboard, or the AI's `aims_failover`) still marks and switches when you run it: it is you deciding.

aims tells the five-hour limit from the weekly one by the provider's message ("session limit" and "5-hour limit" are the five-hour one) or, when the message does not say, by which window the last usage report shows used up. A limit it cannot place counts as a reason to switch.

## Settings

In `~/.aims/config.json` (see [Configuration](../configuration)):

```json
{
  "failover": {
    "auto": true,
    "threshold": 95,
    "cooldownMinutes": 300,
    "fiveHour": true
  }
}
```

| Key | Default | Meaning |
| --- | --- | --- |
| `auto` | `true` | Pick another account when the preferred one is unusable. With `false`, aims warns but starts the preferred one. |
| `threshold` | `95` | Usage percentage of a window that counts as "near limit". |
| `cooldownMinutes` | `300` | How long a limit mark lasts when the reset time is unknown. |
| `fiveHour` | `true` | Switch when the five-hour window is used up. With `false`, only the weekly limit and login problems switch; sessions wait for the five-hour reset on the same account. |

## Undoing a mark

```bash
aims clear claude work     # one account
aims clear claude          # every Claude Code account
aims clear --all           # every account
```
