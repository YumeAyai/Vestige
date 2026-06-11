#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT="$ROOT/dist/desktop-exe"
VERSION="${VESTIGE_VERSION:-0.1.0}"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"

mkdir -p "$OUT"

printf 'Building frontend assets...\n'
(cd "$ROOT/frontend" && npm run build)

printf 'Building Windows desktop exe...\n'
(cd "$ROOT/backend/cmd/desktop" && \
	"$HOME/go/bin/wails" build \
		-platform windows/amd64 \
		-s \
		-m \
		-skipbindings \
		-skipembedcreate \
		-nopackage \
		-ldflags "-X Vestige/pkg/config.BuildVersion=$VERSION")

cp "$ROOT/backend/cmd/desktop/build/bin/Jianji.exe" "$OUT/Jianji-windows-amd64.exe"

printf 'EXE: %s\n' "$OUT/Jianji-windows-amd64.exe"
