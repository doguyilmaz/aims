<p align="center">
  <img src="assets/logo.svg" width="96" height="96" alt="aims logo: a prompt whose cursor is coral and ivory">
</p>

<h1 align="center">aims</h1>

<p align="center">
  Several Claude Code and Codex accounts on one machine.<br>
  Switch instantly, share history, fail over when one hits its limit.
</p>

<p align="center">
  <a href="https://doguyilmaz.github.io/aims/">Documentation</a> ·
  <a href="https://doguyilmaz.github.io/aims/docs/installation/">Install</a> ·
  <a href="https://doguyilmaz.github.io/aims/docs/quick-start/">Quick start</a>
</p>

---

**aims** stands for **AI multi-session**. It lets you use a personal and a work account (or more) for Claude Code and Codex on one computer, without logging out and in to switch.

- **Log in once per account.** Each account keeps its own login in its own folder. Nothing is copied.
- **Sessions move on their own.** A session starts on your first account. At a usage limit it moves to the next one and continues the conversation, and it moves back once the first works again.
- **One history.** Conversations, settings, skills and MCP servers are shared, so a conversation started on one account continues on the other. Or keep an account's history separate.
- **Your AI can use it.** An MCP server and a skill let Claude Code and Codex check accounts, fail over, and hand a task to another account.

```text
$ aims status
Claude Code  ~/.claude
  ● personal   me@gmail.com     pro    ready           5h [##....]  38%   7d [#.....]  12%
    work       me@company.com   max    limited 2h10m   5h [######] 100%   7d [####..]  64%   5h resets 2h10m

Codex  ~/.codex
  ● personal   me@gmail.com     plus   ready           5h [#.....]   9%   7d [##....]  31%
    work       me@company.com   team   ready           5h [......]    –   7d [......]    –
```

## Install

```bash
brew install --cask doguyilmaz/tap/aims                                          # macOS, Linux
curl -fsSL https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.sh | sh
npm install -g @doguyilmaz/aims
go install github.com/doguyilmaz/aims/cmd/aims@latest
```

On Windows: `irm https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.ps1 | iex`, or Scoop. See [Installation](https://doguyilmaz.github.io/aims/docs/installation/).

## Start

```bash
aims init
```

The setup finds Claude Code and Codex, turns the login you already have into your first account, creates the second, asks which one sessions start on, and connects the tools to aims. Then start sessions the way you always do:

```bash
claude                     # or codex: aims picks the account and watches the session
```

With the shell integration (`aims setup` offers it), plain `claude` and `codex` go through aims. Without it, type `aims claude` and `aims codex`. Each session says what aims does with it when it starts, and Claude Code's status line shows it while it runs:

```text
› claude: personal (auto → work)
personal ▰▱▱▱▱  12% · 7d 31% · auto → work
```

| Start a session with | It starts on | At a limit |
| --- | --- | --- |
| `claude`, `aims claude` | your first account | moves to the next account, and back once the first works again |
| `aims claude@work` | work | stays on work (pinned) |

`aims` alone shows your accounts, the sessions open now and anything left to set up. `aims order` changes which account comes first, and `aims doctor` checks everything.

## How it works

Claude Code and Codex read their state from one folder (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`). aims gives each account its own folder holding only its login, and links everything else to your normal `~/.claude` and `~/.codex`. `aims claude` sets the variable and starts the real `claude`; aims is not a proxy and never sees your prompts. Details in [How it works](https://doguyilmaz.github.io/aims/docs/how-it-works/).

## Development

```bash
go test ./...                     # unit and end-to-end tests (stand-ins for claude and codex)
go run ./cmd/aims status
hugo server --source docs         # the documentation site
```

Each supported CLI is an adapter in `internal/tool/<name>`; adding one is described in [Adding a tool](https://doguyilmaz.github.io/aims/docs/how-it-works/#adding-a-tool).

## License

[MIT](LICENSE)
