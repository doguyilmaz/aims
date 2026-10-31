---
title: What accounts share
weight: 6
aliases: ["/docs/sharing/"]
---

Each account (aims calls it a profile) has its own folder. The login in it is private; the rest links to the tool's normal folder, `~/.claude` or `~/.codex`, which aims calls the hub.

## Sharing modes

Choose per profile, when you create it or later in the [config file](../configuration):

| Mode | Shares | Keeps to itself | For |
| --- | --- | --- | --- |
| `all` (default) | history and settings | the login | one person with several subscriptions |
| `settings` | settings, skills, MCP servers | the login and conversation history | a client or employer whose conversations must stay apart |
| `none` | nothing | everything | a fully separate setup |

```bash
aims add claude client-x --share settings
aims login codex sandbox --share none
```

## Claude Code

| Shared | What it is | History |
| --- | --- | --- |
| `projects/` | conversations and auto memory, per project | yes |
| `file-history/`, `todos/`, `plans/`, `tasks/` | session state | yes |
| `history.jsonl` | your prompt history | yes |
| `settings.json`, `keybindings.json`, `CLAUDE.md` | your settings and global instructions | |
| `commands/`, `agents/`, `skills/`, `output-styles/`, `hooks/`, `plugins/`, `rules/` | your customizations | |

Private to each profile: `.credentials.json` (or the macOS keychain entry) and `.claude.json`, which holds the account, per-project trust and the MCP servers you added with `claude mcp add`. aims keeps the settings in it the same across your accounts: user MCP servers, and per project the trust, allowed tools, MCP servers and `.mcp.json` approvals. Before each launch and each move to another account, what you added, changed or removed in one account reaches the others. When two accounts changed the same setting, the newer edit wins, and trust given in one account is never taken back by another. aims takes Claude Code's own lock on the file and edits only these keys; the account fields are never read or copied. Log in to OAuth MCP servers (Atlassian, Figma, Slack) once per account: those tokens are kept with each login.

## Codex

| Shared | What it is | History |
| --- | --- | --- |
| `sessions/`, `archived_sessions/` | conversations | yes |
| `thread-writer-locks/` | prevents two processes from writing the same shared conversation | yes |
| `history.jsonl` | prompt history | yes |
| `memories/`, `state_*.sqlite`, `goals_*.sqlite` and similar | memory and session databases; aims also sets `CODEX_SQLITE_HOME` to the hub, so a database a new Codex adds is shared from its start | yes |
| `config.toml` | settings, MCP servers, profiles | |
| `AGENTS.md`, `prompts/`, `skills/`, `rules/`, `plugins/` | your instructions and customizations | |
| `.hook-trust/` | hooks you reviewed, so a moved session does not ask again | |

Private to each profile: `auth.json` (or the OS keyring entry). Daemon processes and their control sockets also remain private: sharing history does not mean sharing an authenticated running server.

## Work and personal accounts

By default a work and a personal account share everything above: conversations, settings, skills, MCP servers and project trust. `aims doctor` points out such a pair. If your company's rules keep work data out of a personal account, give the work profile `"share": "settings"` (its own conversations, everything else shared) or `"none"` (nothing shared) in `~/.aims/config.json`; see [Changing the mode later](#changing-the-mode-later).

Shared MCP servers reach every account with their settings, so keep tokens out of them: `${NAME}` in Claude Code's `.claude.json`, `env_vars` or `bearer_token_env_var` in Codex's `config.toml`. `aims doctor` names a server that has one written out.

## Keeping one entry private

To share everything except, say, Claude Code's `settings.json` for one account, list it under `exclude` in the config file:

```json
"work": { "exclude": ["settings.json"] }
```

The next launch replaces the link with a private copy.

## When a tool replaces a link

Tools often save a file by writing a new one and renaming it over the old, which turns aims' link into a plain file inside the profile. aims repairs that on every launch, before the tool starts:

- A file or folder that exists only in the profile moves to the hub, and the link comes back.
- Folders present in both are merged. Nothing is overwritten: when the same file differs, both copies are kept, the profile's with an `.aims-<profile>-<time>` suffix.
- For a plain file in both places, the newer one wins and the other is kept next to it as a backup.
- Prompt history files (`.jsonl`) are appended, not replaced.
- A database that may be open (its `-wal` file is not empty) is left alone until you close the tool and run `aims doctor --fix`.
- If both the profile and hub already have a Codex database with the same name, both are kept. For an index or log, Codex reads the hub's (`CODEX_SQLITE_HOME`) and doctor names the profile's copy as unused; for one that holds data, the profile keeps using its own and doctor reports incomplete history sharing.
- An existing private `thread-writer-locks/` directory is never merged: replacing a lock file could bypass a live writer's lock. aims sets it aside, on its own, only once no process has a file in it open. See [private writer locks](../troubleshooting#private-writer-locks).

`aims doctor` and `aims sync` run the same repair for every profile and report anything that needs a decision.

## Changing the mode later

Edit `share` in the config file. On the next launch, entries the profile should no longer share are replaced: settings with a private copy, history with an empty folder, so work conversations never copy into another account's folder. Going back to `all` merges history folders, but existing databases and writer lock directories may require the steps above. Close the tools before changing sharing modes.

Automatic moves require shared history on both profiles. A profile using `settings` or `none`, excluding a history entry, or still holding a private data database or writer lock directory cannot move a conversation to another account or receive one from it. aims checks existing links before choosing another profile and again before stopping the current session for a move.

## Runtime cost and disk use

Once links are in place, profiles access the same files. Normal synchronization checks the home directory's entries and link targets; it does not read database contents, walk conversation directories, copy history, or produce recurring backups. The move checks are also metadata-only and do not call a model or a provider API.

Repairs run when a tool starts or when you explicitly run `aims sync` or `aims doctor`; there is no background migration loop. If a tool replaces a link, the next repair may need to move or merge that entry. On the same filesystem, moves use rename rather than making a second copy. Across filesystems, a move must copy before removing the source. Comparing duplicate files and appending prompt history stream data with bounded memory.

Backups are retained for actual conflicts, not on every launch. Existing independent databases remain untouched until a deliberate migration reconciles them; aims cannot safely discard one merely because the other looks newer. Do not change sharing or migrate history while the affected tools are running.
