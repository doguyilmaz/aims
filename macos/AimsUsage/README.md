# Aims Usage for macOS

Native menu-bar overview and optional Edge Dock. For installation and everyday controls, read the [app guide](https://aims.doguyilmaz.com/macos-usage). For provider/cache semantics, see [Usage data and privacy](https://aims.doguyilmaz.com/usage-data).

## Development

Requires macOS 14.0.0+, Swift 6.2.0+ and the Go version in the repository’s `go.mod`.

```sh
scripts/build-macos-app.sh
open 'dist/Aims Usage.app'
swift test --package-path macos/AimsUsage
swift test --configuration release --package-path macos/AimsUsage
```

The builder ad hoc signs a development app, bundles the matching aims CLI and removes temporary compilation products. It does not replace an installed app or enable production updates.

| Input | Default / purpose |
| --- | --- |
| `AIMS_USAGE_VERSION` | `0.3.0` (SemVer) |
| `AIMS_USAGE_BUILD_NUMBER` | `2` (positive integer) |
| `AIMS_CLI_VERSION` | `0.4.7-dev` |
| `AIMS_USAGE_CLI_BINARY` | Optional prebuilt CLI for this Mac |
| `AIMS_USAGE_UNIVERSAL=1` | Build Apple Silicon and Intel |
| `AIMS_USAGE_SOURCE_REVISION` | Override the automatic Git/dirty label |

## Fixture preview

```sh
AIMS_USAGE_PREVIEW=1 scripts/build-macos-app.sh
open 'dist/Aims Usage Preview.app'
```

Preview has a separate preferences domain, bundled illustrative data and no provider calls. `AIMS_USAGE_FIXTURE=/absolute/path/snapshot.json` selects a local fixture. It uses the same UI components and entry points as the app.

## Packaging

The app ships one application icon. Provider marks come from the pinned [Lobe assets](../../packaging/provider-marks); Sparkle 2.10.1 and its notices are [bundled](../../packaging/sparkle). No icon package is downloaded at runtime.

Development builds have no updater feed. Signed universal builds, notarization, appcast generation and the separate Homebrew cask are described in [release preparation](../../docs/macos-release.md). Production activation is a separate step.

Documentation previews use illustrative fixture data and PNG alpha at 2× scale. Their opaque material matches Reduce Transparency, keeping exported views independent of the desktop behind them. No live account data is used.
