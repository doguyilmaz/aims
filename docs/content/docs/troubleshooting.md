---
title: Troubleshooting
weight: 13
---

## Start with doctor

```bash
aims doctor
```

```text
aims
  ✓ aims v0.1.0 ~/.local/bin/aims
  ✓ config ~/.aims/config.json

Claude Code
  ✓ 2.1.0 /opt/homebrew/bin/claude
  ✓ claude/personal me@gmail.com
  ! claude/work: not logged in
    → aims login claude work

Codex
  ✓ 0.200.0 /usr/local/bin/codex
  ✓ codex/personal me@gmail.com
  ✓ codex/work me@company.com

Integrations
  ✓ shell integration in ~/.zshrc
  ✓ Claude Code skill
  ✓ Codex skill
```

It checks the tools, every login and shared folder, and the integrations, repairs shared links as it goes, and prints the command for anything left. It exits with 1 when something needs you, so it also works in scripts.

## Common problems

### Both profiles log in as the same account

The browser signed in with whatever account it was already logged into. aims warns when it sees this (`me@gmail.com is already used by personal`). Log the second profile in again from a private window, or let Claude Code pre-fill the address:

```bash
aims login claude work -- --email me@company.com
aims login codex work -- --device-auth      # a code you enter in any browser
```

### "ignoring ANTHROPIC_API_KEY from your shell"

An API key in your shell would override every profile's login, so aims removes it for the tool it starts. To use an API key on purpose, give it its own profile with the key in [`env`](../configuration#profiles).

### Plain `claude` ignores the active account

The [shell integration](../everyday#plain-claude-and-codex) is missing or not loaded yet. Run `aims setup --shell` and open a new terminal. `type claude` should say it is a function.

### A profile shows "limited" but works again

The limit reset early, or the mark was a mistake. `aims clear claude work` removes it, and `aims status --live` checks with the provider.

### "shared 'settings.json': points to …, not the shared folder"

The entry in the profile is a link aims did not make, for example from a dotfiles tool. aims leaves it alone. Remove it if it should be shared, or list it under [`exclude`](../sharing#keeping-one-entry-private) to keep it.

### "the database is open; close the tool and run `aims doctor --fix`"

Codex keeps some state in SQLite databases. aims moves a database into the shared folder only when it is closed. Quit Codex and run `aims doctor --fix`.

### Windows: links are hard links or junctions

Without Developer Mode, Windows does not let normal users create symlinks, so aims uses junctions for folders and hard links for files. Both work for sharing. Turning Developer Mode on makes later links symlinks.

### The MCP tools do not show up

Restart the session after `aims setup`. In Claude Code, `/mcp` lists the servers; `claude mcp list` should include `aims`. If aims is installed somewhere not on the `PATH` that Claude Code sees, run `aims setup` again: it registers the full path in that case.

## Starting over

```bash
aims clean      # fresh start: logins and account folders stay; then aims init
aims wipe       # also logs out and deletes the accounts aims created
```

Neither touches `~/.claude` or `~/.codex`: every conversation, session, skill, setting and your main logins stay.

## Getting help

Open an [issue](https://github.com/doguyilmaz/aims/issues) with the output of `aims --version` and `aims doctor`. Remove e-mail addresses you would rather not post.
