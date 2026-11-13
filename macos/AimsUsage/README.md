# Aims Usage 0.2.0

A native macOS menu bar app with an Edge Dock and an overview you can pin beside it. It uses the screen's visible frame, so it stays clear of a Dock on the bottom or either side. It runs as an accessory app with one icon and no Dock tile.

The app reads `aims usage --json` and refreshes with `aims usage --refresh`. Its bundled aims executable uses the existing aims configuration and provider logins. The app does not read or copy credential files, change accounts, reconcile sharing, start a model turn, redeem resets or buy credits. The Kiro adapter in aims reads Kiro's own cached login for its native usage API; the other adapters use their provider CLI. Native CLIs retain responsibility for their own authentication and renewal.

## Build and open

Requires macOS 14.0.0+, Swift 6.2.0+ and the Go version declared in the repository's `go.mod`.

```sh
scripts/build-macos-app.sh
open 'dist/Aims Usage.app'
```

The script builds for the Mac it runs on, bundles the matching aims CLI, uses the existing aims logo as one app icon, and applies an ad hoc signature. It does not install a login item or replace an installed app. Copy the resulting app to `~/Applications` if desired. Distribution signing/notarization is a separate release step; the local build is a development artifact.

Set `AIMS_USAGE_CLI_BINARY` to a prebuilt, executable aims CLI if the matching Go toolchain is unavailable locally. It must target this Mac and support the usage JSON schema. `AIMS_USAGE_VERSION` defaults to `0.2.0`; `AIMS_USAGE_BUILD_NUMBER` is a separate positive integer (default `1`); `AIMS_CLI_VERSION` defaults to `0.4.7-dev`. The builder does not download a Go toolchain automatically. Temporary compilation products are removed on exit.

```sh
swift test --package-path macos/AimsUsage
AIMS_USAGE_PREVIEW=1 scripts/build-macos-app.sh
open 'dist/Aims Usage Preview.app'
```

Preview uses bundled test data, a separate preferences domain and no provider calls. It opens the side panel automatically. `AIMS_USAGE_FIXTURE=/absolute/path/snapshot.json` can also supply a local fixture for development. Preview data is visibly labeled.

The development preview's More menu opens native menu-content and small-screen overflow test windows, and can switch the preview app between light and dark appearance. Those controls and windows are compiled only for preview builds. They do not change the Mac's system appearance.

## Daily use

- **Edge Dock** is on by default. Its 64-point glass rail sits flush with the usable right edge, with curved attachment shoulders rather than a detached capsule. Provider marks show the highest current usage for each provider; unavailable or stale-only percentages stay unknown. Hover a provider to peek at every account/model group; click to pin and interact with that detail. Use its pin to unpin, or ×/Escape to dismiss. The grid button opens the full overview. Hide/show the dock in Settings or More.
- Choose **Compact** (the default) or **Gauges** in the single-row toolbar, or in Settings. The saved choice applies to the menu and right-edge panel. Compact uses a narrower 400-point surface with aligned provider rows; Gauges retains 560 points for account/model cards, with a ring for **every reported limit window**. The host resizes when switching modes. Two columns provide room when the screen permits; three columns fit shorter screens without reducing type size. Both layouts measure their content: they grow to fit it, and scroll only when the available screen height is genuinely exceeded. Both views show percentages **used** and reset countdowns. The app follows the Mac's light/dark appearance.
- No window is hidden behind a selected allowance. The account's info or banked-reset button opens its full reading, plan, observation time, error and reported banked-reset inventory, with counts and expiries kept separate for each account. Passed resets and missing values never become locally refilled or zero-percent quotas.

- Click the gauge in the menu bar. The optional percentage is the highest **current, successfully read** usage among visible providers; stale windows and past reset times do not contribute. It reevaluates each minute, even with automatic provider refresh disabled, and hides after a failed snapshot read. This only updates the display and does not contact a provider.
- The Edge Dock opens on first launch; its overview button opens the panel. Click the menu's pin to show it on the screen containing the pointer, or the panel's × to close it. The borderless panel has no title-bar spacer and reserves room beside the Edge Dock. It stays visible across Spaces and remembers whether it was open, including when you quit and relaunch. Closing the panel explicitly keeps it closed on the next launch and leaves the menu bar app running. Quit stops its in-flight refresh before exiting.
- Refresh asks Codex and installed meters at most once per refresh, without a model prompt. Automatic refresh runs every five minutes while enabled, including when the menu is closed. It is independent of aims keepalive and updates observation caches without clearing manual routing/login marks.
- Claude uses its last native status-line report. `aims usage claude --live` is the existing optional paid one-word probe; the app does not run it automatically.
- Reset times use the Mac's local time zone. Hover for the full date. Past times display “refresh needed”; the app never resets percentages locally.
- Banked reset inventories are separate from normal quota windows. A known zero and unavailable information have different displays. Counts can be known while expiry details are not reported; detail lists can be shorter than the available count. The app only reads them.
- More → Add custom reset reminder creates a personal reminder. It does not assert that a provider has reset its quota. Reminders are shared with the CLI through `AIMS_HOME/usage-reminders.json`.
- Settings controls automatic refresh, the menu percentage and an optional explicit aims executable. The bundled CLI is the default. For a custom `AIMS_HOME`, select that existing data folder in Settings; Finder does not load shell exports. An `AIMS_HOME` inherited on the first launch is remembered. The selected path stays in Settings, and changing it never moves files. Apply is disabled during a refresh; applying then clears the display and reads the newly selected cache.
- Finder launches search the usual provider, Go, Cargo, npm, Bun and Homebrew directories. Add other absolute CLI folders in Settings (one per line). Custom folders take precedence; no shell startup files are executed.

Provider order is Claude (each configured account), Codex (each account), Kiro, Grok, Antigravity (each reported model group), then OpenCode. All Antigravity group IDs and bucket IDs are retained; Gemini, Claude/GPT, Argon and Carbon are displayed separately **when reported by that account's CLI**. An unreported model group is not fabricated. OpenCode shows Go windows, zero-dollar blocking budgets, and separate per-model budgets when returned.

Errors retain the previous reading with its age; they do not turn old percentages into fresh data. Missing quotas and reset inventories stay unknown. A capped reset-detail list is labeled incomplete, and explicit non-expiring credits differ from unknown expiry. The app stores preferences and selected paths only. Removing it leaves aims, its caches, custom reminders, credentials, sessions, history and other integrations intact.

See the [provider support and sources](../../docs/content/macos-usage.md) for API limitations.

The app version and source revision appear in More and Settings so a running older build can be identified. `AIMS_USAGE_SOURCE_REVISION` can override the development revision label; the builder records the Git revision and a dirty marker for tracked changes or untracked files by default. Ignored build outputs do not mark it dirty; an unreadable Git status is labeled unknown. These are build identifiers, separate from the SemVer app version and integer build number.

Provider marks are the pinned Lobe Icons 1.102.0 SVG assets (MIT); the source metadata and license are bundled from [packaging/provider-marks](../../packaging/provider-marks). The app still ships one application icon. No icon package is downloaded at runtime.
