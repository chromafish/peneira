#!/bin/sh
# bundle.sh <binary> <version> — assemble build/Check.app around a binary.
#
# The bundle is what makes the binary behave like an application on macOS: its
# own Dock icon, a menu bar, and Retina rendering. The version is stamped into
# Info.plist so the Finder's Get Info agrees with what the binary reports.
set -eu

binary=${1:?usage: bundle.sh <binary> <version>}
version=${2:?usage: bundle.sh <binary> <version>}
bundle=build/Check.app

# Info.plist wants a bare number, so a v-prefixed tag loses its v.
plist_version=${version#v}

rm -rf "$bundle"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
cp "$binary" "$bundle/Contents/MacOS/Check"
cp packaging/Info.plist "$bundle/Contents/Info.plist"
cp packaging/Check.icns "$bundle/Contents/Resources/Check.icns"

if [ "$version" != "dev" ]; then
	/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $plist_version" "$bundle/Contents/Info.plist"
	/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $plist_version" "$bundle/Contents/Info.plist"
fi

echo "built $bundle ($version)"
