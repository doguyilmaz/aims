---
title: Troubleshooting
weight: 13
aliases: ["/docs/troubleshooting/"]
---

## Start with doctor

```bash
aims doctor
```

```bash
aims doctor --check --json   # read-only, scriptable report
aims doctor --fix            # conservative repairs; recovery copies stay
aims doctor --logins         # optional login/version inspection
```

The default check inspects paths and integration settings without invoking a provider or accessing credentials. `--fix` creates missing links and applies explicit sharing-policy changes, then checks again. Independent data and externally managed links are kept. Exit `1` means a problem remains; read its message before retrying. `--logins` may invoke local CLIs, read the keychain and update cached account details.

## Common problems

### Profile labels point to the same login folder

New profile names must differ beyond letter case, so moving between case-sensitive and case-insensitive filesystems cannot merge two login homes. Doctor also reports existing labels that resolve to the same folder. aims leaves their credentials and files in place and refuses to launch, repair or purge the ambiguous home. Remove only the duplicate label with `aims rm` without `--purge`; do not delete its folder or log it out to resolve the duplicate.

### Conversations or accounts are missing in T3 Code or another app

Run `aims bridge` inside the affected environment. It shows the actual CLI homes and shared transcript paths without modifying them. An external `shadow` home may share conversations while using a separate managed login, and T3's own thread list needs its session/project import flow even when the files are shared. `aims bridge t3` previews connections to aims' existing CLI accounts; `aims exec -- <program>` passes those accounts to apps that inherit the environment. See [Other coding apps](../other-apps) before changing paths or logins. Never copy credentials or delete session databases to make the lists agree.

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

The limit reset early, or the mark was a mistake. `aims clear claude work` removes it, and `aims usage --live` checks with the provider.

### A login lapsed after weeks of not using an account

The tool could not renew it. Run `aims login <tool> <profile>`, and `aims keepalive on` so it does not happen again; see [Keeping logins alive](../failover#keeping-logins-alive).

### "shared 'settings.json': points to …, not the shared folder"

The entry in the profile is a link aims did not make, for example from a dotfiles tool. aims leaves it alone. Remove it if it should be shared, or list it under [`exclude`](../sharing#keeping-one-entry-private) to keep it.

### A database may be open or need recovery

Codex keeps some state in SQLite databases. aims adopts an independent database only when no process has its folder open and no SQLite sidecar remains in either location. Close affected apps and daemons, retain the database with all its `-wal`, `-shm` and `-journal` files, and checkpoint/recover it with SQLite before running `aims sync`. Never delete a sidecar to silence the warning. `aims doctor --fix` cannot force adoption or merge two databases.

### "This conversation is open in another app"

Another Codex process owns the conversation's writer lock. Close that conversation in its other terminal or app, then press `r` in the waiting terminal. A background daemon can keep the conversation loaded after the original terminal closes.

Supervised aims sessions run with `--no-daemon`, but a session started another way (`aims codex@work`, plain `codex`, an editor) can start a daemon that keeps running after it closes; `aims doctor --fix` cannot set a profile's private writer locks aside while one holds them. If an old daemon owns the conversation, stop that specific profile's daemon after finishing any work it hosts. For a profile called `personal` in the default aims directory on macOS or Linux:

```bash
CODEX_HOME="$HOME/.aims/profiles/codex/personal" command codex app-server daemon stop
```

Use the profile's actual folder shown by aims; an account using the original login may use `~/.codex` instead. Stopping a daemon disconnects all conversations it hosts. Do not delete lock files to bypass an active writer.

### Private writer locks

New shared Codex profiles link `thread-writer-locks/` to the hub. Older profiles may already have their own directory; `aims` lists such a profile under Next. A lock file is open for as long as Codex has the conversation open, so on the next launch (or `aims sync`, `aims doctor --fix`) aims checks that no process has a file in the profile's directory open, sets the directory aside (as `thread-writer-locks.aims-<profile>-<time>`) and links the shared one. While any Codex still uses it, it stays, and `aims` lists the profile under Next: close every Codex using that profile, including the app and background daemons (`codex app-server daemon stop` with that profile's `CODEX_HOME`). Do not delete lock files to bypass an active writer.

### Unused index or log database copies

For a Codex profile sharing history, aims sets `CODEX_SQLITE_HOME` to the hub unless an explicit override or private data database prevents it. Older index/log copies in the profile may be unused. `aims doctor --fix` archives a verified-unused copy and all its sidecars in `.aims-recovery-*` inside the profile. It never deletes that recovery data. Keep the archive until you have checked the conversations in the tools you use; it is not included in automatic cleanup. If aims cannot establish that the folder is unused, the copy stays where it is. On Windows, aims probes files and directories with native sharing restrictions. An open or inaccessible entry prevents reconciliation; closing the affected tools allows the next sync or repair to try again.

### An independent file remains after doctor fix

This is a deliberate conflict, not a failed upgrade. Doctor does not choose between independent settings, history folders or databases. Close affected tools, review and back up both copies, then use `aims sync` for files/folders. For two data databases, use a deliberate SQLite reconciliation. Running `--fix` repeatedly will not discard one.

### A skill is managed through a symlink

Update it through its owner (for example a dotfiles tool or lockstep). aims setup and refresh leave that path alone. When setup updates a regular installed aims skill, its previous text is kept in a private `.aims-skill-*.bak` file beside the `aims` skill directory. `aims clean` keeps recovery files and user additions. Locally edited or externally managed skills are not deleted during uninstall.

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
