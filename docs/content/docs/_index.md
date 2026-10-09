---
title: Introduction
weight: 1
---

aims (short for **AI multi-session**) runs several Claude Code and Codex accounts on one machine. A personal and a work subscription, say, or two work seats. Each account keeps its own login; everything else is shared. You switch accounts without logging out, and when one account runs out of quota the next session moves to another.

## The problem

Claude Code and Codex keep one login per machine. To use a second account you log out, log in with the other one in the browser, and do it again to switch back. Your conversations and settings stay with whichever folder the tool happens to use, and a limit in the middle of a task means stopping until it resets.

Both tools can be pointed at a different config folder (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`), which keeps logins apart. But then each folder starts empty: no history, no settings, no skills, no MCP servers, and nothing to tell you which folder is which.

aims does the bookkeeping. It gives every account its own folder, links the shared parts back to your normal `~/.claude` and `~/.codex`, remembers which account is active, and watches for limits and dead logins.

## How it looks

```text
$ aims status
Claude Code  ~/.claude
  ● personal   me@gmail.com     pro    ready           5h [##....]  38%   7d [#.....]  12%
    work       me@company.com   max    limited 2h10m   5h [######] 100%   7d [####..]  64%   5h resets 2h10m

Codex  ~/.codex
  ● personal   me@gmail.com     plus   ready           5h [#.....]   9%   7d [##....]  31%
    work       me@company.com   team   ready           5h [......]    –   7d [......]    –
```

```bash
aims claude              # the active account (personal), or the next usable one
aims claude@work         # this account, for this session only
aims use work            # new sessions of both tools use work from now on
aims failover claude     # personal is out: mark it, switch, and carry on
aims claude --continue   # same conversation, now on the other account
```

Run `aims` with no arguments for a [dashboard](dashboard) that does all of this with a few keys.

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

Other CLIs can be added: each tool is one adapter behind a small interface. See [Adding a tool](how-it-works#adding-a-tool).

## Where next

{{< cards >}}
  {{< card link="installation" title="Installation" icon="download" >}}
  {{< card link="quick-start" title="Quick start" icon="play" >}}
  {{< card link="failover" title="Limits and failover" icon="switch-horizontal" >}}
  {{< card link="commands" title="Commands" icon="terminal" >}}
{{< /cards >}}
