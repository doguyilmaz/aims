#!/bin/sh
# macapp.sh OS VERSION BINARY OUT: wraps a macOS aims binary in OUT/aims.app, so
# the login keepalive shows up under the app's name and icon in macOS's
# Login Items. The tapster hub signs and notarizes it.
set -eu
[ "$1" = darwin ] || exit 0
version=${2#v}
version=${version%%-*}
app=$4/aims.app
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$3" "$app/Contents/MacOS/aims"
chmod 755 "$app/Contents/MacOS/aims"
cp packaging/AppIcon.icns "$app/Contents/Resources/AppIcon.icns"
sed "s/__VERSION__/$version/g" packaging/Info.plist >"$app/Contents/Info.plist"
