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
"$repo/scripts/build-macos-app.sh"
app="$repo/dist/Aims Usage.app"
scratch=$(mktemp -d "${TMPDIR:-/tmp}/aims-usage-release.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
ditto -c -k --sequesterRsrc --keepParent "$app" "$scratch/notarization.zip"
if [[ -n "${AIMS_NOTARY_KEYCHAIN_PROFILE:-}" ]]; then
  xcrun notarytool submit "$scratch/notarization.zip" --keychain-profile "$AIMS_NOTARY_KEYCHAIN_PROFILE" --wait
else
  : "${AIMS_NOTARY_KEY_FILE:?Set a notary API key file or AIMS_NOTARY_KEYCHAIN_PROFILE}"
  : "${AIMS_NOTARY_KEY_ID:?Set the notary API key ID}"
  : "${AIMS_NOTARY_ISSUER:?Set the notary API issuer}"
  xcrun notarytool submit "$scratch/notarization.zip" --key "$AIMS_NOTARY_KEY_FILE" \
    --key-id "$AIMS_NOTARY_KEY_ID" --issuer "$AIMS_NOTARY_ISSUER" --wait
fi
xcrun stapler staple "$app"
xcrun stapler validate "$app"
codesign --verify --deep --strict "$app"
spctl --assess --type execute "$app"
output="$repo/dist/usage-release-${AIMS_USAGE_VERSION}-${AIMS_USAGE_BUILD_NUMBER}"
if [[ -e "$output" || -L "$output" ]]; then
  echo "Refusing to overwrite existing release artifacts: $output" >&2
  exit 1
fi
mkdir "$output"
archive="$output/Aims-Usage-${AIMS_USAGE_VERSION}-${AIMS_USAGE_BUILD_NUMBER}-universal.zip"
ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
"$AIMS_SPARKLE_TOOLS/generate_appcast" --ed-key-file "$AIMS_SPARKLE_PRIVATE_KEY_FILE" \
  --download-url-prefix "$AIMS_USAGE_DOWNLOAD_URL_PREFIX" -o "$output/usage-appcast.xml" "$output"
python3 -B "$repo/scripts/verify-macos-appcast.py" "$output/usage-appcast.xml" "$archive"
(cd "$output" && shasum -a 256 ./*.zip usage-appcast.xml > checksums.txt)
echo "Prepared signed artifacts in $output; no release or update feed was published."
