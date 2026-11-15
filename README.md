<p align="center">
  <img src="assets/logo.svg" width="96" height="96" alt="aims logo: a prompt whose cursor is coral and ivory">
</p>

<h1 align="center">aims</h1>

<p align="center">
  Separate logins for Claude Code and Codex.<br>
  Session routing in your terminal, usage limits in your macOS menu bar and Edge Dock.
</p>

<p align="center">
  <a href="https://aims.apps.doguyilmaz.com">Documentation</a> ·
  <a href="https://aims.apps.doguyilmaz.com/installation">Install</a> ·
  <a href="#aims-usage-for-macos">macOS app</a> ·
  <a href="https://aims.apps.doguyilmaz.com/quick-start">Quick start</a>
</p>

---

**aims** stands for **AI multi-session**. It lets you use a personal and a work account (or more) for Claude Code and Codex on one computer, without logging out and in to switch.

| Interface | What it does | Get started |
| --- | --- | --- |
| **aims CLI** — macOS, Linux, Windows | Manage accounts, share or isolate history, route sessions and inspect usage | [Install the CLI](#install) |
| **Aims Usage** — macOS | Watch provider quotas, resets and budgets in the menu bar, Edge Dock and overview | [App preview and setup](#aims-usage-for-macos) |

- **Log in once per account.** Each account keeps its own login in its own folder. Nothing is copied.
- **Sessions move on their own.** A session starts on your first account. At a usage limit it moves to the next one and continues the conversation, and it moves back once the first works again.
- **One history.** Conversations, settings, skills and MCP servers are shared, so a conversation started on one account continues on the other. Or keep an account's history separate.
- **Your AI can use it.** An MCP server and a skill let Claude Code and Codex check accounts, fail over, and hand a task to another account.

## Aims Usage for macOS

**Source preview.** The app can be built from this repository. Public signed app downloads, the `aims-usage` Homebrew cask and the Sparkle update channel have not been activated yet. The existing `doguyilmaz/tap/aims` package installs the CLI and its keepalive wrapper.

**One app, three surfaces:** a menu-bar popover for a quick check, an optional Edge Dock for an always-nearby reading, and an overview you can pin open. The Edge Dock can use either edge, Standard or Mini size, vertical positioning, hover-to-show and a direct pin button. Choose Compact or Gauges in Settings.

<table>
  <tr>
    <th>Gauges overview</th>
    <th>Edge Dock</th>
  </tr>
  <tr>
    <td align="center"><img src="docs/static/images/aims-usage-overview.jpg" width="340" alt="Aims Usage Gauges overview with illustrative provider quotas and reset countdowns"></td>
    <td align="center"><img src="docs/static/images/aims-usage-edge-dock.jpg" width="64" alt="Aims Usage Edge Dock with six provider indicators"></td>
  </tr>
</table>

*Screenshots use illustrative preview data. Model groups and reset inventory appear only when reported by the provider.*

- Claude and Codex accounts stay separate. Choose All or one account for each Dock indicator without changing the active CLI login.
- Choose the Dock's allowance per provider — 5 hours, weekly, monthly or budget — and show Used or Remaining. Hover retains all account readings.
- Kiro, Grok, Antigravity model groups and OpenCode budgets use the same usage data as the CLI. Scheduled resets, reported banked resets and personal reminders remain distinct.
- Refresh reads quotas without a model prompt. Missing reset times say **Not reported**; a passed reset waits for a new measurement instead of assuming zero.

Build on macOS 14.0.0+ with Swift 6.2.0+ and the Go version declared in [go.mod](go.mod) available on `PATH`:

```sh
scripts/build-macos-app.sh
open 'dist/Aims Usage.app'
```

For daily use, quit the development app and move the bundle to your Applications folder. It includes its own aims executable and reads your existing aims profiles and native provider logins. Use the CLI to set up/manage Claude and Codex accounts; select an existing custom `AIMS_HOME` in the app's Settings when needed. Provider CLIs retain their own installation and authentication.

See the [macOS app guide](docs/content/macos-usage.md) for setup, account/window controls and data-source limits, or [release operations](docs/macos-release.md) for the prepared signed ZIP, separate cask and Sparkle channel. Updating or removing the app preserves account data, sessions and history.

## Accounts and usage in the terminal

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

`aims` alone opens a [dashboard](https://aims.apps.doguyilmaz.com/dashboard) with both views (`tab` switches) and keys for everything else.

`aims usage --refresh` refreshes quotas without sending a model prompt. Claude reads its existing profile's OAuth usage API without renewing or rewriting the login; if access has expired, use that account in Claude Code and retry. Native status-line reports also update its cache. `--live` additionally checks Claude model access with a one-word prompt. `aims usage --json` exports a versioned usage snapshot, including provider-reported reset times and Codex banked reset counts. Unknown reset details stay unknown; a past reset time asks for a new measurement rather than assuming a full allowance.

## Install

These methods install the **CLI**. The macOS app has its own [preview setup](#aims-usage-for-macos).

```bash
brew install --cask doguyilmaz/tap/aims                                          # macOS, Linux
curl -fsSL https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.sh | sh
npm install -g @doguyilmaz/aims
go install github.com/doguyilmaz/aims/cmd/aims@latest
```

On Windows: `irm https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.ps1 | iex`, or Scoop. See [Installation](https://aims.apps.doguyilmaz.com/installation).

Check an existing installation with `aims doctor --check` (or `--json` for scripts), then use `aims doctor --fix` for conservative repairs. Independent settings and databases stay; unused index/log copies are archived with their recovery files. Login inspection is explicit with `aims doctor --logins`. See [safe recovery](docs/content/troubleshooting.md).

A stricter sharing policy must be applied before aims starts new work for that profile. Purge and wipe also retain a profile when native logout cannot complete, including when its folder is missing but a system keyring login may remain.

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

## Editors and orchestrators

```bash
aims bridge                         # read-only: compare actual homes and shared history
aims exec -- my-orchestrator         # pass aims' CLI accounts to another program
aims bridge install                 # dedicated executable paths for IDEs and SDKs
aims bridge commands --json          # CLI, app-server and ACP command/argument arrays
aims bridge acp claude               # use an installed upstream ACP adapter
```

CLI, SDK, app-server and ACP clients can use the same aims accounts without copying credentials. Protocol launches pass native streams through and leave running sessions to the client. ACP adapters keep their own bundled CLI versions; dedicated launchers let apps such as Cline choose one executable path. An app's managed login and thread list can still be separate. See [Other coding apps](https://aims.apps.doguyilmaz.com/other-apps) for setup, and the [source-backed compatibility survey](https://aims.apps.doguyilmaz.com/compatibility) for Zed, Cline, Paradev, Vibe Kanban, cmux and other integration types. T3's optional settings export remains available through `aims bridge t3`.

## How it works

Claude Code and Codex read their state from one folder (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`). aims gives each account its own login folder and links the configured shared parts to your normal `~/.claude` and `~/.codex`. `aims claude` sets the variable and starts the real `claude`. The official CLI handles model requests; aims supplies launch environment and session routing. Details in [How it works](https://aims.apps.doguyilmaz.com/how-it-works).

## Development

```bash
go test ./...                     # unit and end-to-end tests (stand-ins for claude and codex)
go run ./cmd/aims status
hugo server --source docs         # the documentation site
```

Each supported CLI is an adapter in `internal/tool/<name>`; adding one is described in [Adding a tool](https://aims.apps.doguyilmaz.com/how-it-works#adding-a-tool).

## Fair use

aims exists to keep work and personal apart. Many people have a work account and a personal one, and want each project to use the right one without logging in and out. aims runs your own accounts on your own machine. Sessions use the official Claude Code and Codex CLIs; quota readers use native CLIs and first-party usage APIs with each provider's existing login. It does not share, pool or resell access. Each account stays under its provider's terms, and aims is not meant for stacking accounts to get around a plan's limits. aims is not affiliated with Anthropic, OpenAI, Google, xAI, Amazon or OpenCode.

## License

[MIT](LICENSE)
