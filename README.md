<p align="center">
  <img src="assets/logo.svg" width="96" height="96" alt="aims logo: two linked rings">
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
- **Switch instantly.** `aims use work` changes the account for new sessions; `aims claude@work` starts one right away.
- **One history.** Conversations, settings, skills and MCP servers are shared, so `aims claude --continue` picks up a conversation the other account started. Or keep an account's history separate.
- **Failover.** When an account is out of quota or its login died, the next session goes to the next account. Headless runs retry on their own when the failed attempt changed nothing.
- **Your AI can use it.** An MCP server and a skill let Claude Code and Codex check accounts, fail over, and hand a task to another account.

```text
$ aims status
Claude Code  ~/.claude
  ● personal   me@gmail.com · pro      ready           5h [###.....]  38%
    work       me@company.com · max    limited 2h10m   5h [########] 100%

Codex  ~/.codex
  ● personal   me@gmail.com · plus     ready           5h [#.......]   9%
    work       me@company.com · team   ready
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

The setup finds Claude Code and Codex, turns the login you already have into your first account, creates the second, and connects the tools to aims. Then:

```bash
aims                       # dashboard
aims claude                # Claude Code as the active account
aims claude@work           # as work, for this session
aims use work              # new sessions use work
aims failover claude --resume   # this account is out: switch and continue the conversation
aims doctor                # check everything
```

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
