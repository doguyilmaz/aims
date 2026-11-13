#!/bin/bash
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
python3 -B "$repo/scripts/configure-macos-updates.py" --validate
version=${AIMS_USAGE_VERSION:-0.3.0}
cli_version=${AIMS_CLI_VERSION:-0.4.7-dev}
build_number=${AIMS_USAGE_BUILD_NUMBER:-2}
source_revision=${AIMS_USAGE_SOURCE_REVISION:-$("$repo/scripts/source-revision.sh" "$repo")}
if [[ ! "$source_revision" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "AIMS_USAGE_SOURCE_REVISION must contain only letters, digits, dots, hyphens or underscores" >&2
  exit 2
fi
core='(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
if [[ ! "$version" =~ ^${core}$ ]]; then
  echo "AIMS_USAGE_VERSION must be MAJOR.MINOR.PATCH" >&2
  exit 2
fi
if [[ ! "$cli_version" =~ ^${core}(-[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*)?(\+[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*)?$ ]]; then
  echo "AIMS_CLI_VERSION must be SemVer" >&2
  exit 2
fi
bare_cli=${cli_version%%+*}
if [[ "$bare_cli" == *-* ]]; then
  prerelease=${bare_cli#*-}
  IFS='.' read -r -a identifiers <<< "$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^0[0-9]+$ ]]; then
      echo "Numeric SemVer prerelease identifiers cannot have leading zeroes" >&2
      exit 2
    fi
  done
fi
if [[ ! "$build_number" =~ ^[1-9][0-9]*$ ]]; then
  echo "AIMS_USAGE_BUILD_NUMBER must be a positive integer" >&2
  exit 2
fi
build=$(mktemp -d "${TMPDIR:-/tmp}/aims-usage-build.XXXXXX")
trap 'rm -rf "$build"' EXIT
export GOCACHE=${GOCACHE:-"$build/go-cache"}
app="$repo/dist/Aims Usage.app"
bundle_id=com.doguyilmaz.aims.usage
display_name='Aims Usage'
preview=false
if [[ "${AIMS_USAGE_PREVIEW:-0}" == 1 ]]; then
  app="$repo/dist/Aims Usage Preview.app"
  bundle_id=com.doguyilmaz.aims.usage.preview
  display_name='Aims Usage Preview'
  preview=true
fi
swift_args=(--package-path "$repo/macos/AimsUsage" --scratch-path "$build/swift" -c release)
if [[ "${AIMS_USAGE_UNIVERSAL:-0}" == 1 ]]; then
  swift_args+=(--arch arm64 --arch x86_64)
fi
if [[ "$preview" == true ]]; then
  swift_args+=(-Xswiftc -DAIMS_USAGE_PREVIEW)
fi
swift build "${swift_args[@]}"
bin=$(swift build "${swift_args[@]}" --show-bin-path)
mkdir -p "$build/Aims Usage.app/Contents/MacOS" "$build/Aims Usage.app/Contents/Resources"
mkdir -p "$build/Aims Usage.app/Contents/Frameworks"
ditto "$bin/Sparkle.framework" "$build/Aims Usage.app/Contents/Frameworks/Sparkle.framework"
cp "$repo/packaging/AppIcon.icns" "$build/Aims Usage.app/Contents/Resources/AppIcon.icns"
cp "$repo/LICENSE" "$build/Aims Usage.app/Contents/Resources/LICENSE"
cp -R "$repo/packaging/provider-marks" "$build/Aims Usage.app/Contents/Resources/provider-marks"
cp -R "$repo/packaging/sparkle" "$build/Aims Usage.app/Contents/Resources/sparkle"
cp "$bin/AimsUsage" "$build/Aims Usage.app/Contents/MacOS/AimsUsage"
if [[ -n "${AIMS_USAGE_CLI_BINARY:-}" ]]; then
  if [[ ! -f "$AIMS_USAGE_CLI_BINARY" || ! -x "$AIMS_USAGE_CLI_BINARY" ]]; then
    echo "AIMS_USAGE_CLI_BINARY must name a prebuilt executable" >&2
    exit 2
  fi
  cp "$AIMS_USAGE_CLI_BINARY" "$build/Aims Usage.app/Contents/Resources/aims"
  if [[ "${AIMS_USAGE_UNIVERSAL:-0}" == 1 ]]; then
    lipo "$build/Aims Usage.app/Contents/Resources/aims" -verify_arch arm64 x86_64
  fi
else
  (
    cd "$repo"
    if [[ "${AIMS_USAGE_UNIVERSAL:-0}" == 1 ]]; then
      for arch in arm64 amd64; do
        GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$cli_version" -o "$build/aims-$arch" ./cmd/aims
      done
      lipo -create "$build/aims-arm64" "$build/aims-amd64" -output "$build/Aims Usage.app/Contents/Resources/aims"
    else
      GOTOOLCHAIN=local CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$cli_version" -o "$build/Aims Usage.app/Contents/Resources/aims" ./cmd/aims
    fi
  )
fi
cat > "$build/Aims Usage.app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleIdentifier</key><string>$bundle_id</string>
  <key>CFBundleName</key><string>$display_name</string>
  <key>CFBundleDisplayName</key><string>$display_name</string>
  <key>CFBundleExecutable</key><string>AimsUsage</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$version</string>
  <key>CFBundleVersion</key><string>$build_number</string>
  <key>AimsSourceRevision</key><string>$source_revision</string>
  <key>LSMinimumSystemVersion</key><string>14.0.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
  <key>AimsUsagePreview</key><$preview/>
</dict></plist>
EOF
if [[ "$preview" == true ]]; then
  cp "$repo/macos/AimsUsage/Fixtures/usage.json" "$build/Aims Usage.app/Contents/Resources/usage-preview.json"
fi
python3 "$repo/scripts/configure-macos-updates.py" "$build/Aims Usage.app/Contents/Info.plist"
"$repo/scripts/sign-macos-app.sh" "$build/Aims Usage.app"
mkdir -p "$repo/dist"
# Refuse to replace an unrelated directory or a symbolic link.
if [[ -e "$app" || -L "$app" ]]; then
  if [[ -L "$app" || ! -f "$app/Contents/Info.plist" || $(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$app/Contents/Info.plist") != "$bundle_id" ]]; then
    echo "Refusing to replace an unrecognized output: $app" >&2
    exit 1
  fi
  rm -rf "$app"
fi
mv "$build/Aims Usage.app" "$app"
echo "$app"
