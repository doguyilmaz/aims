---
title: Troubleshooting
weight: 13
aliases: ["/docs/troubleshooting/"]
---

## Start with doctor

```bash
aims doctor
```

```text
aims
  ✓ aims v0.1.3 ~/.local/bin/aims
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
  ✓ login keepalive ~/Library/LaunchAgents/com.doguyilmaz.aims.keepalive.plist
```

It checks the tools, every login and shared folder, and the integrations, repairs shared links as it goes, and prints the command for anything left. It exits with 1 when something needs you, so it also works in scripts.

## Common problems

### Both profiles log in as the same account

The browser signed in with whatever account it was already logged into. aims warns when it sees this (`me@gmail.com is already used by personal`), and `aims status` lists it under **Next**. Log the second profile in again from a private window, or let Claude Code pre-fill the address:

```bash
aims login claude work -- --email me@company.com
aims login codex work -- --device-auth      # a code you enter in any browser
```

### "account details are out of date"

Claude Code keeps the account's details next to the login and does not always rewrite them when the login changes. When they name another kind of plan than the login, aims stops trusting them, so the profile is not taken for another account and does not share its limits. Logging in again rewrites them: `aims login claude work`.

### "ignoring ANTHROPIC_API_KEY from your shell"

An API key in your shell would override every profile's login, so aims removes it for the tool it starts. To use an API key on purpose, give it its own profile with the key in [`env`](../configuration#profiles).

### Plain `claude` ignores the active account

The [shell integration](../everyday#plain-claude-and-codex) is missing or not loaded yet. Run `aims setup --shell` and open a new terminal. `type claude` should say it is a function.

### A profile shows "limited" but works again

The limit reset early, or the mark was a mistake. `aims clear claude work` removes it, and `aims status --live` checks with the provider.

### A login lapsed after weeks of not using an account

The tool could not renew it. Run `aims login <tool> <profile>`, and `aims keepalive on` so it does not happen again; see [Keeping logins alive](../failover#keeping-logins-alive).

### "shared 'settings.json': points to …, not the shared folder"

The entry in the profile is a link aims did not make, for example from a dotfiles tool. aims leaves it alone. Remove it if it should be shared, or list it under [`exclude`](../sharing#keeping-one-entry-private) to keep it.

### "the database is open; close the tool and run `aims doctor --fix`"

Codex keeps some state in SQLite databases. aims moves a database into the shared folder only when it is closed. Quit Codex and run `aims doctor --fix`.

### "This conversation is open in another app"

Another Codex process owns the conversation's writer lock. Close that conversation in its other terminal or app, then press `r` in the waiting terminal. A background daemon can keep the conversation loaded after the original terminal closes.

Supervised aims sessions run with `--no-daemon`, but a session started another way (`aims codex@work`, plain `codex`, an editor) can start a daemon that keeps running after it closes; `aims doctor --fix` cannot set a profile's private writer locks aside while one holds them. If an old daemon owns the conversation, stop that specific profile's daemon after finishing any work it hosts. For a profile called `personal` in the default aims directory on macOS or Linux:

```bash
CODEX_HOME="$HOME/.aims/profiles/codex/personal" command codex app-server daemon stop
```

Use the profile's actual folder shown by aims; an account using the original login may use `~/.codex` instead. Stopping a daemon disconnects all conversations it hosts. Do not delete lock files to bypass an active writer.

### Private writer locks

New shared Codex profiles link `thread-writer-locks/` to the hub. Older profiles may already have their own directory, and aims warns about it on launch. A lock file is open for as long as Codex has the conversation open, so on the next launch (or `aims sync`, `aims doctor`) aims checks that no process has a file in the profile's directory open, sets the directory aside (as `thread-writer-locks.aims-<profile>-<time>`) and links the shared one. While any Codex still uses it, it stays, and `aims` lists the profile under Next: close every Codex using that profile, including the app and background daemons (`codex app-server daemon stop` with that profile's `CODEX_HOME`). Do not delete lock files to bypass an active writer.

### "the tool uses the shared copy; delete this one to free its space"

aims starts every Codex profile that shares history with `CODEX_SQLITE_HOME` set to `~/.codex`, so all of them read and write the same databases, including ones a new Codex version adds. An index or log database still in the profile folder (`thread_history_*.sqlite`, `logs_*.sqlite`), created by Codex before aims did this, is no longer read: the index is rebuilt from the shared rollout files, so the conversations themselves are not in it. Once no Codex started before the update is running, you can delete the file and its `-wal` and `-shm` sidecars. aims never deletes it itself.

### "history sharing is incomplete"

Both the profile and the hub have a database that holds data, such as `goals_1.sqlite`. aims keeps both, and the profile keeps using its own (aims does not set `CODEX_SQLITE_HOME` for it), so nothing it wrote is hidden. `aims doctor --fix` cannot merge the two, and automatic moves skip the profile until its history is shared again. Close the affected tools and daemons and back up both databases with their SQLite sidecars before reconciling them; once only the hub's is left, the profile reads the shared databases.

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
