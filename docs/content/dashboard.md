---
title: Dashboard
weight: 8
aliases: ["/docs/dashboard/"]
---

`aims` with no arguments opens a full-screen view of every account, the sessions open now and what is left to set up. It runs in your terminal; nothing is served or opened in a browser.

```text
 aims  accounts for Claude Code and Codex                                                  v0.1.3

 Claude Code                                                                            ~/.claude
 ▸ ● personal   me@gmail.com     pro    ready           5h ━━━━━━  38%   7d ━━━━━━  12%
     work       me@company.com   max    limited 2h10m   5h ━━━━━━ 100%   7d ━━━━━━  64%   5h resets 2h10m

 Codex                                                                                   ~/.codex
   ● personal   me@gmail.com     plus   ready           5h ━━━━━━   9%   7d ━━━━━━  31%
     work       me@company.com   team   ready           5h ━━━━━━    –   7d ━━━━━━    –
     new sessions: personal (auto → work)

 Open sessions
     Claude Code  personal          ~/src/app · 2h
     Codex        work      pinned  ~/src/api · 20m

 Next
   → codex in ~/src/api is pinned to work; for auto, restart it with  aims codex resume --last

 start: claude, codex or press s

 ╭──────────────────────────────────────────────────────────────────────────────╮
 │ claude / personal  active · default login                                    │
 │                                                                              │
 │ folder    ~/.claude                                                          │
 │ account   me@gmail.com · pro                                                 │
 │ state     ready                                                              │
 │ 5h        ━━━━━━━━━━━━━━━━━━━━━━━━  38%  resets in 3h12m                     │
 │ 7d        ━━━━━━━━━━━━━━━━━━━━━━━━  12%  resets in 4d                        │
 │ measured  4m ago                                                             │
 │ last used 4m ago                                                             │
 ╰──────────────────────────────────────────────────────────────────────────────╯

 s start a session • enter make first • l log in • a add account • tab usage • ? more • q quit
```

## Keys

| Key | Does |
| --- | --- |
| `↑` `↓` or `k` `j` | Move between accounts |
| `s` | Close the dashboard and start a session of the selected tool. aims picks the account and moves the session at a limit |
| `enter` or `u` | Make the selected account the one sessions start on in this project (or folder) |
| `shift+↑` `shift+↓` or `K` `J` | Move the selected account up or down the [failover order](../failover#order); the top one is active. This changes the order of the project (or folder) you opened the dashboard in, named at the top |
| `g` | Use the order shown for every folder without an order of its own |
| `x` | Drop this project's own order; it follows the order of every folder again |
| `l` | Log the selected account in (the browser opens; the dashboard comes back after) |
| `f` | Mark the selected account as limited, so new sessions use the next one (asks first) |
| `c` | Clear the selected account's marks |
| `r` | Check every account live with its provider (see [costs](../failover#where-usage-numbers-come-from)) |
| `a` | Add an account (runs the setup) |
| `tab` | Switch to the usage view and back |
| `?` | Show all keys |
| `q`, `esc` | Quit |

**Open sessions** are the sessions aims started that still run; `pinned` or `auto off` marks one that stays on its account at a limit. **Next** lists what is still missing for sessions to move on their own (a second account, a login, plain `claude` going through aims), each with the command that does it.

The bars turn yellow at 70% and red at the failover threshold. The card under the list shows the selected account's folder, usage windows with their reset times, and when aims last heard from it.

`tab` switches to the usage view: the same table as [`aims usage`](../commands#aims-usage-tool), every window with its reset time, also for Antigravity, Grok, Kiro and OpenCode. `r` there checks every account and tool live; `tab` goes back.

When the output is not a terminal, `aims` prints the same information as `aims status` instead.
