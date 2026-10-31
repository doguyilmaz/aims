<p align="center">
  <img src="assets/logo.svg" width="96" height="96" alt="aims logo: a prompt whose cursor is coral and ivory">
</p>

<h1 align="center">aims</h1>

<p align="center">
  Several Claude Code and Codex accounts on one machine.<br>
  Switch instantly, share history, fail over when one hits its limit.
</p>

<p align="center">
  <a href="https://aims.apps.doguyilmaz.com">Documentation</a> ·
  <a href="https://aims.apps.doguyilmaz.com/installation">Install</a> ·
  <a href="https://aims.apps.doguyilmaz.com/quick-start">Quick start</a>
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
  order of this project (~/code/client-app); elsewhere personal → work
  ● personal   me@gmail.com     pro        login expired   5h ━━━━━━    –   7d ━━━━━━  37%  marked by a session
    work       me@company.com   team       ready           5h ━━━━━━   2%   7d ━━━━━━  84%   7d resets 4d  default login, standing in for personal

Codex  ~/.codex
  ● personal   me@gmail.com     plus       ready           5h ━━━━━━   9%   7d ━━━━━━  31%
    work       me@company.com   business   ready           5h ━━━━━━    –   7d ━━━━━━    –  default login

Open sessions
  Claude Code  work      auto  ~/code/client-app · 12h59m ago
  Claude Code  work      auto  ~/code/api · 2h4m ago
  Codex        personal  auto  ~/code/blog · 6m ago

Next
  → claude/personal is not logged in  aims login claude personal
  → codex/personal keeps unused database copies (51 MB) that can go  aims doctor

How it works
  claude and codex start on personal. At a limit or a dead login the
  conversation continues on work, and goes back to personal once it works again.
  claude/personal cannot be used now (login expired); new claude sessions start
  on work.
  aims claude@work keeps a session on work and never moves it.
```

## Install

```bash
brew install --cask doguyilmaz/tap/aims                                          # macOS, Linux
curl -fsSL https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.sh | sh
npm install -g @doguyilmaz/aims
go install github.com/doguyilmaz/aims/cmd/aims@latest
```

On Windows: `irm https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.ps1 | iex`, or Scoop. See [Installation](https://aims.apps.doguyilmaz.com/installation).

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

`aims` alone shows your accounts, the sessions open now and anything left to set up. `aims order` changes which account comes first (in this project; `--global` for every folder), and `aims doctor` checks everything.

## How it works

Claude Code and Codex read their state from one folder (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`). aims gives each account its own folder holding only its login, and links everything else to your normal `~/.claude` and `~/.codex`. `aims claude` sets the variable and starts the real `claude`; aims is not a proxy and never sees your prompts. Details in [How it works](https://aims.apps.doguyilmaz.com/how-it-works).

## Development

```bash
go test ./...                     # unit and end-to-end tests (stand-ins for claude and codex)
go run ./cmd/aims status
hugo server --source docs         # the documentation site
```

Each supported CLI is an adapter in `internal/tool/<name>`; adding one is described in [Adding a tool](https://aims.apps.doguyilmaz.com/how-it-works#adding-a-tool).

## Fair use

aims exists to keep work and personal apart. Many people have a work account and a personal one, and want each project to use the right one without logging in and out. aims runs only your own accounts, on your own machine. It starts sessions with the official Claude Code and Codex CLIs, and reads the limits of Antigravity, Grok, Kiro and OpenCode through each tool's own CLI and login (for Kiro, the same usage request `kiro-cli` makes). It is not a service: no one else's account passes through it, and it never shares, pools or resells access. It doesn't disguise itself or send the providers anything their own CLIs wouldn't. Each account stays under its provider's terms, and aims is not meant for stacking accounts to get around a plan's limits. aims is not affiliated with Anthropic, OpenAI, Google, xAI, Amazon or OpenCode.

## License

[MIT](LICENSE)
