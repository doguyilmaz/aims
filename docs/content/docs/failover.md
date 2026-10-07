---
title: Limits and failover
weight: 5
---

Plans have usage windows (five hours, a week) and logins expire. aims keeps track of both, steers new sessions away from accounts that cannot work right now, and repeats a headless run on another account when that is safe.

## When an account counts as unusable

| Mark | Set by | Cleared |
| --- | --- | --- |
| **Limited** until a time | a run that failed on a usage limit; `aims failover`; a live check | at that time, or `aims clear` |
| **Login expired** | a run that failed on a login error; `aims failover --reason login`; a live check | `aims login`, or `aims clear` |
| **Not logged in** | the account's folder has no login | `aims login` |
| **Near limit** | the last usage report has a window at or over the threshold (95% by default) | when that window resets |

A limit mark lasts until the reset time when aims knows it (from the last usage report), and for 5 hours otherwise. `aims failover --minutes 90` sets it yourself.

## Before a session starts

Every `aims claude` or `aims codex` without `@profile` checks the preferred account first (the terminal's pin, then the active account). If it is unusable, aims takes the next usable account in failover order and tells you which one it skipped and why. If none is usable it starts the preferred one anyway, with a warning, so you still see the tool's own message.

`@profile` means "this account, no matter what": aims warns if it looks unusable but does not switch.

## During a headless run

A headless run (`claude -p`, `codex exec`, or the `aims_run` MCP tool) that fails with a limit or login error is marked as above. aims then runs it again on the next account **only when the failed attempt cannot have changed anything**:

| Output format | Error | Repeated? | Why |
| --- | --- | --- | --- |
| `claude -p --output-format stream-json`, `codex exec --json` | limit or login, no tool call seen | yes | the stream shows every tool call, and there was none |
| same | limit or login, after a tool call | no | the run may already have edited files or run commands |
| plain text or `json` | login error | yes | a dead login stops the first request |
| plain text or `json` | limit | no | the output does not show whether tools ran |

When it does not repeat a run, aims says so, and the next run goes to another account. Output of a failed attempt is held back for a few seconds, so a repeated run prints only the second attempt's answer.

Errors are read only from the tool's own error output and final status, never from the model's text, so a prompt about rate limits does not trigger a failover.

## In an interactive session

A running session cannot change its login. When Claude Code or Codex reports a limit in the middle of a conversation:

```bash
aims failover claude --resume
```

marks the account you used last, makes the next one active, and continues the same conversation there. Inside the session, the model can do the first part itself through the [MCP server](../ai-integration#mcp-server) and then tell you the command to continue.

With the [status line](../ai-integration#status-line) on, Claude Code shows the plan usage of the running account, and suggests `aims failover claude` with the next account's name once a window passes the threshold.

## Where usage numbers come from

| Tool | Source | Cost |
| --- | --- | --- |
| Claude Code | The status line: Claude Code passes the current account's usage to it while you work. | none |
| Claude Code | `aims status --live`: a one-word prompt to the smallest model, to verify the login | a few tokens |
| Codex | `aims status --live`: Codex's local app server reports the account and its rate limits | none |

`aims status` shows the last report and how old it is; `r` in the dashboard runs the live check.

## Order

Failover tries accounts in the order they were added. Change it with:

```bash
aims order claude work personal
```

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

## Undoing a mark

```bash
aims clear claude work     # one account
aims clear claude          # every Claude Code account
```
