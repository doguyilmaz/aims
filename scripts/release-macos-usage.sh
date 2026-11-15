#!/bin/bash
# Prepare a notarized, signed update locally. This script never publishes it.
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
: "${AIMS_USAGE_VERSION:?Set the MAJOR.MINOR.PATCH app version}"
: "${AIMS_USAGE_BUILD_NUMBER:?Set a monotonically increasing integer build number}"
: "${AIMS_USAGE_SIGNING_IDENTITY:?Set the Developer ID Application identity}"
: "${AIMS_USAGE_FEED_URL:?Set the stable HTTPS appcast URL}"
: "${AIMS_USAGE_PUBLIC_ED_KEY:?Set the Sparkle public key}"
: "${AIMS_SPARKLE_TOOLS:?Set the Sparkle bin directory}"
: "${AIMS_SPARKLE_PRIVATE_KEY_FILE:?Set the private signing-key file path}"
: "${AIMS_USAGE_DOWNLOAD_URL_PREFIX:?Set the versioned HTTPS release asset directory}"
if [[ "$AIMS_USAGE_SIGNING_IDENTITY" == - || ! -f "$AIMS_SPARKLE_PRIVATE_KEY_FILE" ]]; then
  echo 'A signed release requires a Developer ID identity and Sparkle signing-key file.' >&2
  exit 2
fi
if [[ ! -x "$AIMS_SPARKLE_TOOLS/generate_appcast" ]]; then
  echo 'AIMS_SPARKLE_TOOLS must contain generate_appcast.' >&2
  exit 2
fi
export AIMS_USAGE_UNIVERSAL=1 AIMS_USAGE_PREVIEW=0
python3 -B "$repo/scripts/configure-macos-updates.py" --validate
python3 -B "$repo/scripts/notarize-macos-usage.py" --validate
output="$repo/dist/usage-release-${AIMS_USAGE_VERSION}-${AIMS_USAGE_BUILD_NUMBER}"
if [[ -e "$output" || -L "$output" ]]; then
  echo "Refusing to overwrite existing release artifacts: $output" >&2
  exit 1
fi
"$repo/scripts/build-macos-app.sh"
app="$repo/dist/Aims Usage.app"
scratch=$(mktemp -d "${TMPDIR:-/tmp}/aims-usage-release.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
ditto -c -k --sequesterRsrc --keepParent "$app" "$scratch/notarization.zip"
python3 -B "$repo/scripts/notarize-macos-usage.py" "$scratch/notarization.zip"
xcrun stapler staple "$app"
xcrun stapler validate "$app"
codesign --verify --deep --strict "$app"
spctl --assess --type execute "$app"
mkdir "$output"
archive="$output/Aims-Usage-${AIMS_USAGE_VERSION}-${AIMS_USAGE_BUILD_NUMBER}-universal.zip"
ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
"$AIMS_SPARKLE_TOOLS/generate_appcast" --ed-key-file "$AIMS_SPARKLE_PRIVATE_KEY_FILE" \
  --download-url-prefix "$AIMS_USAGE_DOWNLOAD_URL_PREFIX" -o "$output/usage-appcast.xml" "$output"
python3 -B "$repo/scripts/verify-macos-appcast.py" "$output/usage-appcast.xml" "$archive"
python3 -B "$repo/scripts/prepare-macos-usage-cask.py" "$archive" \
  --version "$AIMS_USAGE_VERSION" --build "$AIMS_USAGE_BUILD_NUMBER" \
  --url-prefix "$AIMS_USAGE_DOWNLOAD_URL_PREFIX" --output "$output/aims-usage.rb"
(cd "$output" && shasum -a 256 ./*.zip usage-appcast.xml aims-usage.rb > checksums.txt)
echo "Prepared signed artifacts in $output; no release or update feed was published."
