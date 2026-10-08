---
title: Everyday use
weight: 4
---

## Starting a session

`aims claude` and `aims codex` start the real tools with an account's login. Everything after the tool name goes to the tool unchanged:

```bash
aims claude                         # the active account
aims claude --model opus -c         # flags pass straight through
aims claude@work                    # this account, this session only
aims codex@personal exec "fix the failing test"
aims run claude@work -p "explain"   # the same as aims claude@work -p ...
```

Which account a session gets, in order:

1. `@profile` on the command line. This also turns automatic failover off for that run.
2. The terminal's pin, if you set one with `aims env` (below).
3. The active account (`aims use`).
4. If that account is limited or logged out, the next usable one in [failover order](../failover#order), until the first works again. aims says so when it skips one:

```text
› claude: skipping "personal" (limit for 2h10m), using "work"
```

## Switching

```bash
aims use work               # every tool that has a "work" profile
aims use codex personal     # one tool
```

This changes the account for **new** sessions and moves it to the front of the [failover order](../failover#order). A session that is already running keeps its login until it ends; to move it, see [continuing on another account](#continuing-on-another-account).

## Continuing on another account

With history shared (the default), conversations are visible to every account, so a conversation started on one account continues on another:

```bash
aims claude@work --continue     # Claude Code: the latest conversation here
aims codex@work resume --last   # Codex
```

`aims failover claude --resume` combines the switch and the continue.

## Pinning one terminal

Sometimes one terminal should stay on one account while others follow the active one:

```bash
eval "$(aims env work)"          # this terminal: work, for both tools
eval "$(aims env claude client-x)"
eval "$(aims env --reset)"       # back to following the active account
```

`aims env` prints the variables to set (`AIMS_CLAUDE_PROFILE`, and `CLAUDE_CONFIG_DIR` or `CODEX_HOME` so even the bare binaries use that login). It prints for your shell; pass `--shell fish` or `--shell powershell` to choose. In fish, use `aims env work | source`.

## Plain `claude` and `codex`

`aims setup --shell` (part of `aims init`) adds one line to your shell's startup file:

```bash
# >>> aims >>>
command -v aims >/dev/null 2>&1 && eval "$(aims shell-init zsh)"
# <<< aims <<<
```

It defines small `claude` and `codex` functions that run `aims claude` and `aims codex`, so the commands you already type follow the active account and get failover. It also loads completion for `aims`. The line does nothing if aims is not installed, and `aims clean` removes it again.

For fish, aims writes `~/.config/fish/conf.d/aims.fish`. For PowerShell it prints the line to add to your `$PROFILE`.

## The dashboard

`aims` with no arguments opens the dashboard on a terminal. See [Dashboard](../dashboard). When the output is a pipe, it prints `aims status` instead.

## Scripts and automation

```bash
aims status --json                   # everything the table shows, as JSON
aims claude@work -p "..." < /dev/null
aims init --yes --integrations none  # set up without questions
```

Headless runs (`claude -p`, `codex exec`) read their prompt's extra input from stdin like the tools do. When nothing arrives on stdin within 3 seconds, aims goes on without it; redirect from `/dev/null` to skip the wait. Exit codes are the tool's own.
