#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIST="$ROOT/dist"
OUT_BIN="$DIST/main"
BOOTSTRAP="$DIST/scf_bootstrap"
ZIP="$DIST/tracker.zip"
VERSION="${TRACKER_VERSION:-event-$(date -u +%Y%m%d-%H%M%S)}"

mkdir -p "$DIST"

GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}" \
GOOS=linux \
GOARCH=amd64 \
CGO_ENABLED=0 \
go build -trimpath -ldflags="-s -w -X Vestige/tracker/internal/handler.serviceVersion=$VERSION" -o "$OUT_BIN" "$ROOT/tracker/cmd/scf"

cat > "$BOOTSTRAP" <<'EOF'
#!/bin/bash
set -eu
export PORT=9000
export GIN_MODE=release
exec ./main
EOF
chmod +x "$BOOTSTRAP"

rm -f "$ZIP"
(cd "$DIST" && zip -q tracker.zip main scf_bootstrap)

printf 'Built %s\n' "$OUT_BIN"
printf 'Packed %s\n' "$ZIP"
printf 'Version %s\n' "$VERSION"
