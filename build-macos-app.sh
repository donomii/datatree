#!/bin/sh
set -eu

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR"

GO_BIN="/opt/homebrew/bin/go"
if [ ! -f "$GO_BIN" ]; then
    GO_BIN="go"
fi

APP_DIR="$PROJECT_DIR/Datatree.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

echo "Building Datatree Go binary..."
CGO_ENABLED=0 "$GO_BIN" build -o "$PROJECT_DIR/datatree" .

mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"
cp "$PROJECT_DIR/datatree" "$MACOS_DIR/datatree-bin.new"
chmod 755 "$MACOS_DIR/datatree-bin.new"
mv -f "$MACOS_DIR/datatree-bin.new" "$MACOS_DIR/datatree-bin"

echo "Compiling macOS launcher..."
clang -O2 -fobjc-arc -framework Cocoa -o "$MACOS_DIR/Datatree.new" "$PROJECT_DIR/macos/DatatreeLauncher.m"
chmod 755 "$MACOS_DIR/Datatree.new"
mv -f "$MACOS_DIR/Datatree.new" "$MACOS_DIR/Datatree"

echo "Generating macOS app icon..."
"$GO_BIN" run ./macos/makeicon -out "$RESOURCES_DIR/AppIcon.icns.new"
mv -f "$RESOURCES_DIR/AppIcon.icns.new" "$RESOURCES_DIR/AppIcon.icns"

cp "$PROJECT_DIR/macos/Info.plist" "$CONTENTS_DIR/Info.plist.new"
plutil -lint "$CONTENTS_DIR/Info.plist.new"
mv -f "$CONTENTS_DIR/Info.plist.new" "$CONTENTS_DIR/Info.plist"

echo "Signing Datatree.app (ad-hoc)..."
codesign --force --deep --sign - "$APP_DIR"

echo "Built macOS app bundle: $APP_DIR"
echo "Open with: open \"$APP_DIR\""
