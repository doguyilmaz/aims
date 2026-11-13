#!/bin/bash
set -euo pipefail
app=${1:?Pass the app bundle}
identity=${AIMS_USAGE_SIGNING_IDENTITY:--}
args=(--force --sign "$identity")
if [[ "$identity" != - ]]; then
  args+=(--options runtime --timestamp)
fi
framework="$app/Contents/Frameworks/Sparkle.framework/Versions/B"
# Sign nested code inside out; preserve the framework's versioned symlinks.
for component in XPCServices/Downloader.xpc XPCServices/Installer.xpc Autoupdate Updater.app; do
  codesign "${args[@]}" "$framework/$component"
done
codesign "${args[@]}" "$app/Contents/Frameworks/Sparkle.framework"
codesign "${args[@]}" "$app/Contents/Resources/aims"
codesign "${args[@]}" "$app"
codesign --verify --deep --strict "$app"
