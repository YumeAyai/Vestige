#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIST="$ROOT/dist"
APP_DIST="$DIST/app"
VERSION="${VESTIGE_VERSION:-app-$(date -u +%Y%m%d-%H%M%S)}"

mkdir -p "$APP_DIST"

printf 'Building frontend assets...\n'
(cd "$ROOT/frontend" && npm run build)

build_app() {
	os="$1"
	arch="$2"
	ext="$3"
	out="$APP_DIST/vestige-$os-$arch$ext"
	printf 'Building %s/%s -> %s\n' "$os" "$arch" "$out"
	GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}" \
	GOOS="$os" \
	GOARCH="$arch" \
	CGO_ENABLED=0 \
	go build -trimpath -ldflags="-s -w -X Vestige/pkg/config.BuildVersion=$VERSION" -o "$out" "$ROOT/backend/cmd/server"
}

build_app linux amd64 ""
build_app linux arm64 ""
build_app windows amd64 ".exe"
build_app darwin amd64 ""
build_app darwin arm64 ""

printf 'App dist: %s\n' "$APP_DIST"
printf 'Version: %s\n' "$VERSION"
