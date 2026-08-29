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
mkdir -p /cache/npm

install_app() {
  app="$1"
  lockfile="/workspace/$app/package-lock.json"
  marker="/workspace/$app/node_modules/.notifuse-lock"
  if [ ! -f "$lockfile" ]; then
    echo "Missing lockfile: $lockfile" >&2
    exit 66
  fi
  digest="$(sha256sum "$lockfile" | awk '{print $1}')"
  if [ ! -f "$marker" ] || [ "$(cat "$marker")" != "$digest" ]; then
    echo "Installing $app dependencies for lockfile $digest"
    (cd "/workspace/$app" && npm ci --cache /cache/npm)
    printf '%s' "$digest" > "$marker"
  fi
}

install_app console
install_app notification_center

if [ "$DEV_HOT_RELOAD" = false ]; then
  echo "Building React applications in restart-only mode"
  (cd /workspace/console && npm run build)
  (cd /workspace/notification_center && npm run build)
  cp /etc/notifuse/nginx.static.conf /etc/nginx/http.d/default.conf
  exec nginx -g 'daemon off;'
fi

echo "Starting React applications with Vite hot reload"
cp /etc/notifuse/nginx.hot.conf /etc/nginx/http.d/default.conf

(cd /workspace/console && npm run dev -- --host 0.0.0.0 --port 5173 --strictPort) &
console_pid=$!
(cd /workspace/notification_center && npm run dev -- --host 0.0.0.0 --port 5174 --strictPort) &
notification_pid=$!
nginx -g 'daemon off;' &
nginx_pid=$!

shutdown() {
  trap - INT TERM
  kill "$nginx_pid" "$console_pid" "$notification_pid" 2>/dev/null || true
  wait "$nginx_pid" "$console_pid" "$notification_pid" 2>/dev/null || true
}

trap 'shutdown; exit 0' INT TERM

while :; do
  for process in "$nginx_pid" "$console_pid" "$notification_pid"; do
    if ! kill -0 "$process" 2>/dev/null; then
      echo "A frontend development process exited unexpectedly" >&2
      shutdown
      exit 1
    fi
  done
  sleep 1
done
