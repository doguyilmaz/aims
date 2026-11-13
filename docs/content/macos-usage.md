---
title: macOS usage app
weight: 9
---

Aims Usage 0.2.0 is an optional native menu bar app with an Edge Dock and an overview you can pin beside it. The macOS Dock can remain on the bottom or either side; the usage rail attaches to your chosen usable screen edge. It reads the same usage snapshot as `aims usage --json`; personal/work profiles, model groups, reset inventories and custom reminders remain separate.

Build from the repository with `scripts/build-macos-app.sh`, then open `dist/Aims Usage.app`. Requirements: macOS 14.0.0+, Swift 6.2.0+, and the Go version in `go.mod`. The app is a locally signed development build; public release signing/notarization is separate. Its menu pin toggles the side panel. More contains Settings, custom reminders and Quit. No login item is installed automatically.

### Provider data and resets

Choose **Compact** (the default) or **Gauges** in Settings → Display → Usage appearance. Compact uses 400-point provider rows with usage bars. The Gauges menu uses a 440-point list of horizontal account/model rows with small rings and adjacent reset countdowns. Extra windows wrap within their own account; no reported window is dropped. The pinned Gauges overview retains its separate 560-point card grid. The saved choice applies to menu and overview, and is independent of Edge Dock appearance. Both measure their content, growing to fit and scrolling only when the screen has insufficient room. System light/dark appearance, native provider marks and reduced-motion/transparency preferences are respected. Account info and banked-reset buttons open the full reading and expiry details; unknown values stay unknown.

The **Edge Dock** can attach to the usable **left or right** edge. Settings → Edge Dock offers **Standard** (64 points) or **Mini** (44 points), plus **Automatically hide**. Use **Vertical position** to move it between the usable top and bottom; **Center** restores the middle. The saved position adapts to the available screen height and Dock size, keeping the rail and hidden marker within the usable edge. The shape and detail-card direction follow the selected side. Mini keeps the same providers and values with shorter rows; a blocked quota uses `!` and exposes the full state in its tooltip/details. These preferences are saved independently of Compact/Gauges.

The menu popover stays anchored to its menu-bar icon. The pinned overview stays at the upper right of its screen; changing the Edge Dock's side, size, vertical position or visibility does not move it.

Automatic hiding is enabled by default. Hover the thin marker in the selected edge region to reveal the rail; it hides after the pointer leaves the Dock and its detail card. Crossing into details and pinned cards keep it visible. Uncheck the option for an always-visible Dock. Disabling Edge Dock removes both rail and marker. Short fades respect reduced motion and keyboard actions. Mouse-movement monitors are removed when no longer needed; no keyboard events are monitored or suppressed.

The rail stays clear of the macOS Dock on any side. Hover a provider for all accounts/models, click to pin 400-point details, or open the complete overview. It shows the highest current known usage per provider; a missing/stale-only ratio is an em dash. Enable or hide it in Settings or More. The app version and Git build revision are visible in More and Settings.

The optional menu-bar percentage also uses current successful readings. It is reevaluated every minute, including with automatic refresh disabled, and hidden after a failed snapshot read. A reset passing removes that window from the summary; it does not invent a new allowance or contact the provider.

| Provider | Data source | Scheduled reset | Banked / other resets |
| --- | --- | --- | --- |
| Claude, each configured account | Cached native status line; optional explicit CLI live probe | Reported window timestamps | Not exposed by this status-line reader; shown as unavailable |
| Codex, each configured account | Native app-server `account/rateLimits/read` | Main and additional model/feature windows | `rateLimitResetCredits` count; available details/expiries retained when supplied |
| Kiro | Native cached login and `GetUsageLimits` | Per-resource or root `nextDateReset`, seconds or milliseconds | No invented banked reset inventory; add-on credits are not resets |
| Grok | Native ACP `_x.ai/billing` | `currentPeriod.end` | Inventory unavailable. Unified billing may omit the usage percentage even while reporting the period; the percentage then stays unknown |
| Antigravity | `agy -p /usage --output-format json` | Each reported group's bucket timestamps | Unreported groups or reset inventories stay unavailable |
| OpenCode | Native `/api/experimental/usage/limits` | Each Go meter and budget timestamp | Budget amounts and per-model limits remain distinct; no inferred banked resets |
| Custom | User-defined reminder | Explicit date with timezone | A reminder only; never changes the provider or recorded usage |

Antigravity retains every reported model-group ID, including separate Gemini, Claude/GPT, Argon 4 and Carbon 4 groups when supplied. Groups keep their own usage and reset timestamps even when their bucket IDs match. An account that does not report a group is not given an invented allowance for it.

`aims usage --refresh` and the app's automatic five-minute refresh do not send a model prompt. Claude therefore remains cached until its native status line or an explicit `--live` updates it. The refresh timestamp of one provider never makes another provider's numbers fresh. A failure retains the original observation time, and a reset timestamp passing does not prove that the provider refreshed the allowance.

The passive `--refresh` path updates observation caches without clearing manual routing/login marks. Successful user-controlled reauthentication invalidates old cached quotas; an explicit Claude live probe checks the native offline account identity before attributing cached windows to a changed login. Credentials and history are not copied or migrated by this bookkeeping.

Finder does not load shell exports. For a custom `AIMS_HOME`, select the existing folder in the app's Settings; the selected data location stays in Settings. A custom home inherited on the first launch is remembered. Additional CLI folders can be entered one per line, including Go/Cargo/npm or version-manager paths. No shell startup script is run, and changing the data location never moves files.

The installed Codex CLI 0.167.1 protocol was inspected with `app-server generate-ts --experimental`. It documents that the reset-credit detail list may be absent or capped, and that `availableCount` is independent of the list length. Other Codex versions may not report this field. Additional quota buckets are for display and do not alter the existing main-account failover decision.

OpenAI distinguishes [banked, automatic promotional and purchased resets](https://help.openai.com/en/articles/20001498-how-resets-work-in-codex) from scheduled usage refreshes. Promotional resets have no guaranteed schedule; aims does not predict a future grant or redeem an available one. Google's [native quota command](https://antigravity.google/docs/cli/commands/usage) reports model groups that share limits. Kiro's [billing documentation](https://kiro.dev/docs/billing/) describes renewal at the next billing cycle; that URL uses the canonical trailing slash because the slashless URL redirects. OpenCode's [budget documentation](https://opencode.ai/docs/console/budgets) distinguishes overlapping budget policies, and its [API schema](https://opencode.ai/docs/api) reports their individual reset timestamps. The app uses the returned timestamp rather than constructing a billing date.

### JSON and custom reminders

The snapshot has `schemaVersion: 1`, `generatedAt`, ordered `providers`, and `customResets`. Rows retain their own `observedAt`, windows, optional banked-reset balance, and errors. `bankedResets: null` means unavailable; `{ "available": 0, "credits": null }` means a reported empty inventory without details. A detail list shorter than the reported inventory is labeled incomplete. `noExpiry: true` means the provider explicitly reported a non-expiring reset; an absent expiry alone stays unknown. `pctUnknown: true` means a numeric usage ratio is unavailable, including a zero-dollar budget. This is different from zero percent usage.

```sh
aims usage --refresh --json
aims usage agy --json
aims usage reset add codex personal --at 2026-12-01T09:00:00+03:00 --label 'Check reset'
aims usage reset remove REMINDER_ID
```

Custom reminders use a separate private file, `AIMS_HOME/usage-reminders.json`. Damaged or newer reminder formats are retained and reported; they do not suppress other usage data. To repair a malformed file, preserve a copy and correct its JSON. Removing the macOS app does not remove reminders, aims configuration, logins, sessions or history.
