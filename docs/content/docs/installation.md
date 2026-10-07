---
title: Installation
weight: 2
---

aims is one static binary with no runtime to install. It needs Claude Code, Codex, or both; it does not install them.

## Install

{{< tabs >}}
  {{< tab name="Homebrew" >}}
macOS and Linux:

```bash
brew install --cask doguyilmaz/tap/aims
```

Shell completion for bash, zsh and fish comes with it. `aims upgrade` (or `brew upgrade --cask aims`) keeps it current.
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

npm installs only the binary for your platform (through optional dependencies), plus a small launcher. Do not use `--omit=optional` or `--no-optional`.
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

## Check it works

```bash
aims --version
aims doctor
```

`doctor` lists what aims found: the tools and their versions, your logins, the shared folders and the integrations, with the command to fix anything that is off. Then go on with the [Quick start](../quick-start).

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

aims never replaces its own binary. It works out how it was installed (Homebrew, Scoop, npm or `go install`) and runs that tool's upgrade. A copy from the install script gets the command that installs the newest release. `aims upgrade --check` only prints what it would run.

aims checks for a new release at most once a day, in the background, and mentions it after a command finishes. It asks GitHub where its "latest release" page points and sends nothing about you. Set `AIMS_NO_UPDATE_CHECK=1` to turn this off.

## Uninstall

```bash
aims uninstall                # remove the skill, MCP server, status line and shell line
aims rm claude work --purge   # optional: log out and delete an account's folder
```

Then remove the binary the way you installed it (`brew uninstall --cask aims`, `npm uninstall -g @doguyilmaz/aims`, or delete the file). `~/.aims` holds the profile folders and settings; delete it last if you want everything gone. Your `~/.claude` and `~/.codex` stay as they are, including everything the accounts shared.
