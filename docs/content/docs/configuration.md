---
title: Configuration
weight: 10
---

aims keeps everything in `~/.aims` (or `$AIMS_HOME`):

| Path | Holds | Edit by hand? |
| --- | --- | --- |
| `config.json` | your profiles and preferences | yes |
| `state.json` | what aims learned: marks, usage, accounts, last use | no, it is a cache |
| `profiles/<tool>/<name>/` | one config folder per account | no |

Both JSON files are written atomically with owner-only permissions, under a lock, so two aims processes never interleave a change.

## config.json

```json
{
  "version": 1,
  "failover": {
    "auto": true,
    "cooldownMinutes": 300,
    "threshold": 95
  },
  "statusline": {
    "chain": "~/bin/my-status-line"
  },
  "tools": {
    "claude": {
      "hub": "/Users/me/.claude",
      "active": "personal",
      "order": ["personal", "work"],
      "profiles": {
        "personal": { "existing": true },
        "work": {},
        "client-x": {
          "share": "settings",
          "dir": "~/clients/x/claude",
          "exclude": ["CLAUDE.md"]
        },
        "api": {
          "share": "none",
          "env": { "ANTHROPIC_API_KEY": "sk-ant-..." }
        }
      }
    }
  }
}
```

### Profiles

| Key | Meaning |
| --- | --- |
| `existing` | This profile is the tool's own folder and the login already in it. It runs with `CLAUDE_CONFIG_DIR` / `CODEX_HOME` exactly as you had it, usually unset, so the tool behaves as it does without aims. At most one per tool. |
| `share` | `all` (default), `settings` or `none`. See [What accounts share](../sharing). |
| `dir` | The profile's folder, if not `~/.aims/profiles/<tool>/<name>` |
| `exclude` | Shared entries this profile keeps private |
| `env` | Extra environment for the tool, such as an API key for an account that does not log in through the browser. Variables like `ANTHROPIC_API_KEY` that would override a login are otherwise removed from the tool's environment. |

### Tools

| Key | Meaning |
| --- | --- |
| `hub` | The tool's normal folder, recorded the first time aims needs it. Shared entries link here. |
| `hubEnv` | `CLAUDE_CONFIG_DIR` or `CODEX_HOME` as you had it set, if you did |
| `active` | The profile new sessions get. aims keeps it first in `order`. |
| `order` | Which profiles stand in, in turn, while the active one is limited or logged out |

`failover` is described in [Limits and failover](../failover#settings). `statusline.chain` is the status line command you had before `aims setup`, which aims runs first.

A config file that does not parse is an error: aims stops and tells you, and never replaces it with an empty one.

## Environment variables

| Variable | Meaning |
| --- | --- |
| `AIMS_HOME` | Use this folder instead of `~/.aims` |
| `AIMS_CLAUDE_BIN`, `AIMS_CODEX_BIN` | The tool binary to run, if not the one on `PATH` |
| `AIMS_CLAUDE_PROFILE`, `AIMS_CODEX_PROFILE` | Pin this terminal to a profile (what `aims env` sets) |
| `AIMS_SESSION_PROFILE` | Set by aims for the tool it starts: `claude:work`. Read by the status line and the MCP server. |
| `AIMS_NO_UPDATE_CHECK` | Set to anything to turn off the daily release check |
| `NO_COLOR` | Plain output without colour |
| `ACCESSIBLE` | Set to anything for setup questions as plain numbered prompts, for screen readers |
