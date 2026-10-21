---
title: Dashboard
weight: 8
---

`aims` with no arguments opens a full-screen view of every account, the sessions open now and what is left to set up. It runs in your terminal; nothing is served or opened in a browser.

```text
 aims  accounts for Claude Code and Codex                                                  v0.1.3

 Claude Code                                                                            ~/.claude
 ▸ ● personal   me@gmail.com     pro    ready           5h [##....]  38%   7d [#.....]  12%
     work       me@company.com   max    limited 2h10m   5h [######] 100%   7d [####..]  64%   5h resets 2h10m

 Codex                                                                                   ~/.codex
   ● personal   me@gmail.com     plus   ready           5h [#.....]   9%   7d [##....]  31%
     work       me@company.com   team   ready           5h [......]    –   7d [......]    –
     sessions start on personal; at a limit they move to work, and back once personal works again

 Open sessions
     Claude Code  personal  moves at a limit       ~/src/app · 2h
     Codex        work      pinned, stays on work  ~/src/api · 20m

 Next
   → Codex in ~/src/api is pinned to work: it stays there at a limit. To let aims move it, end it and run  aims codex resume --last

 Start a session with claude or codex in a terminal, or press s: aims picks the account and moves the session at a limit

 ╭──────────────────────────────────────────────────────────────────────────────╮
 │ claude / personal  active · default login                                    │
 │                                                                              │
 │ folder    ~/.claude                                                          │
 │ account   me@gmail.com · pro                                                 │
 │ state     ready                                                              │
 │ 5h        [#########...............]  38%  resets in 3h12m                   │
 │ 7d        [###.....................]  12%  resets in 4d                      │
 │ measured  4m ago                                                             │
 │ last used 4m ago                                                             │
 ╰──────────────────────────────────────────────────────────────────────────────╯

 s start a session • enter make first • l log in • a add account • ? more • q quit
```

## Keys

| Key | Does |
| --- | --- |
| `↑` `↓` or `k` `j` | Move between accounts |
| `s` | Close the dashboard and start a session of the selected tool. aims picks the account and moves the session at a limit |
| `enter` or `u` | Make the selected account the one sessions start on |
| `shift+↑` `shift+↓` or `K` `J` | Move the selected account up or down the [failover order](../failover#order); the top one is active |
| `l` | Log the selected account in (the browser opens; the dashboard comes back after) |
| `f` | Mark the selected account as limited, so new sessions use the next one (asks first) |
| `c` | Clear the selected account's marks |
| `r` | Check every account live with its provider (see [costs](../failover#where-usage-numbers-come-from)) |
| `a` | Add an account (runs the setup) |
| `?` | Show all keys |
| `q`, `esc` | Quit |

**Open sessions** are the sessions aims started that still run, with whether each moves at a limit. **Next** lists what is still missing for sessions to move on their own (a second account, a login, plain `claude` going through aims), each with the command that does it.

The bars turn yellow at 70% and red at the failover threshold. The card under the list shows the selected account's folder, usage windows with their reset times, and when aims last heard from it.

When the output is not a terminal, `aims` prints the same information as `aims status` instead.
