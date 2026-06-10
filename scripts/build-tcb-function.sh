#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR="$ROOT/functions/jianji"
OUT_BIN="$OUT_DIR/main"

mkdir -p "$OUT_DIR"

GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}" \
GOOS=linux \
GOARCH=amd64 \
CGO_ENABLED=0 \
go build -trimpath -ldflags="-s -w" -o "$OUT_BIN" "$ROOT/tracking-server/cmd/scf"

printf 'Built %s\n' "$OUT_BIN"
