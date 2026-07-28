#!/usr/bin/env bash
# Renders the guide, serves it, and measures it in a real browser.
#
# Skips (0) rather than fails when the tooling is not installed: PinchTab and a browser are
# not dependencies of this product, and `make check` must stay runnable on a box with neither.
# It skips *honestly* now — the old version gated on a hardcoded
# /opt/node22/lib/node_modules/playwright/index.mjs, which existed on one machine, so
# everywhere else this check skipped in a way that reads exactly like a pass.
#
# The browser instance is started here, on its own port, and stopped on the way out. That is
# what makes the run repeatable: PinchTab commands act on an instance's current tab, so an
# editor or an MCP integration sharing the default instance navigates the tab out from under a
# measurement. Measured before this was added: 2 of 3 runs failed on a shared instance, 0 of 3
# on a dedicated one.
set -euo pipefail
cd "$(dirname "$0")/.."

PT=$(command -v pinchtab || true)
if ! command -v node >/dev/null 2>&1 || [ -z "$PT" ]; then
  echo "  skipped check-ui (needs node + pinchtab on PATH — npm i -g pinchtab)"
  exit 0
fi
# `pinchtab doctor` is the only thing that can answer whether a browser is actually reachable,
# so its verdict is the gate rather than a guess about install paths.
if ! "$PT" doctor >/dev/null 2>&1; then
  echo "  skipped check-ui (pinchtab has no browser — run \`pinchtab doctor\`)"
  exit 0
fi
if [ -z "$(ls -A web/vendor 2>/dev/null)" ]; then
  make --no-print-directory vendor >/dev/null
fi

port=${PORT_UI:-8123}
ptport=${PINCHTAB_PORT:-9871}
dir=$(mktemp -d)
cleanup() {
  rm -rf "$dir"
  [ -n "${srv:-}" ] && kill "$srv" 2>/dev/null || true
  [ -n "${inst:-}" ] && "$PT" instance stop "$inst" >/dev/null 2>&1 || true
}
trap cleanup EXIT

go run ./cmd/rendocs -d "$dir" -base /vendor >/dev/null
cp -r web/vendor "$dir/vendor"
cp web/*.svg "$dir/"

python3 -m http.server "$port" -d "$dir" >/dev/null 2>&1 &
srv=$!
for _ in $(seq 1 40); do
  curl -sf -o /dev/null "http://127.0.0.1:$port/index.html" && break
  sleep 0.25
done

inst=$("$PT" instance start --port "$ptport" --mode headless 2>/dev/null \
  | sed -n 's/.*"id": *"\([^"]*\)".*/\1/p' | head -1)
if [ -z "$inst" ]; then
  echo "  skipped check-ui (pinchtab could not start an instance on :$ptport)"
  exit 0
fi

PINCHTAB_BIN="$PT" PINCHTAB_SERVER="http://127.0.0.1:$ptport" \
  node scripts/check-docs-ui.mjs "http://127.0.0.1:$port"
