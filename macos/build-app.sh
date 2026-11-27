#!/bin/bash
# Builds dist/SyncAmp.app (universal, menu-bar only). Run on a Mac with Xcode 15+ / Swift 5.9+;
# Xcode 26 / Swift 6.2 enables the Liquid Glass look. No Apple developer account needed for local use.
set -euo pipefail
cd "$(dirname "$0")"
VERSION="${VERSION:-0.1.0}"
swift test
swift build -c release --arch arm64 --arch x86_64
APP="dist/SyncAmp.app"
rm -rf "$APP"; mkdir -p "$APP/Contents/MacOS"
cp ".build/apple/Products/Release/SyncAmpMenu" "$APP/Contents/MacOS/SyncAmpMenu"
# One download: bundle the background service (a universal `syncamp`) inside the app.
if [ -n "${SYNCAMP_BIN:-}" ]; then
  mkdir -p "$APP/Contents/Resources"
  cp "$SYNCAMP_BIN" "$APP/Contents/Resources/syncamp"
  chmod 755 "$APP/Contents/Resources/syncamp"
else
  echo "note: SYNCAMP_BIN not set, so the app will not be able to install the service by itself"
fi
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleIdentifier</key><string>app.syncamp.menu</string>
  <key>CFBundleName</key><string>SyncAmp</string>
  <key>CFBundleExecutable</key><string>SyncAmpMenu</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSAppTransportSecurity</key><dict><key>NSAllowsLocalNetworking</key><true/></dict>
</dict></plist>
PLIST
# Ad-hoc sign (enough to run locally; use a Developer ID + notarization to distribute).
codesign --force --deep --sign - "$APP"
echo "Built $APP  —  open it, or: cp -R $APP /Applications/"
