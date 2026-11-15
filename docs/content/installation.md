---
title: Installation
weight: 2
aliases: ["/docs/installation/"]
---

Choose the interface you need:

| Interface | Availability |
| --- | --- |
| **aims CLI** | Released for macOS, Linux and Windows; installation methods below |
| **Aims Usage for macOS** | [Source preview](#aims-usage-for-macos) with menu bar, optional Edge Dock and pinned overview |

The CLI is one static binary with no runtime to install. It needs Claude Code, Codex, or both; provider CLIs are installed separately.

## Install

These commands install the **CLI**. The Homebrew `aims` cask includes the CLI's keepalive wrapper on macOS; it does not install Aims Usage.

{{< tabs >}}
  {{< tab name="Homebrew" >}}
macOS and Linux:

```bash
brew install --cask doguyilmaz/tap/aims
```

Shell completion for bash, zsh and fish comes with it. `aims upgrade` (or `brew upgrade --cask doguyilmaz/tap/aims`) keeps it current.
  {{< /tab >}}
  {{< tab name="Script" >}}
macOS and Linux, no Homebrew and no sudo:

```bash
curl -fsSL https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.sh | sh
```

It downloads the release for your system, checks it against the release's SHA-256 checksums (and stops on a mismatch), and puts `aims` in `~/.local/bin`. If that folder is not on your `PATH`, it prints the line to add.

| Variable | Default | Meaning |
| --- | --- | --- |
| `AIMS_VERSION` | the latest release | A tag such as `v0.2.0` |
| `AIMS_BIN_DIR` | `~/.local/bin` | Where to install |
| `AIMS_BASE_URL` | the GitHub releases page | Download from a mirror |
  {{< /tab >}}
  {{< tab name="npm" >}}
```bash
npm install -g @doguyilmaz/aims
```

The package holds the binaries for every supported platform (about 25 MB) and a small launcher that runs the one for your machine. It has no install scripts, so it also works with `--ignore-scripts`.
  {{< /tab >}}
  {{< tab name="Windows" >}}
PowerShell:

```powershell
irm https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.ps1 | iex
```

It installs to `%LOCALAPPDATA%\Programs\aims` and adds that folder to your user `PATH`. Or use [Scoop](https://scoop.sh):

```powershell
scoop bucket add doguyilmaz https://github.com/doguyilmaz/homebrew-tap
scoop install aims
```
  {{< /tab >}}
  {{< tab name="Go" >}}
```bash
go install github.com/doguyilmaz/aims/cmd/aims@latest
```

This puts `aims` in `$(go env GOPATH)/bin`. Make sure that folder is on your `PATH`.
  {{< /tab >}}
{{< /tabs >}}

Release archives for every platform, with `checksums.txt`, are on the [releases page](https://github.com/doguyilmaz/aims/releases).

## Aims Usage for macOS

The menu bar, Edge Dock and overview ship together in **Aims Usage.app**. The app includes an aims executable and reads your existing aims configuration and native provider logins. Standalone CLI/package-manager updates remain separate.

**Source preview:** the signed public app archive, `aims-usage` cask and production Sparkle feed have not been activated. Build from a checkout containing the app on macOS 14.0.0+ with Swift 6.2.0+ and the Go version declared in `go.mod` available on `PATH`:

```sh
scripts/build-macos-app.sh
'dist/Aims Usage.app/Contents/Resources/aims' app install --from 'dist/Aims Usage.app'
"$HOME/Applications/Aims Usage.app/Contents/Resources/aims" app open
```

This copies the verified bundle to `~/Applications` without replacing an existing app. It exposes the bundled CLI at `~/.local/bin/aims` only when no terminal CLI exists; add that directory to PATH if needed. Existing CLI installations and shell files remain intact. Set up Claude/Codex profiles with `aims init`; for an existing custom `AIMS_HOME`, select that same folder in the app's Settings. Provider CLIs manage their own logins. See [Aims Usage for macOS](../macos-usage) for the complete guide and preview images.

On macOS, the CLI offers `aims app`, `aims app install`, `aims app open` and `aims app status --json`; the TUI offers **m**, and init/setup expose `--app`. Public installation uses the app cask after its release; source previews use `--from` above. The prepared app cask also installs the CLI dependency, while a direct download already includes a bundled CLI. **More → Terminal command…** in the app adds that command to `~/.local/bin` without overwriting an existing CLI. These app-specific commands/options and doctor checks are absent on Linux/Windows. An absent optional app is informational in doctor and never installed by `doctor --fix`.

## Check it works

```bash
aims --version
aims doctor
```

`doctor` checks paths, shared folders and integrations without changes or credential access. Use `aims doctor --fix` for conservative repairs and `aims doctor --logins` for explicit local login/version inspection. Then go on with the [Quick start](../quick-start).

## Supported platforms

| OS | Architectures | Shared folders are linked with |
| --- | --- | --- |
| macOS | `arm64`, `amd64` | symlinks |
| Linux | `arm64`, `amd64` | symlinks |
| Windows | `amd64` | symlinks, or junctions and hard links without Developer Mode |

## Shell completion

Homebrew installs completion for you. Otherwise `aims setup --shell` adds one guarded line to your shell's startup file that loads completion together with the [shell integration](../everyday#plain-claude-and-codex). To load only completion:

{{< tabs >}}
  {{< tab name="zsh" >}}
```bash
source <(aims completion zsh)
```
  {{< /tab >}}
  {{< tab name="bash" >}}
```bash
source <(aims completion bash)
```
  {{< /tab >}}
  {{< tab name="fish" >}}
```fish
aims completion fish | source
```
  {{< /tab >}}
  {{< tab name="PowerShell" >}}
```powershell
aims completion powershell | Out-String | Invoke-Expression
```
  {{< /tab >}}
{{< /tabs >}}

Completion knows your tools and profile names, so `aims use <Tab>` lists your accounts.

## Upgrade

```bash
aims upgrade
```

aims works out how it was installed (Homebrew, Scoop, npm or `go install`) and runs that tool's upgrade. A copy where the install script puts it (`AIMS_BIN_DIR`, by default `~/.local/bin`) is upgraded by running the script again, which checks the download against the release's checksums. A copy anywhere else gets the command that installs the newest release. `aims upgrade --check` only prints what it would run.

aims checks for a new release at most once a day, in the background, and mentions it after a command finishes. It asks GitHub where its "latest release" page points and sends nothing about you. Set `AIMS_NO_UPDATE_CHECK=1` to turn this off.

## Uninstall

To remove the CLI, use its package manager (`brew uninstall --cask doguyilmaz/tap/aims`, `npm uninstall -g @doguyilmaz/aims`) or remove the installed binary. To remove Aims Usage, quit it and move its app bundle to Trash. These steps preserve aims profiles, provider logins, sessions, history and reminders.

`aims wipe` is an explicit, separate data-removal operation: it logs out and deletes the profiles aims created. Use it only when you also want that account data removed; ordinary uninstallation does not require it. The hub's `~/.claude` and `~/.codex` remain, including shared data. Private profile data can be removed by wipe, so preserve anything you need before choosing it.
