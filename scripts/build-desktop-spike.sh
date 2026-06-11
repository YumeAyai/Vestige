#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT="$ROOT/dist/desktop"
VERSION="${VESTIGE_VERSION:-desktop-$(date -u +%Y%m%d-%H%M%S)}"
GOOS_VALUE=$(go env GOOS)
GOARCH_VALUE=$(go env GOARCH)

mkdir -p "$OUT"

printf 'Building frontend assets...\n'
(cd "$ROOT/frontend" && npm run build)

if [ "$GOOS_VALUE" = "darwin" ]; then
	CGO_LDFLAGS="${CGO_LDFLAGS:-}"
	case " $CGO_LDFLAGS " in
		*" UniformTypeIdentifiers "*) ;;
		*) CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }-framework UniformTypeIdentifiers" ;;
	esac
	export CGO_LDFLAGS
fi

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"

out="$OUT/Jianji-$GOOS_VALUE-$GOARCH_VALUE"
printf 'Building desktop spike -> %s\n' "$out"
go build \
	-buildvcs=false \
	-tags "desktop,wv2runtime.download,production" \
	-trimpath \
	-ldflags="-s -w -X Vestige/pkg/config.BuildVersion=$VERSION" \
	-o "$out" \
	"$ROOT/backend/cmd/desktop"

printf 'Desktop spike binary: %s\n' "$out"
printf 'Version: %s\n' "$VERSION"
