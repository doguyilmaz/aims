# aims: several Claude Code and Codex accounts on one machine

`aims` (AI Multi-Session) lets you use a **personal** and a **work** account (or
more) for **Claude Code** and **Codex** on one computer:

- **Log in once per account.** No more logout/login to switch.
- **Switch instantly** with `aims use work`, per terminal or globally.
- **Shared history.** Transcripts, prompt history, settings, skills, plugins and
  MCP config are shared, so you can continue a conversation on the other account.
- **Failover.** When an account hits its usage limit or its login dies, aims
  moves to the next account. It also tells you which account needs to log in again.
- **Your AI can use it too.** An MCP server and a skill let Claude or Codex check
  accounts, switch them, fail over, or run a task with the other account.

Written in TypeScript for [Bun](https://bun.sh), with no runtime dependencies. It also compiles to a
single self-contained binary (`bun run build`) that needs neither Bun nor Node.

## How it works

Both CLIs read their whole state from one directory: `CLAUDE_CONFIG_DIR`
(default `~/.claude`) and `CODEX_HOME` (default `~/.codex`). aims gives every
account its own directory, which holds only that account's login. Everything
else is a link back to the normal directory (the "hub"):

```
~/.claude/                      <- hub: your normal Claude home (shared stuff lives here)
  projects/  history.jsonl  settings.json  skills/  plugins/  ...
  .credentials.json             <- login of the account adopted with --existing

~/.aims/profiles/claude/work/   <- CLAUDE_CONFIG_DIR for "work"
  .credentials.json  .claude.json  remote-settings.json   (private: login + org policy)
  projects -> ~/.claude/projects        (shared)
  settings.json -> ~/.claude/settings.json
  ...
```

`aims claude` starts the real `claude` with the right `CLAUDE_CONFIG_DIR`, the
same way `aims codex` sets `CODEX_HOME`. Logins are never copied or moved. Both
providers rotate refresh tokens, so two copies of one login eventually log each
other out. Copying login files is the usual cause of "I keep having to log in again".

| Shared across accounts | Private per account |
| --- | --- |
| Claude: `projects/` (transcripts + auto-memory), `history.jsonl`, `settings.json`, `CLAUDE.md`, `skills/`, `agents/`, `commands/`, `plugins/`, `hooks/`, `rules/`, `output-styles/`, `file-history/`, `todos/`, `plans/`, `tasks/`, `keybindings.json` | Claude: `.credentials.json` / macOS keychain entry, `.claude.json` (account, trust, user MCP servers*), org `remote-settings.json` / `policy-limits.json` |
| Codex: `sessions/`, `archived_sessions/`, `history.jsonl`, `config.toml` (incl. MCP servers), `AGENTS.md`, `prompts/`, `skills/`, `rules/`, `plugins/`, `memories/`, thread/memory databases (`state_N.sqlite`, ...) | Codex: `auth.json` / keyring entry, logs |

\* New Claude profiles start with a copy of your user MCP servers, project
trust and allowed tools, but no account data. `aims sync` copies MCP servers
you add later.

If a tool replaces a link with a real file (for example an atomic save of `settings.json`),
the next `aims` command moves the newer content into the hub and restores the
link. The older version is kept next to it as `*.aims-previous-*`.

## Install

```sh
bun add -g github:doguyilmaz/ai-multi-session    # needs Bun >= 1.2
aims --version
```

Or as a standalone binary (no runtime needed on the machine that runs it):

```sh
git clone https://github.com/doguyilmaz/ai-multi-session && cd ai-multi-session
bun install && bun run build          # -> dist/aims, put it on your PATH
# other platforms: bun build --compile --target=bun-darwin-arm64 src/cli.ts --outfile aims
```

## Quick start

```sh
# 1. The account you are logged in with today becomes a profile (nothing moves)
aims add claude personal --existing
aims add codex  personal --existing

# 2. Log in the other account once (opens the browser)
aims login claude work -- --email you@company.com   # --email pre-fills the login page
aims login codex  work -- --device-auth             # device code: open it in the right browser profile

# 3. Optional but recommended
aims setup --statusline           # skill + MCP server for Claude & Codex, Claude status line
echo 'eval "$(aims shell-init)"' >> ~/.zshrc   # plain `claude` / `codex` follow the active profile

aims                              # see everything
```

> Tip: the browser login uses whichever claude.ai / ChatGPT account the browser
> is signed into. Use a private window for the second account. `aims` warns you
> if two profiles end up on the same account.

## Daily use

```sh
aims use work                # both tools -> work (from now on, every new session)
aims use claude personal     # just one tool
claude                       # with shell-init: runs as the active profile
aims claude                  # same without shell-init
aims claude@work -p "..."    # one-off as a specific profile (also codex@work)

eval "$(aims env work)"      # pin only THIS terminal to work (sets CLAUDE_CONFIG_DIR / CODEX_HOME)
eval "$(aims env --reset)"   # un-pin
```

Sessions on different accounts can run at the same time, side by side.

`aims` with no arguments shows the status:

```
Claude Code  shared: ~/.claude
  * personal  me@gmail.com · pro (default login)  ok            5h 42% · 7d 13%
    work      me@company.com · team               limit for 2h  5h 100% (resets 2h)
Codex  shared: ~/.codex
  * personal  me@gmail.com · plus (default login) ok
    work      not logged in                       not logged in
```

## Failover

aims fails over at three levels:

1. **Before launching.** `aims claude` / `aims codex` skips an account that is
   cooling down, logged out, or at ≥95% of its 5-hour/weekly window. Usage
   comes from the Claude status line (`aims setup --statusline`) and from
   `aims status --live`. It then launches the next account in `aims order` and
   prints which one it used.
2. **Headless runs** (`claude -p`, `codex exec`/`review`). A usage-limit or
   login error marks that account, so the next run goes elsewhere. aims
   **retries the same run** on the next account only when the failed attempt
   provably did nothing: no tool call in `--output-format stream-json` /
   `codex exec --json` output, or a login error. A run that may already have
   pushed, written or deployed something is never repeated behind your back.
   Output of a retried attempt is held back, so scripts only see the good run.
3. **Interactive sessions** can't be switched mid-run. When you hit a limit:
   ```sh
   aims failover claude --resume   # mark current as limited, activate next, continue the same conversation
   ```
   "Current" is the account aims launched last (which may differ from the
   active one after an automatic skip); `--from <name>` picks another. A reset
   time that is already known is never extended.
   Because transcripts are shared, `claude --continue` / `codex resume --last` picks
   up the conversation you just left, now on the other account.

Accounts that need a fresh login show up as `login expired` / `not logged in` in
`aims status`. `aims status --live` checks with the providers. For Codex it uses
the account API and costs nothing. For Claude it sends a one-word Haiku prompt.
Fix with `aims login <tool> <profile>`; `aims clear <tool> [profile]` drops a mark.

Settings live in `~/.aims/config.json`:

```json
{ "failover": { "auto": true, "cooldownMinutes": 300, "threshold": 95 } }
```

## Letting the AI switch

`aims setup` installs:

- the **skill** `aims` into `~/.claude/skills` and `~/.codex/skills` (shared by all profiles), and
- the **MCP server** `aims mcp` into every Claude login and the shared Codex `config.toml`.

Claude-only alternative: the repo is also a plugin marketplace.

```
/plugin marketplace add doguyilmaz/ai-multi-session
/plugin install aims@ai-multi-session
```

(The plugin starts the server with `bun`. Use either `aims setup --tools codex` + the plugin, or `aims setup`; doing both gives Claude two copies of the server.)

MCP tools: `aims_status`, `aims_switch`, `aims_failover`, `aims_clear`,
`aims_run`, `aims_login_help`.

What the AI can and cannot do:

- A **running** session keeps the account it started with. Neither CLI can swap
  credentials mid-conversation. The session's account is in `AIMS_SESSION_PROFILE`.
- `aims_switch` / `aims_failover` change the account for **new** sessions immediately.
- `aims_run` uses another account **right now** for a self-contained task
  (`claude -p` / `codex exec` under that profile, with failover), e.g. "ask
  Codex on my work account to review this diff". It returns the final answer.
- Logging in needs a browser. The AI hands you the exact command (`aims_login_help`).

## Commands

```
aims add <tool> <name> [--existing] [--isolated] [--dir d]
                                                   create profile (--isolated: share nothing)
aims login | logout <tool> <name> [-- args]        browser login for that profile
aims rm <tool> <name> [--purge [--force]]          forget profile (--purge deletes its dir after moving
                                                   anything shared into the hub; never shared data)
aims use [tool] <name>                             set active profile
aims order <tool> <names...>                       failover order
aims [status|ls] [tool] [--live] [--json]          overview
aims <tool>[@name] [args]   /  aims run ...        launch through aims
aims env [tool] [name] [--shell sh|fish|powershell] [--reset]
aims failover <tool> [--resume] [--to n] [--minutes m] [--reason r]
aims clear <tool> [name]
aims sync [tool]                                   repair links, copy MCP servers to Claude profiles
aims doctor [--fix]
aims setup [--statusline] [--tools claude,codex] [--no-skill] [--no-mcp]
aims shell-init [bash|zsh|fish|powershell]
aims mcp | statusline                              used by the tools themselves
```

Environment: `AIMS_HOME` (default `~/.aims`), `AIMS_CLAUDE_BIN` / `AIMS_CODEX_BIN`
(binary override), `AIMS_CLAUDE_PROFILE` / `AIMS_CODEX_PROFILE` (terminal pin, set by `aims env`).

A profile can also carry environment variables, for example an API-key account:
`"tools": {"claude": {"profiles": {"api": {"env": {"ANTHROPIC_API_KEY": "..."}}}}}`.
Otherwise aims drops `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`,
`CLAUDE_CODE_OAUTH_TOKEN` and `CODEX_API_KEY` from your shell when launching,
because they would override the profile's login.

## Notes and limits

- **Work vs. personal data.** Shared transcripts mean a conversation can move
  between accounts. If your employer requires separation, create that profile
  with `--isolated` (nothing shared). You can also exclude single entries:
  `"exclude": ["projects", "history.jsonl"]` on the profile in `config.json`.
  Exclude `settings.json` / `config.toml` too if one account needs different
  login settings (`forceLoginOrgUUID`, `forced_chatgpt_workspace_id`, ...).
  Turning either on later takes effect on the next `aims` run: settings-type
  entries become a private copy, conversation data starts empty.
- `config.json` is meant to be hand-edited; if it has a JSON error, aims stops
  and tells you instead of starting over.
- **macOS.** Claude Code keeps each login in the keychain under
  `Claude Code-credentials-<hash of CLAUDE_CONFIG_DIR>`. aims always passes the
  same absolute path, so each profile keeps its own entry. The `--existing`
  profile runs with `CLAUDE_CONFIG_DIR` unset, which keeps your current entry.
- **Windows.** Directory links are junctions (no admin needed). File links fall
  back to hard links without Developer Mode. Use `aims shell-init powershell`.
- The usage numbers come from fields that Claude Code passes to status-line
  commands, and from Codex's (experimental) `app-server` account API. If a
  future version changes them, aims falls back to cooldowns from actual
  limit errors.
- Tested on Linux with Claude Code 2.1.291 and Codex 0.160.1 (`bun test` runs
  the suite against a stand-in `claude`). The macOS keychain naming was checked
  against Claude Code 2.1.291's code, but the macOS and Windows code paths have
  not been run on those systems yet.

## Development

```sh
bun install
bun run check        # tsc (strict) + bun test
bun src/cli.ts ...   # run from source
```

`src/` layout: `cli.ts` (commands), `run.ts` (launch + failover), `health.ts`
(logins, usage, live checks), `links.ts` (shared folders), `analyze.ts`
(reading headless output safely), `proc.ts` (Bun.spawn wrappers), `mcp.ts`,
`statusline.ts`, `ops.ts`, `config.ts`.

## Uninstall

```sh
aims rm claude work --purge      # per profile; shared data in ~/.claude / ~/.codex stays
claude mcp remove aims -s user   # per login, if you used `aims setup`
rm -rf ~/.aims && bun remove -g ai-multi-session
```
