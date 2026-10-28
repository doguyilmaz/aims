---
title: Quick start
weight: 3
aliases: ["/docs/quick-start/"]
---

Five minutes from install to two working accounts. You need Claude Code or Codex installed, ideally already logged in with one of your accounts.

## 1. Run the setup

```bash
aims init
```

The setup walks you through a few questions and does the work as it goes:

```text
┌   aims   accounts for Claude Code and Codex
│
◇  Found
│  Claude Code 2.1.0       me@gmail.com · pro
│  Codex codex-cli 0.200   not logged in
│
◇  Who is me@gmail.com?
│  personal
│
◇  Name your other account
│  work
│
◇  Which account first?
│  personal
│
◇  What should personal and work share?
│  Everything
│
◇  Connect aims to your tools
│  MCP server and skill, status line, plain claude and codex through aims
│
│  ✓ claude/personal  your current login, me@gmail.com
│  ✓ claude/work  ~/.aims/profiles/claude/work
│  ✓ codex/personal  ~/.aims/profiles/codex/personal
│  ✓ codex/work  ~/.aims/profiles/codex/work
│  ✓ skill in 2 places
│  ✓ MCP server in 3 places
│  ✓ status line
│  ✓ shell integration
│
◆  Log in to Claude Code as work now?
│  The browser signs in whichever claude.ai account it is already logged
│  into. Use a private window, or pre-fill the address:
│  aims login claude <profile> -- --email you@company.com
│
│                    Yes     No
```

It ends with what to type from now on:

```text
│  ╭───────────────────────────────────────────╮
│  │ claude, codex       starts on personal    │
│  │ at a limit          → work, then back     │
│  │ aims claude@work    pinned to work        │
│  │ aims                status and next steps │
│  ╰───────────────────────────────────────────╯
└  Ready.
```

What happened:

- **Your current login became a profile.** aims did not copy or move it. The profile called `personal` simply means "Claude Code with its normal folder", so nothing changes for sessions you start the old way.
- **The second account got its own folder**, with links back to your shared history and settings.
- **The integrations** let Claude Code and Codex see and switch accounts themselves, show the account and its usage in Claude Code's status line, and make plain `claude` and `codex` go through aims, which is what lets a session move on at a limit. Each is optional; see [AI integration](../ai-integration).
- **Logins** happen in your browser, one per account. If the browser is already signed in to your personal account, use a private window for the work one.

{{< callout type="info" >}}
On a script or a dotfiles bootstrap, `aims init --yes` does the same without questions and leaves the logins for later: `aims init --yes --current personal --second work --share all`. It is safe to run again.
{{< /callout >}}

## 2. Check the result

```bash
aims status
```

```text
Claude Code  ~/.claude
  ● personal   me@gmail.com     pro    ready           5h ━━━━━━    –   7d ━━━━━━    –  default login
    work       me@company.com   max    ready           5h ━━━━━━    –   7d ━━━━━━    –

Codex  ~/.codex
  ● personal   me@gmail.com     plus   ready           5h ━━━━━━    –   7d ━━━━━━    –
    work       no login                not logged in   5h ━━━━━━    –   7d ━━━━━━    –

Next
  → codex/work is not logged in  aims login codex work

How it works
  claude and codex start on personal. At a limit or a dead login the
  conversation continues on work, and goes back to personal once it works again.
  codex/work cannot be used now (not logged in); a codex session at its limit
  has nowhere to move.
  aims claude@work keeps a session on work and never moves it.
```

`●` marks the account sessions start on. **Next** lists what is still missing, each with the command that does it; when it is empty, aims is fully set up. **How it works** says in words what a new session does and what keeps an account out right now.

## 3. Use it

Start sessions the way you always do: `claude` and `codex`. With the shell integration, they go through aims in new terminals; without it, type `aims claude` and `aims codex`.

Each session says, as it starts, what aims will do with it:

```text
› claude: personal (auto → work)
```

and Claude Code's status line keeps saying it while the session runs:

```text
personal ▰▱▱▱▱  12% · 7d 31% · auto → work
```

| Start a session with | It starts on | At a limit | Status line |
| --- | --- | --- | --- |
| `claude`, `aims claude` | your first account | moves to the next one, and back once the first works again | `auto → work` |
| `aims claude@work` | work | stays on work | `pinned` |
| `claude` without the shell integration | whatever `~/.claude` is logged in as | stays | `not via aims` |

`aims` alone shows your accounts, the sessions open now (and which of them will not move), and what is left to set up. `aims order` changes which account comes first, in this project or, with `--global`, everywhere.

## 4. When an account hits its limit

Usually you do nothing. A session aims watches moves by itself: aims restarts it on the next usable account, continues the same conversation, and moves it back once the first account works again. When every account is limited, it waits for the first reset and goes on. New sessions skip the limited account too.

A pinned session (`aims claude@work`) or one started without aims keeps its account. To move its conversation, end the session and run:

```bash
aims failover claude --resume
```

This marks the account you were using as limited until its reset time and continues your last conversation, with all its context, on the next account. New sessions use that one until the limit resets, then go back to your active account on their own. [Limits and failover](../failover) explains what aims does on its own.

## Setting it up again

Run `aims init` again and pick **Go through setup again**. Accounts and logins stay. aims checks every login (not logged in, two profiles on one account, out of date details) and offers to log in again, asks which account comes first, then what it connects, and ends with how it works.

## Adding more accounts

Run `aims init` again and pick **Add an account**, or:

```bash
aims login claude client-x          # creates the profile and logs it in
aims login codex client-x -- --device-auth
```

Profile names are yours to choose: letters, digits, `.`, `_` and `-`.
