---
title: Usage data and privacy
description: Provider sources, reset semantics, cache preservation and update ownership.
weight: 30
---

This is the technical reference. For setup and everyday controls, use the [macOS app guide](../macos-usage).

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

Antigravity quota reads use the terminal CLI's login. The [T3 Antigravity provider](../other-apps#antigravity) has a separate sign-in and reports conversation usage in T3. Aims retains every reported model-group ID, including separate Gemini, Claude/GPT, Argon 4 and Carbon 4 groups when supplied. Groups keep their own usage and reset timestamps even when their bucket IDs match.

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

## Login probe resource controls

Quota Refresh sends no model prompt. An explicit Claude live login check or due-login keepalive uses the native `haiku` alias, low effort, one turn and a 64-output-token limit. It offers no tools or MCP servers, disables hooks and session persistence, and starts in a fresh temporary working directory. Automatic API retries and the retry watchdog are disabled for this check. Thinking is disabled on models that permit it; newer Haiku models still use adaptive thinking at low effort.

These controls live only in the child's environment and inline settings, including when the parent or saved settings request higher effort. The native CLI retains ownership of authentication renewal; aims does not change saved conversation models, effort or prompts. This is a small model-access check, not a measurement of every model's quota. Tests use fake providers and spend no model tokens. Codex login/limit checks use its account API without a model turn.

## CLI compatibility and update ownership

Quota refresh, authentication keepalive and software upgrades are separate operations. The login keepalive runs hourly through the native tools and renews due logins; it does not redeem a quota reset. `aims upgrade` updates the standalone aims installation through its owner. Provider CLIs keep their own installers/updaters, and a running native process keeps the version it started with.

New provider releases can change commands and data contracts. The adapters are checked with fixtures and selected installed native versions; CI does not claim live authentication/usage coverage for every future CLI release. Invalid replies retain failures or unknown data rather than creating zero usage. Codex's required `usedPercent` field is checked for presence: explicit zero is known zero, while a missing/null/renamed value is unknown, including model/feature groups.

The development app and standalone aims may be different versions. Older aims cannot read the new passive-observation cache; its whole-file state writes cannot erase the app's separate reset/group observations. Old state writers can omit identity revision fields, making the app require a fresh free usage read instead of borrowing an uncertain measurement. Old configuration writers can still omit newer registration/removal metadata, so align the standalone CLI and scheduled keepalive through their supported release paths before production use. App refresh does not rewrite or upgrade the standalone CLI, provider CLIs or existing sessions.

## Application updates

Aims Usage includes pinned Sparkle 2.10.1 support. Signed release builds with a configured HTTPS update feed and public verification key offer **Check for Updates…** in More and Settings → Updates & About. Automatic checks are opt-in, and automatic installation and system profiling default to off. The menu, Edge Dock and bundled aims update together. A custom aims executable or the provider CLIs still update through their own installers.

The prepared Homebrew cask is named `aims-usage`, declares Sparkle updates, installs `Aims Usage.app` and declares the standalone `aims` cask as a dependency. The app cask has no separate binary link or profile/login cleanup; an existing CLI keeps its installer ownership. The release preparation workflow retains the signed ZIP, appcast and cask for review without publishing them.

Development and fixture preview builds have no active update channel. CI development artifacts are not public updater releases. Preparing a public release requires signing/notarization credentials, the matching Sparkle private key and publication of the signed archive plus appcast. Installation/relaunch still needs a two-build smoke test before enabling a production feed. Removing or updating the app does not remove aims data, logins, reminders or history.

Failed provider commands retain the previous reading and its original observation time. Non-empty stdout from a non-zero native CLI exit is not accepted as fresh quota data. Claude status-line cache updates also recognize removed windows and changed unknown/blocked/reset metadata; a removed allowance does not linger merely because another window is unchanged.
