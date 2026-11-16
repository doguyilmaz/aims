---
title: Aims Usage for macOS
weight: 9
---

Aims Usage is one native macOS app with three surfaces: a menu-bar popover, an optional Edge Dock and an overview you can pin open. The macOS Dock can remain on the bottom or either side; the usage rail attaches to your chosen usable screen edge. It reads the same usage snapshot as `aims usage --json`; personal/work profiles, model groups, reset inventories and custom reminders remain separate.

## Availability and installation

**Source preview.** Public signed app downloads, the separate `aims-usage` Homebrew cask and the production Sparkle channel have not been activated. The existing `doguyilmaz/tap/aims` cask installs the CLI and its keepalive wrapper, `aims.app`.

Build from a checkout containing the macOS app. Requirements are macOS 14.0.0+, Swift 6.2.0+ and the Go version declared in `go.mod` on `PATH`; the builder does not download a Go toolchain.

```sh
scripts/build-macos-app.sh
'dist/Aims Usage.app/Contents/Resources/aims' app install --from 'dist/Aims Usage.app'
"$HOME/Applications/Aims Usage.app/Contents/Resources/aims" app open
```

The installer verifies the local signature and copies the app to `~/Applications`. It refuses to replace an existing app, including symlinks; use that app’s updater or explicitly move it before selecting another build. The app includes an aims executable; a standalone CLI installation is managed separately. The local development build is ad hoc signed and has no active updater channel. Public signing/notarization belongs to the release process.

Set up/manage Claude and Codex accounts with the [CLI](../installation), and authenticate the other providers with their native CLIs. The app reads those existing profiles and logins. At first launch, use Settings to select an existing custom `AIMS_HOME` if needed; changing the folder does not move account data. More contains Settings, custom reminders and Quit. No login item is installed automatically.

## CLI, dashboard and setup

On macOS, `aims app` opens a small install/open menu; `aims app status --json` reports the app and terminal command without changing profiles. The accounts and usage dashboard both offer **m · macOS app**. `aims init` offers optional app installation; `aims init --app` requests it explicitly, and `aims setup --app` installs it separately from tool integrations. App commands/options and the dashboard shortcut are absent on Linux and Windows.

`aims app install` uses the separate public Homebrew cask when available. The prepared `aims-usage` cask depends on the `aims` CLI cask. If a CLI is already on PATH, the command skips that cask dependency and keeps the existing installer's ownership. Until the public cask exists, use the source-preview `--from` command above. Installation preserves provider homes, configuration and sessions. `aims app open` opens the detected app.

Every app includes its CLI. For a direct-download install, **More → Terminal command…** and **Settings → CLI & paths → Terminal command** can expose it at `~/.local/bin/aims`. Move the app to Applications first. Existing CLI commands, files and unrelated links are kept; no shell file is edited. Add `~/.local/bin` to your shell PATH if needed. `aims app link-cli` offers the same operation from the terminal. The local `--from` installer creates this link when no CLI is on PATH. App updates keep the bundle path stable; the standalone package-manager CLI continues to update separately.

Doctor reports an absent app as **info**, with an install hint. `doctor --fix` does not install the optional app. No companion app check appears on other systems.

## One app, three surfaces

| Surface | Use it for |
| --- | --- |
| Menu bar | Open a quick usage summary; choose Compact or Gauges in Settings |
| Edge Dock | Keep provider indicators at either edge, hover for detail, or pin the rail |
| Pinned overview | Leave the complete usage summary open while you work |

<table>
  <tr><th>Gauges overview</th><th>Edge Dock</th></tr>
  <tr>
    <td align="center"><img src="/images/aims-usage-overview.jpg" width="340" alt="Aims Usage Gauges overview with illustrative quotas and reset countdowns"></td>
    <td align="center"><img src="/images/aims-usage-edge-dock.jpg" width="64" alt="Aims Usage Edge Dock with six provider indicators"></td>
  </tr>
</table>

*Illustrative preview data. Model groups and reset inventories appear only when a provider reports them.*

![Aims Usage provider details with account choices and separate reset countdowns, using preview data](/images/aims-usage-hover.jpg)

## Everyday controls

Settings uses native grouped forms and system-sized controls. Its sidebar separates **General**, **Edge Dock → Layout & display**, **Edge Dock → Usage readings**, **CLI & paths** and **Updates**. Layout & display holds monitor previews, placement and visibility; Usage readings holds percentages and per-provider limit windows. The window can be resized; pages keep a stable size and position. The native toolbar and floating sidebar use the system’s adaptive corner geometry, without a separate title-bar divider. The app version and source revision are always visible in the sidebar footer. Terminal integration, executable and data-folder preferences are under **CLI & paths**.

On **macOS 26 and newer**, custom floating surfaces use native **Liquid Glass**: the Edge Dock, provider details, options and pinned overview. The menu popover and Settings split view retain their system-provided material. Glass is applied once per floating surface; quota rows stay readable without extra glass layers. With **Reduce Transparency**, custom panels use an opaque system background. Earlier macOS versions use standard material. The app follows the Mac's appearance and accessibility choices without changing them.

Choose **Compact** (the default) or **Gauges** in Settings → General → Usage appearance. Compact uses 400-point provider rows with usage bars. Gauges uses the same 340-point layout in the menu and pinned overview: one heading per provider, short account/model rows, small rings and adjacent reset countdowns. Extra windows wrap within their own account; no reported window is dropped. The saved choice applies to menu and overview, and is independent of Edge Dock appearance. Both measure their content, growing to fit and scrolling only when the screen has insufficient room. System light/dark appearance, native provider marks and reduced-motion/transparency preferences are respected. Account info and banked-reset buttons open the full reading and expiry details; unknown values stay unknown.

Application controls share monochrome SF Symbols with consistent hover and press feedback. Pinned controls use the Mac's accent color and a filled pin. The overview icon depicts stacked rows; banked resets use a reset arrow. The menu-bar symbol uses the inner aims terminal mark, also shown beside the popover's aims heading; More uses a plain ellipsis. Provider marks remain separate. Pointer actions leave no keyboard-focus frame behind; Tab navigation retains its visible focus indicator and Escape dismisses the floating panel.

The **Edge Dock** can attach to the usable **left or right** edge. Settings → Edge Dock → **Layout & display** offers **Standard** (64 points) or **Mini** (44 points), plus **Automatically hide**. Use **Vertical position** to move it between the usable top and bottom; **Center** restores the middle. The saved position adapts to the available screen height and Dock size, keeping the rail and hidden marker within the usable edge. The shape and detail-card direction follow the selected side. Mini keeps the same providers and values with shorter rows; a blocked quota uses `!` and exposes the full state in its tooltip/details. Its hover card also becomes smaller (300 points instead of Standard’s 400), with tighter spacing and rows while keeping every account, window, reset and action available. These preferences are saved independently of Compact/Gauges.

For multiple monitors, choose **Settings → Edge Dock → Layout & display → Display**. **Follow primary display** tracks the system's primary screen; click a monitor card to keep the Dock there independently of keyboard focus, the menu and the overview. The app remembers the screen's identity across launches and display rearrangements. If that screen is disconnected, the Dock temporarily uses the primary screen and returns when it reconnects. The rail, hidden marker, hover details and options use that screen's current global coordinates and usable bounds, including screens to the left, right, above or below the primary screen. Display scaling does not change the sizes in points. Monitor cards show each display's name and a Dock silhouette that follows side, size and vertical-position changes. Any number of connected screens can appear in the selector; the Dock is shown on one chosen screen. Mirroring uses the shared logical desktop. Screen labels distinguish **Primary** and **Secondary**. The selected screen has an accent-colored preview and checkmark; only its preview shows the Dock.

Monitor choices use compact 116-point thumbnails. Two, three or four screens share one row; more screens wrap into additional rows. Full display names remain available in tooltips, and the selected monitor keeps its name, checkmark and Dock silhouette.

For Claude and Codex with multiple accounts, the hover details offer a small **Dock: All / Personal / Work** row using your configured account names. Choose one account for that provider's Dock indicator, or **All** for the combined summary. The same choices appear in Dock options. Each provider remembers its own selection across relaunches. This is a display preference: it does not switch the active aims profile or provider login. Details and the overview keep every account visible, and the menu-bar percentage remains independent. A removed selected account stays **Missing** with an unknown Dock value until you choose another account or All; another account is never silently substituted.

Right-click a provider in the Dock to choose its **Usage window**: Automatic, 5 hours, Weekly, Monthly, Budget, or any other window that provider reports. Choices are saved separately for each provider and apply within the selected Dock account or accounts. Automatic uses the highest current known used percentage across their windows, falling back to cached readings when no current percentage is available; an explicit choice considers only that window. A saved window that disappears remains selected and is marked **Not reported**; another allowance is never silently substituted.

Right-click and Control-click open a separate 260-point options panel toward the inside of the screen. Its width is independent of the Standard/Mini rail, and its position stays within the screen's usable frame. Account chips keep the panel open for quick comparisons; choosing a window, percentage mode or action closes it. Escape, its close button or moving focus to another window also dismisses it. Opening with the pointer leaves buttons unfocused; Tab starts normal keyboard navigation with a visible focus indicator. The overview button's options include expandable provider groups with the same account and window choices. VoiceOver's **Dock options** action opens the same panel. Primary clicks and hover details retain their existing behavior.

The same menu offers **Used** or **Remaining** percentages for the entire Dock. Remaining shows `100 − used`, floored at zero: the most constrained account therefore has the lowest remaining percentage. Both the number and ring follow this preference; unknown or blocked readings keep their explicit state. Hover details and the menu-bar percentage continue to use their existing `% used` convention. These options are also available in Settings → Edge Dock → Usage readings. Right-click the overview button for all provider choices and **Hide Edge Dock**.

The Dock's footer pin button keeps the rail visible, independently of pinning a provider detail card. It preserves the Automatically hide setting: unpinning resumes that behavior. Pin state survives relaunches. The footer keeps its existing dimensions with overview and pin buttons; Hide remains available in the options panel and the app's More menu. An open options panel temporarily keeps the rail visible. The rail itself does not become key for pointer-only actions; the options panel accepts keyboard input without leaving an unrelated provider focus highlight behind. Keyboard navigation remains available.

The menu popover stays anchored to its menu-bar icon. The pinned overview stays at the upper right of its screen; changing the Edge Dock's side, size, vertical position or visibility does not move it.

Automatic hiding is enabled by default. Hover the thin marker in the selected edge region to reveal the rail; it hides after the pointer leaves the Dock and its detail card. Crossing into details and pinned cards keep it visible. Uncheck the option for an always-visible Dock. Disabling Edge Dock removes both rail and marker. Short fades respect reduced motion and keyboard actions. Mouse-movement monitors are removed when no longer needed; no keyboard events are monitored or suppressed.

The rail stays clear of the macOS Dock on any side. Hover a provider for all accounts/models, click to pin details sized for Standard or Mini, or open the complete overview. It shows the highest current known usage within each provider's selected Dock accounts and window. If only older readings remain in that selection, the last known percentage stays visible with a small clock badge, a muted ring and its observation time in the tooltip. Readings older than two hours are marked cached; they are not discarded merely because of age. Windows whose reset has passed are excluded from the summary. An unreported percentage (such as Grok billing without a usage ratio) remains an em dash with an explanation in its tooltip. Enable or hide the rail in Settings or More. The app version and Git build revision are visible in More and Settings.

Provider hover details have a **Refresh quotas** button and show the age of the oldest reported reading. Manual refresh updates the provider measurements without a model prompt; it also starts the next five-minute interval, avoiding a second automatic request immediately afterward. The app rereads local usage caches every minute, even when automatic provider refresh is off, so measurements written by another aims session or status hook appear without waiting for the next API poll. A successful cache read does not hide an outstanding provider-refresh failure.

The optional menu-bar percentage also uses current successful readings. It is reevaluated every minute, including with automatic refresh disabled, and hidden after a failed snapshot read. A reset passing removes that window from the summary; it does not invent a new allowance or contact the provider.

## Provider data and resets

| Provider | Data source | Scheduled reset | Banked / other resets |
| --- | --- | --- | --- |
| Claude, each configured account | Existing profile OAuth usage API on Refresh; native status-line cache also retained | Reported window timestamps | Not exposed by this reader; shown as unavailable |
| Codex, each configured account | Native app-server `account/rateLimits/read` | Main and additional model/feature windows | `rateLimitResetCredits` count; available details/expiries retained when supplied |
| Kiro | Native cached login and `GetUsageLimits` | Per-resource or root `nextDateReset`, seconds or milliseconds | No invented banked reset inventory; add-on credits are not resets |
| Grok | Native ACP `_x.ai/billing`, then a bounded Grok web usage check when necessary | Native `currentPeriod.end` remains authoritative | Reset inventory is not read by this integration. A partial billing response can be enriched from the same account's verified web usage |
| Antigravity | `agy -p /usage --output-format json` | Each reported group's bucket timestamps | Unreported groups or reset inventories stay unavailable |
| OpenCode | Native `/api/experimental/usage/limits` | Each Go meter and budget timestamp | Budget amounts and per-model limits remain distinct; no inferred banked resets |
| Custom | User-defined reminder | Explicit date with timezone | A reminder only; never changes the provider or recorded usage |

Antigravity retains every reported model-group ID, including separate Gemini, Claude/GPT, Argon 4 and Carbon 4 groups when supplied. Groups keep their own usage and reset timestamps even when their bucket IDs match. An account that does not report a group is not given an invented allowance for it.

Grok's unified billing can omit the percentage while still returning a weekly period. This partial JSON alone stays unknown. Aims then allows up to six seconds for the read-only web billing method used by grok.com, using one unambiguous, non-expired first-party login from Grok's existing `auth.json`. `GROK_HOME` and an absolute `GROK_AUTH_PATH` are respected. If the credential changes during the native/web read, enrichment is discarded. Aims never refreshes, copies, rewrites or deletes Grok credentials and never reads browser cookies.

The web response must be complete and successful, and its weekly period must match the native period and contain the current time. A published percentage is preferred. A fully validated active-period response with an omitted proto3 scalar can also supply zero, matching [the Grok web client's behavior documented by CodexBar](https://github.com/steipete/CodexBar/blob/main/docs/grok.md). Malformed, expired, conflicting or failed responses cannot supply a percentage. A web outage preserves the original partial reading and reset time. The undocumented web protocol may change; unknown usage remains explicit when its response cannot be verified.

`aims usage --refresh` and the app's automatic five-minute refresh do not send a model prompt. Claude reads the exact profile's existing OAuth access through aims and requests Anthropic's usage API with a bounded timeout and response size. It never follows redirects, renews a token, writes credentials or falls back to a paid probe. If access has expired or is rejected, use that account in Claude Code and retry Refresh; API/token overrides do not fall back to an unrelated subscription login. The menu app receives only the credential-free usage snapshot. Native status-line reports continue to update the cache independently.

The refresh timestamp of one provider never makes another provider's numbers fresh. A failure retains the original observation time and reports the reason. When the provider omits a reset time, Compact/Gauges show **Not reported** with an explanatory tooltip; a known percentage still appears normally. After a window's reset passes, Compact/Gauges details show **— · Reset passed**; the tooltip keeps the previous percentage explicitly labeled as the previous period. The Dock and menu summary exclude it. A new zero is shown only when the provider actually reports zero; a reset timestamp passing never invents a new allowance.

The passive `--refresh` path writes private, versioned `usage-observations.json`, separate from routing/login marks in `state.json`. It never rewrites that routing file or `config.json`. Current readers combine observations only for the matching profile registration; login, logout, removal and recreation invalidate them. Native window updates retain API-only model groups and reset inventories with a conservative timestamp for the oldest contributing data. A native check without quotas keeps the prior measurement for the same known identity; an older successful observation cannot erase a newer native failure. Native account corrections fence in-flight reads and invalidate observations for the previous email or workspace. Losing known email/workspace metadata also requires a fresh observation. New cache replacements enforce owner-private permissions and ACLs, while nonregular or symlinked cache paths are kept for inspection. Optional cache cleanup never blocks login, logout or profile removal; doctor reports ignored stale rows. Earlier unbound cache rows require a fresh free usage read and are kept until replaced. A local cache update or reminder edit does not clear a failed provider-refresh indicator. Damaged or newer observation formats are kept, reported by the app and `aims doctor --check`, and never overwritten by refresh or doctor. Credentials and history are not copied or migrated by this bookkeeping.

Finder does not load shell exports. For a custom `AIMS_HOME`, select the existing folder in the app's Settings; the selected data location stays in Settings. A custom home inherited on the first launch is remembered. Additional CLI folders can be entered one per line, including Go/Cargo/npm or version-manager paths. No shell startup script is run, and changing the data location never moves files.

The installed Codex CLI 0.169.0 protocol was inspected with `app-server generate-ts --experimental`. It documents that the reset-credit detail list may be absent or capped, and that `availableCount` is independent of the list length. Other Codex versions may not report this field. Additional quota buckets are for display and do not alter the existing main-account failover decision.

OpenAI distinguishes [banked, automatic promotional and purchased resets](https://help.openai.com/en/articles/20001498-how-resets-work-in-codex) from scheduled usage refreshes. Promotional resets have no guaranteed schedule; aims does not predict a future grant or redeem an available one. Google's [native quota command](https://antigravity.google/docs/cli/commands/usage) reports model groups that share limits. Kiro's [billing documentation](https://kiro.dev/docs/billing/) describes renewal at the next billing cycle; that URL uses the canonical trailing slash because the slashless URL redirects. OpenCode's [budget documentation](https://opencode.ai/docs/console/budgets) distinguishes overlapping budget policies, and its [API schema](https://opencode.ai/docs/api) reports their individual reset timestamps. The app uses the returned timestamp rather than constructing a billing date.

## JSON and custom reminders

The snapshot has `schemaVersion: 1`, `generatedAt`, ordered `providers`, and `customResets`. Rows retain their own `observedAt`, windows, optional banked-reset balance, and errors. `bankedResets: null` means unavailable; `{ "available": 0, "credits": null }` means a reported empty inventory without details. A detail list shorter than the reported inventory is labeled incomplete. `noExpiry: true` means the provider explicitly reported a non-expiring reset; an absent expiry alone stays unknown. `pctUnknown: true` means a numeric usage ratio is unavailable, including a zero-dollar budget. This is different from zero percent usage.

```sh
aims usage --refresh --json
aims usage agy --json
aims usage reset add codex personal --at 2026-12-01T09:00:00+03:00 --label 'Check reset'
aims usage reset remove REMINDER_ID
```

Custom reminders use a separate private file, `AIMS_HOME/usage-reminders.json`. Damaged or newer reminder formats are retained and reported; they do not suppress other usage data. To repair a malformed file, preserve a copy and correct its JSON. Removing the macOS app does not remove reminders, aims configuration, logins, sessions or history.

## CLI compatibility and update ownership

Quota refresh, authentication keepalive and software upgrades are separate operations. The login keepalive runs hourly through the native tools and renews due logins; it does not redeem a quota reset. `aims upgrade` updates the standalone aims installation through its owner. Provider CLIs keep their own installers/updaters, and a running native process keeps the version it started with.

New provider releases can change commands and data contracts. The adapters are checked with fixtures and selected installed native versions; CI does not claim live authentication/usage coverage for every future CLI release. Invalid replies retain failures or unknown data rather than creating zero usage. Codex's required `usedPercent` field is checked for presence: explicit zero is known zero, while a missing/null/renamed value is unknown, including model/feature groups.

The development app and standalone aims may be different versions. Older aims cannot read the new passive-observation cache; its whole-file state writes cannot erase the app's separate reset/group observations. Old state writers can omit identity revision fields, making the app require a fresh free usage read instead of borrowing an uncertain measurement. Old configuration writers can still omit newer registration/removal metadata, so align the standalone CLI and scheduled keepalive through their supported release paths before production use. App refresh does not rewrite or upgrade the standalone CLI, provider CLIs or existing sessions.

## Application updates

Aims Usage includes pinned Sparkle 2.10.1 support. Signed release builds with a configured HTTPS update feed and public verification key offer **Check for Updates…** in More and Settings → Updates. Automatic checks are opt-in, and automatic installation and system profiling default to off. The menu, Edge Dock and bundled aims update together. A custom aims executable or the provider CLIs still update through their own installers.

The prepared Homebrew cask is named `aims-usage`, declares Sparkle updates, installs `Aims Usage.app` and declares the standalone `aims` cask as a dependency. The app cask has no separate binary link or profile/login cleanup; an existing CLI keeps its installer ownership. The release preparation workflow retains the signed ZIP, appcast and cask for review without publishing them.

Development and fixture preview builds have no active update channel. CI development artifacts are not public updater releases. Preparing a public release requires signing/notarization credentials, the matching Sparkle private key and publication of the signed archive plus appcast. Installation/relaunch still needs a two-build smoke test before enabling a production feed. Removing or updating the app does not remove aims data, logins, reminders or history.

Failed provider commands retain the previous reading and its original observation time. Non-empty stdout from a non-zero native CLI exit is not accepted as fresh quota data. Claude status-line cache updates also recognize removed windows and changed unknown/blocked/reset metadata; a removed allowance does not linger merely because another window is unchanged.
