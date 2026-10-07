---
title: Security and privacy
weight: 12
---

aims handles the logins of your personal and work accounts, so it is built to touch as little as possible.

## Logins

- **Never copied.** Each account logs in once, with the tool's own login, into its own folder. Claude Code and Codex rotate refresh tokens, so a copied login would also log the original out; aims refuses to do it and the skill tells the model not to.
- **Never read for their secrets.** aims checks that a login exists and reads the e-mail and plan stored next to it, to show you which account is which. Token values are not used.
- **Removed cleanly.** `aims rm --purge` runs the tool's own logout before deleting the folder, so no login stays behind in the macOS keychain or an OS keyring.
- **Kept apart.** Variables that would override a profile's login (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `CODEX_API_KEY`) are removed from the tool's environment, and aims tells you when it does, unless the profile sets them itself.

## Network

aims itself makes two kinds of requests:

| Request | When | Sends |
| --- | --- | --- |
| `github.com/doguyilmaz/aims/releases/latest` | at most once a day, on a terminal; and `aims upgrade` | nothing beyond a plain HTTPS request. Off with `AIMS_NO_UPDATE_CHECK=1`. |
| The tools' own providers | `aims status --live`, or `r` in the dashboard | through the tools themselves: Codex's account API, and a one-word Claude Code prompt |

Everything else, including your prompts and conversations, goes from the tools to their providers as it would without aims. aims has no server and collects nothing.

## Files

- `~/.aims`, its `config.json` and `state.json`, and the profile folders are created readable by you only. If you put an API key in a profile's `env`, it is stored in `config.json` in plain text, as the tools store their own.
- Writes are atomic (a temporary file, then a rename), so an interrupted aims never leaves a half-written file. A file you keep as a symlink (dotfiles) is written through, not replaced.
- aims deletes only what it created: `--purge` refuses folders outside `~/.aims/profiles`, and shared content is moved to the hub before a profile folder goes.

## Sessions and the model

The MCP server lets the model see your account names, the e-mail addresses of your logins and their usage, switch the account for new sessions, and run a prompt with another account. It cannot log in, read logins or change the account of the running session. `aims_run` uses the tool's default permissions, so a headless run cannot use tools that would need your approval.

If one account must never see another's conversations, give it `share: settings` or `none`. See [What accounts share](../sharing).

## Reporting a problem

Please report security issues privately through [GitHub security advisories](https://github.com/doguyilmaz/aims/security/advisories/new) rather than in a public issue.
