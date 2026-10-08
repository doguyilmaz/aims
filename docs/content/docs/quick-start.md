---
title: Quick start
weight: 3
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
◇  What should personal and work share?
│  Everything
│
◇  Connect aims to your tools
│  MCP server and skill, status line, shell integration
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

What happened:

- **Your current login became a profile.** aims did not copy or move it. The profile called `personal` simply means "Claude Code with its normal folder", so nothing changes for sessions you start the old way.
- **The second account got its own folder**, with links back to your shared history and settings.
- **The integrations** let Claude Code and Codex see and switch accounts themselves, show the account and its usage in Claude Code's status line, and make plain `claude` and `codex` follow the active account. Each is optional; see [AI integration](../ai-integration).
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
  ● personal   me@gmail.com · pro       ready   default login
    work       me@company.com · max     ready

Codex  ~/.codex
  ● personal   me@gmail.com · plus      ready
    work       no login                 not logged in
               → aims login codex work
```

`●` marks the active account: the one new sessions get. Anything that needs you comes with the command that fixes it.

## 3. Use it

```bash
aims claude              # Claude Code as the active account
aims claude@work         # as work, for this session only
aims codex@work exec "summarize the diff"
aims use work            # from now on, new sessions of both tools use work
aims                     # the dashboard
```

With the shell integration on, plain `claude` and `codex` behave like `aims claude` and `aims codex` in new terminals.

## 4. When an account hits its limit

Usually you do nothing: the next session you start skips the limited account. To move a conversation in the middle of a task:

```bash
aims failover claude --resume
```

This marks the account you were using as limited until its reset time and continues your last conversation on the next account. New sessions use that one until the limit resets, then go back to your active account on their own. [Limits and failover](../failover) explains what aims does on its own.

## Adding more accounts

Run `aims init` again and pick **Add an account**, or:

```bash
aims login claude client-x          # creates the profile and logs it in
aims login codex client-x -- --device-auth
```

Profile names are yours to choose: letters, digits, `.`, `_` and `-`.
