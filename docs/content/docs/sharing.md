---
title: What accounts share
weight: 6
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

Private to each profile: `.credentials.json` (or the macOS keychain entry) and `.claude.json`, which holds the account, per-project trust and the MCP servers you added with `claude mcp add`. A new profile starts with a copy of your MCP servers and project trust from the hub's `.claude.json`, without the account fields; `aims sync` copies servers you add later.

## Codex

| Shared | What it is | History |
| --- | --- | --- |
| `sessions/`, `archived_sessions/` | conversations | yes |
| `history.jsonl` | prompt history | yes |
| `memories/`, `state_*.sqlite`, `goals_*.sqlite` and similar | memory and session databases | yes |
| `config.toml` | settings, MCP servers, profiles | |
| `AGENTS.md`, `prompts/`, `skills/`, `rules/`, `plugins/` | your instructions and customizations | |

Private to each profile: `auth.json` (or the OS keyring entry).

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

`aims doctor` and `aims sync` run the same repair for every profile and report anything that needs a decision.

## Changing the mode later

Edit `share` in the config file. On the next launch, entries the profile should no longer share are replaced: settings with a private copy, history with an empty folder, so work conversations never copy into another account's folder. Going back to `all` merges the profile's own history into the hub.
