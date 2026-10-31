---
title: Introduction
type: docs
cascade:
  type: docs
aliases: ["/docs/"]
---

aims (short for **AI multi-session**) runs several Claude Code and Codex accounts on one machine. A personal and a work subscription, say, or two work seats. Each account keeps its own login; everything else is shared. You switch accounts without logging out, and when one account runs out of quota the next session moves to another.

## The problem

Claude Code and Codex keep one login per machine. To use a second account you log out, log in with the other one in the browser, and do it again to switch back. Your conversations and settings stay with whichever folder the tool happens to use, and a limit in the middle of a task means stopping until it resets.

Both tools can be pointed at a different config folder (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`), which keeps logins apart. But then each folder starts empty: no history, no settings, no skills, no MCP servers, and nothing to tell you which folder is which.

aims does the bookkeeping. It gives every account its own folder, links the shared parts back to your normal `~/.claude` and `~/.codex`, remembers which account is active, and watches for limits and dead logins.

## How it looks

```text
$ aims status
Claude Code
  order of this project (~/code/client-app); elsewhere personal → work
  ● personal  pro       login expired  7d 37%
    work      team      ready          7d 84%  standing in for personal
  new sessions start on work

Codex
  ● personal  plus      ready          7d 31%
    work      business  ready

Open sessions
  Claude Code  work      ~/code/client-app · 12h59m ago
  Claude Code  work      ~/code/api · 2h4m ago
  Codex        personal  ~/code/blog · 6m ago

Next
  → claude/personal is not logged in  aims login claude personal
  → codex/personal keeps 51 MB of unused database copies  aims doctor --fix

Every limit and when it resets, also for Antigravity, Grok, Kiro and OpenCode: aims usage
```

`aims status` is what is going on: each account's state, what a session does now, the sessions open now and, under **Next**, what is left to set up, each with the command that does it. `aims usage` has every number:

```text
$ aims usage
Claude Code
  personal       pro       5h     ━━━━━━    –                 7d     ━━━━━━  37%  resets 4d
  work           team      5h     ━━━━━━   2%  resets 4h10m   7d     ━━━━━━  84%  resets 4d

Codex
  personal       plus      5h     ━━━━━━   9%  resets 2h5m    7d     ━━━━━━  31%  resets 5d
  work           business  5h     ━━━━━━    –                 7d     ━━━━━━    –

Antigravity
  Gemini Models            5h     ━━━━━━   0%  resets 5h      7d     ━━━━━━   5%  resets 3d

Grok  x premium
  credits                  7d     ━━━━━━  26%  resets 47h

Kiro  pro
  credits                  month  ━━━━━━  42%  resets 12d     420 of 1000 credits

OpenCode
  my-team                  budget ━━━━━━  67%  resets 9d      $33.50 of $50
```

```bash
aims claude              # the active account (personal), or the next usable one
aims claude@work         # this account, for this session only
aims use work            # new sessions of both tools use work from now on
aims failover claude     # personal is out: mark it, switch, and carry on
aims claude --continue   # same conversation, now on the other account
```

Run `aims` with no arguments for a [dashboard](dashboard) that does all of this with a few keys; `tab` switches it between accounts and usage.

## How it works

```text
~/.claude/                          your normal Claude Code folder ("the hub")
  projects/  settings.json  skills/ ...
  .credentials.json                 your current login: profile "personal"

~/.aims/profiles/claude/work/       CLAUDE_CONFIG_DIR for the work account
  .credentials.json  .claude.json   its own login (private)
  projects -> ~/.claude/projects    everything else links to the hub
  settings.json -> ~/.claude/settings.json
  ...
```

When you start `aims claude@work`, aims sets `CLAUDE_CONFIG_DIR` to the work folder, repairs any link the tool replaced since the last run, and starts the real `claude`. Codex works the same way with `CODEX_HOME`. aims is not a proxy: it never sees your prompts or traffic, and the tools talk to their providers exactly as they do without it.

The details are in [How it works](how-it-works).

## Supported tools

| Tool | Login kept in | Notes |
| --- | --- | --- |
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `.credentials.json`, or the macOS keychain | Plan usage comes from its status line |
| [Codex CLI](https://github.com/openai/codex) | `auth.json`, or the OS keyring | Plan usage comes from its account API |

`aims usage` also shows the limits of Antigravity, Grok, Kiro and OpenCode, read through each tool's own CLI; aims does not run sessions on them yet.

Other CLIs can be added: each tool is one adapter behind a small interface. See [Adding a tool](how-it-works#adding-a-tool).

## Fair use

aims exists to keep work and personal apart. Many people have a work account and a personal one, and want each project to use the right one without logging in and out. aims runs only your own accounts, on your own machine. It starts sessions with the official Claude Code and Codex CLIs, and reads the limits of Antigravity, Grok, Kiro and OpenCode through each tool's own CLI and login (for Kiro, the same usage request `kiro-cli` makes). It is not a service: no one else's account passes through it, and it never shares, pools or resells access. It doesn't disguise itself or send the providers anything their own CLIs wouldn't. Each account stays under its provider's terms, and aims is not meant for stacking accounts to get around a plan's limits. aims is not affiliated with Anthropic, OpenAI, Google, xAI, Amazon or OpenCode.

## Where next

{{< cards >}}
  {{< card link="installation" title="Installation" icon="download" >}}
  {{< card link="quick-start" title="Quick start" icon="play" >}}
  {{< card link="failover" title="Limits and failover" icon="switch-horizontal" >}}
  {{< card link="commands" title="Commands" icon="terminal" >}}
{{< /cards >}}
