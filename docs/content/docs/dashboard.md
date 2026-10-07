---
title: Dashboard
weight: 8
---

`aims` with no arguments opens a full-screen view of every account. It runs in your terminal; nothing is served or opened in a browser.

```text
 aims  accounts for Claude Code and Codex                         v0.1.0

 Claude Code                                                    ~/.claude
 ▸ ● personal   me@gmail.com · pro      ready          5h ━━━━━━━━   38%
     work       me@company.com · max    limited 2h10m  5h ━━━━━━━━  100%

 Codex                                                           ~/.codex
   ● personal   me@gmail.com · plus     ready          5h ━━━━━━━━    9%
     work       me@company.com · team   ready

 ╭──────────────────────────────────────────────────────────────────────╮
 │ claude / personal  active · default login                            │
 │                                                                      │
 │ folder    ~/.claude                                                  │
 │ account   me@gmail.com · pro                                         │
 │ state     ready                                                      │
 │ 5h        ━━━━━━━━━━━━━━━━━━━━━━━━   38%  resets in 3h12m            │
 │ measured  4m ago                                                     │
 │ last used 4m ago                                                     │
 ╰──────────────────────────────────────────────────────────────────────╯

 enter make active • s start • l log in • f fail over • ? more • q quit
```

## Keys

| Key | Does |
| --- | --- |
| `↑` `↓` or `k` `j` | Move between accounts |
| `enter` or `u` | Make the selected account active for new sessions |
| `s` | Close the dashboard and start the tool as the selected account |
| `l` | Log the selected account in (the browser opens; the dashboard comes back after) |
| `f` | Mark the selected account as limited and activate the next one (asks first) |
| `c` | Clear the selected account's marks |
| `r` | Check every account live with its provider (see [costs](../failover#where-usage-numbers-come-from)) |
| `a` | Add an account (runs the setup) |
| `?` | Show all keys |
| `q`, `esc` | Quit |

The bars turn yellow at 70% and red at the failover threshold. The card under the list shows the selected account's folder, usage windows with their reset times, and when aims last heard from it.

When the output is not a terminal, `aims` prints the same information as `aims status` instead.
