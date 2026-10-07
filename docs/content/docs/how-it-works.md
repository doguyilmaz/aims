---
title: How it works
weight: 11
---

## One folder per login

Claude Code reads its state from `CLAUDE_CONFIG_DIR` (default `~/.claude`), Codex from `CODEX_HOME` (default `~/.codex`). Point a tool at another folder and it uses the login in that folder. aims gives each account such a folder and sets the variable when it starts the tool. It does not patch, wrap or proxy the tools: `aims claude` ends in the real `claude`, with your arguments, your terminal and your stdin.

The profile that adopted your existing login is special. It runs with the variable exactly as you had it, normally unset. This matters for Claude Code on macOS, which keeps the login in the keychain under a name derived from the exact `CLAUDE_CONFIG_DIR` string (`Claude Code-credentials` when unset, `Claude Code-credentials-<hash>` otherwise). Spelling out `~/.claude` would point at a different, empty login.

## Links to the hub

Everything except the login links from the profile folder to the tool's normal folder, the hub. A conversation saved under one account is therefore a file every account sees, which is why `--continue` works across accounts. The links are symlinks on macOS and Linux. On Windows they are symlinks when Developer Mode allows them, and otherwise junctions for folders and hard links for files.

Before each launch aims checks the profile's links and repairs any the tool replaced with a real file, merging content into the hub without overwriting anything. See [When a tool replaces a link](../sharing#when-a-tool-replaces-a-link).

## What aims reads and writes

| | Reads | Writes |
| --- | --- | --- |
| Logins | whether a login file (or keychain entry) exists; the e-mail and plan stored next to it | never; the tools write their own logins |
| Tool settings | Claude Code's `.claude.json` (MCP servers, project trust) to seed a new profile | the aims MCP server and status line entries, when you run `aims setup` |
| Your shell | nothing | one marked block in your rc file, when you run `aims setup --shell` |
| aims' own files | `~/.aims` | `~/.aims` |

The Codex login's ID token is decoded locally for the e-mail and plan name. It is never verified against or sent to anything.

## Choosing an account

For a new session aims takes the preferred profile (an explicit `@profile`, the terminal's pin, the active profile) and evaluates it:

1. Is there a login? (offline: a file check, or the keychain on macOS)
2. Is it marked as limited or logged out in `state.json`?
3. Does the last usage report have a window at or over the threshold that has not reset yet?

If any answer rules it out, aims evaluates the others in failover order and takes the first that passes. The checks are local and fast; no request goes to a provider unless you ask for a live check.

## Headless runs

For `claude -p` and `codex exec`, aims runs the tool with its output passing through a small analyzer. The analyzer reads the tool's structured events (or error lines), never the model's text, and answers three questions: did the run fail, was it a limit or a login error, and did it call any tool before failing. Standard output is held back for a few seconds so that a run that fails at once can be repeated on another account without mixing two answers. The rules are in [Limits and failover](../failover#during-a-headless-run).

Signals are passed on: closing the terminal or stopping aims stops the tool, and Ctrl+C reaches the tool directly, as it would without aims.

## Adding a tool

Each tool is an adapter: a Go package implementing `tool.Adapter`, listed in `internal/tools`. The rest of aims never names Claude Code or Codex.

```go
type Adapter interface {
	ID() ID                 // "claude"
	Title() string          // "Claude Code"
	Layout() Layout         // binary, home variable, what is shared, what is history
	Commands() Commands     // login, logout, resume

	IsHeadless(args []string) bool
	HeadlessArgs(HeadlessOptions) []string  // a run whose output shows tool calls
	NewAnalyzer(args []string) Analyzer     // reads that output

	Account(ctx, Home) Account  // offline: logged in? which e-mail and plan?
	Probe(ctx, Home) Probe      // online and cheap: does the login work, usage?

	MCPScope() MCPScope
	RegisterMCP(ctx, Home, name string, command []string) error
	UnregisterMCP(ctx, Home, name string) (bool, error)
}
```

Optional interfaces add a status line (`StatusLiner`), seeding a new profile from the hub (`Seeder`) and copying MCP servers into per-login lists (`MCPSyncer`). A conformance test in `internal/tools` holds every adapter to the same rules (logins never shared, the prompt always last and after `--`, and so on), and `internal/testutil/fake` shows how to stand in for a CLI in end-to-end tests.
