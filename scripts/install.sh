#!/bin/sh
# Installs the aims binary from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/doguyilmaz/aims/main/scripts/install.sh | sh
#
# AIMS_VERSION    release tag to install (default: the latest)
# AIMS_BIN_DIR    where to put the binary (default: ~/.local/bin)
# AIMS_BASE_URL   releases page or a mirror of it
set -eu

base_url="${AIMS_BASE_URL:-https://github.com/doguyilmaz/aims/releases}"
bin_dir="${AIMS_BIN_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf 'aims install: %s\n' "$*" >&2; exit 1; }
has() { command -v "$1" >/dev/null 2>&1; }

fetch() { # url -> stdout
	if has curl; then curl -fsSL "$1"; elif has wget; then wget -qO- "$1"; else die "needs curl or wget"; fi
}

download() { # url file
	if has curl; then curl -fsSL -o "$2" "$1"; else wget -qO "$2" "$1"; fi
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) die "unsupported system $(uname -s); on Windows use scripts/install.ps1" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "unsupported CPU $(uname -m)" ;;
esac

version="${AIMS_VERSION:-}"
if [ -z "$version" ]; then
	# The latest release page redirects to its tag; no API token needed.
	if has curl; then
		url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$base_url/latest")
	else
		url=$(wget -S --spider "$base_url/latest" 2>&1 | sed -n 's/^ *Location: *//p' | tail -n 1)
	fi
	version="${url##*/}"
fi
case "$version" in
v[0-9]*) ;;
*) die "could not find the latest release (got '$version'); set AIMS_VERSION=vX.Y.Z" ;;
esac

archive="aims_${version#v}_${os}_${arch}.tar.gz"
base="$base_url/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading aims $version for $os/$arch"
download "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive"
fetch "$base/checksums.txt" >"$tmp/checksums.txt" || die "could not download checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "$archive is not listed in checksums.txt"
if has sha256sum; then
	got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
elif has shasum; then
	got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
else
	die "needs sha256sum or shasum to verify the download"
fi
[ "$got" = "$want" ] || die "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" aims
mkdir -p "$bin_dir"
# Replace through a temporary name so a running aims is never half-written.
cp "$tmp/aims" "$bin_dir/.aims.new"
chmod 755 "$bin_dir/.aims.new"
mv -f "$bin_dir/.aims.new" "$bin_dir/aims"

say "Installed $("$bin_dir/aims" --version) to $bin_dir/aims"
case ":$PATH:" in
*":$bin_dir:"*) ;;
*)
	case "${SHELL##*/}" in
	fish) hint="fish_add_path $bin_dir" ;;
	zsh) hint="echo 'export PATH=\"$bin_dir:\$PATH\"' >> ~/.zshrc" ;;
	*) hint="echo 'export PATH=\"$bin_dir:\$PATH\"' >> ~/.bashrc" ;;
	esac
	say "$bin_dir is not on your PATH. Add it with: $hint"
	;;
esac
say "Next: aims init"
