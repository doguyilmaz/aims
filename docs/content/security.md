---
title: Security and privacy
weight: 12
aliases: ["/docs/security/"]
---

aims handles the logins of your personal and work accounts, so it is built to touch as little as possible. What aims is for, and what it is not, is under [Fair use](../#fair-use).


## Logins

- **Never copied.** Each account logs in once, with the tool's own login, into its own folder. Claude Code and Codex rotate refresh tokens, so a copied login would also log the original out; aims refuses to do it and the skill tells the model not to.
- **Read locally when needed.** Account inspection reads local login files and, for Claude Code on macOS, its keychain entry to identify login state and plan. An explicit live Codex check reads its access token to call the account API; Claude checks run through its native CLI. Credential contents are not printed by bridge inspection or the default doctor check, and login files are never copied between profiles.
- **Removed through the tool.** `aims rm --purge` requires the tool's own logout before deleting the folder. If logout fails or the executable is unavailable, registration and remaining files stay for retry, including when the folder is already missing but a keychain/keyring entry may remain.
- **Kept apart.** Variables that would override a profile's login (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `CODEX_API_KEY`) are removed from the tool's environment, and aims tells you when it does, unless the profile sets them itself.

## Network

aims itself makes these requests:

| Request | When | Sends |
| --- | --- | --- |
| `github.com/doguyilmaz/aims/releases/latest` | at most once a day, on a terminal; and `aims upgrade` | nothing beyond a plain HTTPS request. Off with `AIMS_NO_UPDATE_CHECK=1`. |
| The tools' own providers | `aims usage --live` or `aims status --live`, or `r` in the dashboard | through the tools themselves: Codex's account API, and a one-word Claude Code prompt |
| The tools' own providers | `aims keepalive`, hourly in the background after `aims keepalive on` | the same, and only for a login whose access token has run out |

Everything else, including your prompts and conversations, goes from the tools to their providers as it would without aims. aims has no server and collects nothing.

## Files

- `~/.aims`, its `config.json` and `state.json`, and the profile folders are created readable by you only. If you put an API key in a profile's `env`, it is stored in `config.json` in plain text, as the tools store their own.
- Writes use a private temporary file and a rename. Existing file access controls are retained, including Unix ACLs/group ownership and Windows DACLs. A file you keep as a symlink (dotfiles) is written through, not replaced.
- Directory adoption preserves Unix ACLs, ownership, group/setgid and Windows DACLs. Copies are populated privately before their final access controls are restored; if aims cannot preserve those controls, it reports a conflict. T3 exports and recovery files are created privately even in a directory with inherited read grants.
- aims deletes only what it created: `--purge` refuses folders outside `~/.aims/profiles`, and shared content is moved to the hub before a profile folder goes. When it repairs a link it never overwrites: the other version is kept under a new name.
- A custom profile folder (`--dir`) must be new, empty or a config folder of that tool, and may not overlap your home folder, `~/.aims` or the tool's folder, also through a symlink or a different letter case.

## Sessions and the model

The MCP server lets the model see your account names, the e-mail addresses of your logins and their usage, switch the account for new sessions, and run a prompt with another account. It cannot log in, read logins or change the account of the running session. `aims_run` is read-only unless the call asks for `write`, so a restricted session cannot use it to edit files; see [AI integration](../ai-integration#mcp-server).

Codex reviews new or changed hooks before they run. aims skips that review only on the restarts it makes itself, and only when you turn on `failover.trustHooks` (`aims setup --trust-hooks`); see [Failover](../failover#codex-hooks).

On those restarts aims also answers Codex's "Trust this folder?" for that start only, unless the folder has Codex settings of its own; see [Failover](../failover#codex-folder-trust).

If one account must never see another's conversations, give it `share: settings` or `none`. See [What accounts share](../sharing).

## Reporting a problem

Please report security issues privately through [GitHub security advisories](https://github.com/doguyilmaz/aims/security/advisories/new) rather than in a public issue.

Integration changes preserve MCP settings in private, content-addressed files under `~/.aims/recovery/mcp`. Skill updates retain previous text beside the installed skill directory. These recovery copies survive clean; MCP settings can contain inline secrets, so they should remain private. The mechanism never copies `auth.json`, `.credentials.json` or keychain credentials. A failed clean retains aims configuration and account marks for retry.
