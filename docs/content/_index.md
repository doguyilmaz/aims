---
title: aims
linkTitle: Start here
description: Manage Claude Code and Codex accounts. Track AI usage, resets and budgets in the terminal, macOS menu bar and Edge Dock.
type: docs
cascade:
  type: docs
aliases: ["/docs/"]
---

Keep personal and work accounts separate. Continue Claude Code and Codex sessions across accounts, and track usage in the terminal or on your Mac.

## Start in the terminal

```sh
brew install --cask doguyilmaz/tap/aims
aims init
claude                     # or codex
```

On Linux or Windows, follow [Installation](installation). Setup connects your logins and offers shell integration. Each account keeps its own credentials; history sharing is configurable.

Run `aims` for the dashboard, `aims usage` for limits and resets, or `aims doctor --check` to inspect your setup.

## Keep usage in view

Aims Usage is a native macOS menu-bar app with an optional Edge Dock. Choose your layout, display, accounts and allowance windows.

{{< usage-preview >}}

*Illustrative preview data. The app is a source preview; public signed downloads and automatic updates are not active yet.*

[Set up the macOS app →](macos-usage)

## Find what you need

{{< cards >}}
  {{< card link="quick-start" title="First session" icon="play" >}}
  {{< card link="macos-usage" title="Menu bar & Edge Dock" icon="desktop-computer" >}}
  {{< card link="sharing" title="Share or isolate history" icon="folder" >}}
  {{< card link="other-apps" title="Editors & SDKs" icon="terminal" >}}
  {{< card link="troubleshooting" title="Fix a problem" icon="cog" >}}
  {{< card link="commands" title="Command reference" icon="book-open" >}}
{{< /cards >}}
