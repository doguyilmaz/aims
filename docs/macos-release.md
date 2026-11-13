# Aims Usage release operations

The menu, Edge Dock and pinned overview are surfaces of one app, `com.doguyilmaz.aims.usage`. Sparkle updates that bundle and its bundled aims executable. It does not upgrade provider CLIs, a custom aims executable, the standalone installed aims CLI, or the keepalive `aims.app` wrapper produced by GoReleaser. Those retain their existing package-manager ownership.

Development builds use ad hoc signing and have no active update feed. Preview also explicitly refuses updater configuration and uses a different bundle ID. Linking Sparkle does not enable update checks. Configured release builds expose manual checks and an opt-in automatic-check preference; installation still uses Sparkle's standard user flow. System profiling and automatic installation default to off.

## Distribution ownership

The existing CLI release already uses the central [Tapster hub](https://github.com/doguyilmaz/homebrew-tap/blob/main/tapster.yml). Keep its certificate, notarization and publishing secrets there. The Usage app is a separate bundle/version stream; the current hub entry describes the CLI/keepalive wrapper and does not register this app.

Tapster supports Sparkle, but its current [app validation](https://github.com/doguyilmaz/tapster/blob/c19b315fdf14dd7b6613e4fc33d13115f6e17d94/src/appcheck.ts#L141) expects the app marketing version **and build** to equal a `vMAJOR.MINOR.PATCH` tag and derives the feed from the repository's latest release. Aims Usage keeps an integer build number separate from its SemVer marketing version and shares this repository with independently versioned CLI releases. Do not route it through that contract unchanged or reuse the CLI's generic latest-release feed. Central adoption needs explicit integer-build and independent release/feed support, plus the Usage bundle and nested Sparkle helpers registered in the hub. This repository does not change the hub or relocate its credentials.

The manual preparation workflow below is an **optional standalone preparation path**, not an active replacement for Tapster. It only produces reviewable artifacts and is unconfigured by default. Prefer the central signing service once its app contract accommodates this bundle; do not copy hub secrets into the project merely to make this workflow runnable. No production updater channel is activated by these changes.

## Optional standalone preparation setup

Use the pinned [Sparkle 2.10.1 distribution](https://github.com/sparkle-project/Sparkle/releases/tag/2.10.1). Its release archive SHA-256 is `1a856d2a9edb8e628de88121d7be04cf0e2167f8641650b78fca00b14244cf27`; the workflow verifies it before extracting tools. SwiftPM separately verifies the pinned framework artifact checksum.

1. Generate a dedicated Sparkle key with `bin/generate_keys --account com.doguyilmaz.aims.usage`. Retain the private key securely with an independent backup. Do not replace or reuse another app's Keychain account. Use the tool's export option to provide the release runner with this dedicated key; never commit it or print it in CI logs.
2. Choose a stable HTTPS appcast address under an owner-controlled domain or a dedicated GitHub release asset. It must remain stable across versions. Do not use the repository's generic `releases/latest` path: CLI and usage-app releases have separate version streams.
3. Configure repository variables:

| Variable | Value |
| --- | --- |
| `AIMS_USAGE_FEED_URL` | Stable HTTPS appcast URL, without embedded credentials/query/fragment |
| `AIMS_USAGE_PUBLIC_ED_KEY` | Dedicated Sparkle public Ed25519 key, base64, 32 decoded bytes |
| `AIMS_USAGE_SIGNING_IDENTITY` | Developer ID Application identity SHA-1 or full identity name |

4. Only if deliberately choosing this standalone preparation path, configure these secrets on trusted release runners. They are not required for development/CI builds and must not be copied from the central hub as routine setup:

| Secret | Value |
| --- | --- |
| `AIMS_MACOS_CERTIFICATE_P12` | Base64 exported Developer ID Application certificate/private key |
| `AIMS_MACOS_CERTIFICATE_PASSWORD` | P12 export password |
| `AIMS_SPARKLE_PRIVATE_KEY` | Exported dedicated Sparkle key file contents |
| `AIMS_NOTARY_KEY` | App Store Connect notarization API `.p8` contents |
| `AIMS_NOTARY_KEY_ID` | Notarization API key ID |
| `AIMS_NOTARY_ISSUER` | Notarization API issuer ID |

The **Prepare macOS usage release** workflow runs only when manually dispatched. It validates required configuration, tests the app, imports the certificate into a disposable runner Keychain, builds universal binaries, signs/notarizes/staples, and retains release artifacts for review. It has read-only repository permissions and does not create a release, publish an appcast, or deploy a website. Its final cleanup deletes the temporary Keychain and key files, even on failure. Keep the existing merge hold until outstanding review requirements are resolved.

## Prepare and publish a version

Dispatch the workflow with a three-part app version, a build integer greater than **every previously published build**, and the bundled CLI SemVer. Sparkle compares the integer `CFBundleVersion`; increasing only the marketing version is insufficient. Do not reuse a published build number. The local builder defaults to development `0.3.0`, build `2`; production values must be supplied explicitly.

For local preparation, set those same `AIMS_USAGE_*` variables, `AIMS_CLI_VERSION`, `AIMS_SPARKLE_TOOLS` (distribution `bin` directory), `AIMS_SPARKLE_PRIVATE_KEY_FILE`, and `AIMS_USAGE_DOWNLOAD_URL_PREFIX`. Supply an existing `AIMS_NOTARY_KEYCHAIN_PROFILE`, or the `AIMS_NOTARY_KEY_FILE`, `AIMS_NOTARY_KEY_ID` and `AIMS_NOTARY_ISSUER` API-key inputs. Then run:

```sh
scripts/release-macos-usage.sh
```

Output is `dist/usage-release-VERSION-BUILD`: a universal ZIP, `usage-appcast.xml`, and `checksums.txt`. A repeated output directory is refused. No publication occurs. The workflow prepares archive URLs under `https://github.com/OWNER/REPO/releases/download/usage-vVERSION-BUILD`.

After the update smoke test and release approval, publish that versioned GitHub release/tag and archive at the exact generated URL. Upload/replace the appcast at the configured stable feed address **last**, after the archive is available. Keep the signed appcast bytes generated by Sparkle. The release script rejects a missing archive signature, mismatched version/size/URL, or failed cryptographic verification of either archive or feed. The `usage-v` tag prefix does not trigger the CLI's `v*` GoReleaser workflow. Keep old archives available for existing feeds and rollback investigation. Never make a development CI ZIP an updater release.

The first public install needs to contain this updater support; already installed development apps without an active channel require one manual replacement. Do not promise that today's ad hoc build will update itself later.

## Required release smoke test

Before activating the production feed, use two notarized signed builds with the **same bundle ID, public key and signing identity**, increasing the build integer. Use a dedicated test feed with HTTPS and dummy data, not a live account home.

1. Install the older build in a normal writable Applications folder. Confirm automatic checks are off initially, then manually check the test feed.
2. Offer the newer build. Verify displayed marketing version, signature validation, installation and relaunch. Confirm every architecture's supported Mac can launch the app and its bundled aims.
3. Preserve non-secret dummy paths, Compact/Gauges, Dock edge/size/position/autohide, and a dummy custom reminder across replacement. Verify the old process and its refresh child exit before relaunch; a separately installed aims executable remains unchanged.
4. Reject a tampered archive, wrong signing key, older build number, malformed/unavailable feed, interrupted download and read-only installation. The older app and its data must remain usable. Use only a bounded disposable local setup for negative checks.
5. Verify the older build remains usable if the new release is withdrawn. Never decrement a published build to force rollback; release a repaired build with a higher integer.

The current repository tests validate configuration rejection, empty-channel behavior and packaging. They do not establish a successful production update until this signed two-build exercise is recorded.

Sparkle's [programmatic setup](https://sparkle-project.org/documentation/programmatic-setup/) and [publishing guide](https://sparkle-project.org/documentation/publishing/) define the updater/appcast behavior. These documentation URLs use their canonical trailing slash because the slashless URLs redirect. The build preserves framework symlinks and signs nested helpers before the containing framework/app; no library-validation exception is added.
