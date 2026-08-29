#!/bin/sh
set -eu

case "${DEV_HOT_RELOAD:-true}" in
  true|false) ;;
  *)
    echo "DEV_HOT_RELOAD must be true or false" >&2
    exit 64
    ;;
esac

cd /workspace
if [ ! -f go.mod ] || [ ! -f go.sum ]; then
  echo "Go source must be mounted at /workspace" >&2
  exit 66
fi

export GOCACHE="${GOCACHE:-/cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-/cache/go-mod}"
export GOTMPDIR="${GOTMPDIR:-/cache/go-tmp}"
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR" /cache/notifuse-air /cache/notifuse-bin

digest="$(sha256sum go.sum | awk '{print $1}')"
marker="$GOMODCACHE/.notifuse-go-sum"
if [ ! -f "$marker" ] || [ "$(cat "$marker")" != "$digest" ]; then
  echo "Downloading Go modules for go.sum $digest"
  go mod download
  printf '%s' "$digest" > "$marker"
fi

if [ "$DEV_HOT_RELOAD" = true ]; then
  echo "Starting Yaoguang Marketing backend with Go hot reload"
  exec air -c .air.toml
fi

echo "Starting Yaoguang Marketing backend in restart-only mode"
exec go run ./cmd/api
