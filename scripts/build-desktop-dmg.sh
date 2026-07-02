#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
APP_NAME="Jianji"
PRODUCT_NAME="见迹"
VERSION="${VESTIGE_VERSION:-0.1.0}"
GOOS_VALUE=$(go env GOOS)
GOARCH_VALUE=$(go env GOARCH)

if [ "$GOOS_VALUE" != "darwin" ]; then
	echo "DMG packaging must run on macOS." >&2
	exit 1
fi

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"

CGO_LDFLAGS="${CGO_LDFLAGS:-}"
case " $CGO_LDFLAGS " in
	*" UniformTypeIdentifiers "*) ;;
	*) CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }-framework UniformTypeIdentifiers" ;;
esac
export CGO_LDFLAGS

DIST="$ROOT/dist/desktop-dmg"
STAGE="$DIST/stage"
APP="$STAGE/$PRODUCT_NAME.app"
CONTENTS="$APP/Contents"
MACOS="$CONTENTS/MacOS"
RESOURCES="$CONTENTS/Resources"
DMG="$DIST/$APP_NAME-$GOOS_VALUE-$GOARCH_VALUE.dmg"

printf 'Building frontend assets...\n'
(cd "$ROOT/frontend" && npm run build)

printf 'Preparing app bundle...\n'
rm -rf "$STAGE"
mkdir -p "$MACOS" "$RESOURCES"

cat > "$CONTENTS/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>zh_CN</string>
	<key>CFBundleDisplayName</key>
	<string>$PRODUCT_NAME</string>
	<key>CFBundleExecutable</key>
	<string>$APP_NAME</string>
	<key>CFBundleIdentifier</key>
	<string>local.jianji.desktop</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>$PRODUCT_NAME</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
	<key>LSMinimumSystemVersion</key>
	<string>10.15</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
PLIST

printf 'Building desktop executable...\n'
go build \
	-buildvcs=false \
	-tags "desktop,wv2runtime.download,production" \
	-trimpath \
	-ldflags="-s -w -X Vestige/pkg/config.BuildVersion=$VERSION" \
	-o "$MACOS/$APP_NAME" \
	"$ROOT/backend/cmd/desktop"

chmod +x "$MACOS/$APP_NAME"

if command -v codesign >/dev/null 2>&1; then
	printf 'Ad-hoc signing app bundle...\n'
	codesign --force --deep --sign - "$APP"
fi

printf 'Creating DMG...\n'
rm -f "$DMG"
ln -s /Applications "$STAGE/Applications"
hdiutil create \
	-volname "$PRODUCT_NAME" \
	-srcfolder "$STAGE" \
	-ov \
	-format UDZO \
	"$DMG"

printf 'DMG: %s\n' "$DMG"
