# Aims Usage release operations

The menu, Edge Dock and pinned overview are surfaces of one app, `com.doguyilmaz.aims.usage`. Sparkle updates that bundle and its bundled aims executable. It does not upgrade provider CLIs, a custom aims executable, the standalone installed aims CLI, or the keepalive `aims.app` wrapper produced by GoReleaser. Those retain their existing package-manager ownership.

Development builds use ad hoc signing and have no active update feed. Preview also explicitly refuses updater configuration and uses a different bundle ID. Linking Sparkle does not enable update checks. Configured release builds expose manual checks and an opt-in automatic-check preference; installation still uses Sparkle's standard user flow. System profiling and automatic installation default to off.

## Existing app examples and distribution ownership

The [Homebrew tap](https://github.com/doguyilmaz/homebrew-tap/tree/main/Casks) has native Sparkle apps outside its Tapster-managed CLI packages:

| Example | Release pattern worth reusing |
| --- | --- |
| [Battistics](https://github.com/doguyilmaz/battistics/blob/main/.github/workflows/release.yml) | Separate SemVer marketing version and integer build, signing/notarization, signed appcast, then cask update. Its accessory-app updater explicitly activates the app before showing a manual update alert. |
| [Recesso](https://github.com/doguyilmaz/recesso/blob/main/.github/workflows/release.yml) | Validated version/build, notarized final archive, Sparkle signature and an `auto_updates true` cask. |
| [HafifPix](https://github.com/doguyilmaz/hafifpix/blob/main/scripts/update-tap.sh) | Hash the actual release artifact and update exactly one cask. The app's bundled CLI is optional; Aims Usage intentionally exposes no global CLI alias. |

Aims Usage follows this native-app preparation pattern. The existing CLI/keepalive wrapper continues through Tapster. `Casks/aims.rb` belongs to that CLI; the Usage app gets **`Casks/aims-usage.rb`** and bundle ID `com.doguyilmaz.aims.usage`. Installing or uninstalling it must not replace the standalone aims executable or remove `~/.aims`, provider homes or login data. The prepared cask has no `binary` or `zap` stanza.

Tapster also supports Sparkle, but its current [app validation](https://github.com/doguyilmaz/tapster/blob/c19b315fdf14dd7b6613e4fc33d13115f6e17d94/src/appcheck.ts#L141) expects marketing version and build to equal one SemVer tag and derives a latest-release feed. Central adoption would need separate integer-build/release-stream support. That is an optional future consolidation, not a prerequisite for this app's preparation. No hub changes or secret migration are needed now.

The manual **Prepare macOS usage release** workflow builds the app, signs/notarizes/staples it, verifies its signed archive/feed, and prepares a local cask. It is read-only and retains reviewable artifacts; it never publishes a GitHub release, writes to the tap or activates the feed. Live activation and its signed two-build smoke test happen later.

## Release preparation inputs

Use the pinned [Sparkle 2.10.1 distribution](https://github.com/sparkle-project/Sparkle/releases/tag/2.10.1). Its release archive SHA-256 is `1a856d2a9edb8e628de88121d7be04cf0e2167f8641650b78fca00b14244cf27`; the workflow verifies it before extracting tools. SwiftPM separately verifies the pinned framework artifact checksum.

1. Generate a dedicated Sparkle key with `bin/generate_keys --account com.doguyilmaz.aims.usage`. Retain the private key securely with an independent backup. Do not replace or reuse another app's Keychain account. Use the tool's export option to provide the release runner with this dedicated key; never commit it or print it in CI logs.
2. Choose a stable HTTPS appcast address independent of CLI releases. The existing site can serve `https://aims.apps.doguyilmaz.com/usage-appcast.xml` (the eventual signed file belongs under `docs/static/usage-appcast.xml`). Do not commit a dummy feed or use this repository's generic `releases/latest` path: a subsequent CLI release would change that target. Archive downloads remain pinned to `usage-vVERSION-BUILD`. The feed is published last at activation, after its archive exists.
3. Configure repository variables:

| Variable | Value |
| --- | --- |
| `AIMS_USAGE_FEED_URL` | Stable HTTPS appcast URL, without embedded credentials/query/fragment |
| `AIMS_USAGE_PUBLIC_ED_KEY` | Dedicated Sparkle public Ed25519 key, base64, 32 decoded bytes |
| `AIMS_USAGE_SIGNING_IDENTITY` | Developer ID Application identity SHA-1 or full identity name |

4. At release activation, make the appropriate inputs available to the trusted runner. The workflow accepts the names already used by the native app examples, as well as the Aims-specific names below. All references resolve inside this repository; no other app's secrets are read or copied. Reuse the intended Developer ID/team through your normal secret provisioning, and keep a dedicated Sparkle key for Aims Usage. None of these credentials is required for development/CI builds.

| Secret input | Value |
| --- | --- |
| `AIMS_MACOS_CERTIFICATE_P12` or `MACOS_CERT_P12` | Base64 Developer ID Application certificate/private key |
| `AIMS_MACOS_CERTIFICATE_PASSWORD` or `MACOS_CERT_PASSWORD` | P12 password |
| `AIMS_SPARKLE_PRIVATE_KEY` or `SPARKLE_PRIVATE_KEY` | This app's dedicated exported Sparkle key |
| `MACOS_SIGN_IDENTITY` | Accepted when the public `AIMS_USAGE_SIGNING_IDENTITY` variable is unset |
| `AIMS_NOTARY_KEY`, `AIMS_NOTARY_KEY_ID`, `AIMS_NOTARY_ISSUER` | Optional complete notarization API-key mode |
| `APPLE_ID`, `APPLE_TEAM_ID`, `NOTARY_PASSWORD` | Alternative Apple ID + app-specific-password mode, as in Battistics/Recesso |

If any API-key input is set, all three must be present. Otherwise the workflow requires the complete Apple ID mode. Partial authentication fails before expensive builds. No certificate, private key, app password or tap-write token has been provisioned by these changes. Preparing the cask needs no tap-write token.

The **Prepare macOS usage release** workflow runs only when manually dispatched. It validates required configuration, tests the app, imports the certificate into a disposable runner Keychain, builds universal binaries, signs/notarizes/staples, and retains release artifacts for review. It has read-only repository permissions and does not create a release, publish an appcast, or deploy a website. Its final cleanup deletes the temporary Keychain and key files, even on failure. Keep the existing merge hold until outstanding review requirements are resolved.

## Prepare and publish a version

Dispatch the workflow with a three-part app version, a build integer greater than **every previously published build**, and the bundled CLI SemVer. Sparkle compares the integer `CFBundleVersion`; increasing only the marketing version is insufficient. Do not reuse a published build number. The local builder defaults to development `0.3.0`, build `2`; production values must be supplied explicitly.

For local preparation, set those same `AIMS_USAGE_*` variables, `AIMS_CLI_VERSION`, `AIMS_SPARKLE_TOOLS` (distribution `bin` directory), `AIMS_SPARKLE_PRIVATE_KEY_FILE`, and `AIMS_USAGE_DOWNLOAD_URL_PREFIX`. Supply one notarization method, in this precedence:

- An existing `AIMS_NOTARY_KEYCHAIN_PROFILE`.
- `AIMS_NOTARY_KEY_FILE`, `AIMS_NOTARY_KEY_ID` and `AIMS_NOTARY_ISSUER`.
- `AIMS_NOTARY_APPLE_ID`, `AIMS_NOTARY_TEAM_ID` and `AIMS_NOTARY_PASSWORD` (an app-specific password).

The selected credentials and existing output are checked before building. Quit a development app running from this checkout before preparing a replacement. Then run:

```sh
scripts/release-macos-usage.sh
```

Output is `dist/usage-release-VERSION-BUILD`: a universal ZIP, `usage-appcast.xml`, prepared `aims-usage.rb`, and `checksums.txt` covering all three. A repeated output directory is refused. No publication occurs. The workflow prepares archive URLs under `https://github.com/OWNER/REPO/releases/download/usage-vVERSION-BUILD`.

After the update smoke test and release approval, publish that versioned GitHub release/tag and archive at the exact generated URL. Review the prepared cask and submit it as the single `Casks/aims-usage.rb` change in the tap; its SHA is of the exact signed/stapled ZIP, not an earlier build. Its version remains MAJOR.MINOR.PATCH; the integer build is pinned separately in the archive/tag URL. Publish the appcast at the configured stable feed address **last**, after the archive is available. Keep the signed appcast bytes generated by Sparkle. The release script rejects a missing archive signature, mismatched version/size/URL, or failed cryptographic verification of either archive or feed. The `usage-v` tag prefix does not trigger the CLI's `v*` GoReleaser workflow. Keep old archives available for existing feeds and rollback investigation. Never make a development CI ZIP an updater release.

The first public install needs to contain this updater support; already installed development apps without an active channel require one manual replacement. Do not promise that today's ad hoc build will update itself later.

The cask declares `auto_updates true` because Sparkle performs application updates. [Homebrew's upgrade rules](https://docs.brew.sh/Manpage) ordinarily skip those casks; explicit `--greedy` options include them. This does not itself enable Sparkle or publish anything.

To review a cask from a previously verified release ZIP without notarizing or publishing again:

```sh
python3 -B scripts/prepare-macos-usage-cask.py \
  dist/usage-release-0.3.0-2/Aims-Usage-0.3.0-2-universal.zip \
  --version 0.3.0 --build 2 \
  --url-prefix https://github.com/doguyilmaz/aims/releases/download/usage-v0.3.0-2
```

It prints to stdout by default. `--output <new-file>` refuses to overwrite an existing file. The normal release script invokes it only after archive/feed signature verification.

## Later production activation smoke test

This is a release activation gate, outside implementation/preparation completion. Before activating the production feed, use two notarized signed builds with the **same bundle ID, public key and signing identity**, increasing the build integer. Use a dedicated test feed with HTTPS and dummy data, not a live account home.

1. Install the older build in a normal writable Applications folder. Confirm automatic checks are off initially, then manually check the test feed.
2. Offer the newer build. Verify displayed marketing version, signature validation, installation and relaunch. Confirm every architecture's supported Mac can launch the app and its bundled aims.
3. Preserve non-secret dummy paths, Compact/Gauges, Dock edge/size/position/autohide, and a dummy custom reminder across replacement. Verify the old process and its refresh child exit before relaunch; a separately installed aims executable remains unchanged.
4. Reject a tampered archive, wrong signing key, older build number, malformed/unavailable feed, interrupted download and read-only installation. The older app and its data must remain usable. Use only a bounded disposable local setup for negative checks.
5. Verify the older build remains usable if the new release is withdrawn. Never decrement a published build to force rollback; release a repaired build with a higher integer.

The current repository tests validate configuration rejection, empty-channel behavior and packaging. They do not establish a successful production update until this signed two-build exercise is recorded.

Sparkle's [programmatic setup](https://sparkle-project.org/documentation/programmatic-setup/) and [publishing guide](https://sparkle-project.org/documentation/publishing/) define the updater/appcast behavior. These documentation URLs use their canonical trailing slash because the slashless URLs redirect. The build preserves framework symlinks and signs nested helpers before the containing framework/app; no library-validation exception is added.
