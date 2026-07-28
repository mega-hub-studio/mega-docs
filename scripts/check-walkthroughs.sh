#!/usr/bin/env bash
# Renders the guide, serves it, and measures it in a real browser.
# Skips (0) rather than fails when the tooling is not installed: Playwright and a
# browser are not dependencies of this product, and `make check` must stay runnable on
# a box with neither.
set -euo pipefail
cd "$(dirname "$0")/.."

PW=${PLAYWRIGHT_PATH:-/opt/node22/lib/node_modules/playwright/index.mjs}
if ! command -v node >/dev/null 2>&1 || [ ! -f "$PW" ]; then
  echo "  skipped check-walkthroughs (needs node + playwright at $PW)"
  exit 0
fi
if [ ! -d web/vendor/8bit-nes* ] 2>/dev/null; then
  make --no-print-directory vendor >/dev/null
fi

dir=$(mktemp -d)
trap 'rm -rf "$dir"; [ -n "${srv:-}" ] && kill "$srv" 2>/dev/null || true' EXIT

go run ./cmd/rendocs -d "$dir" -base /vendor >/dev/null
cp -r web/vendor "$dir/vendor"
cp web/*.svg "$dir/"

port=${PORT_UI:-8123}
python3 -m http.server "$port" -d "$dir" >/dev/null 2>&1 &
srv=$!
for _ in $(seq 1 40); do
  curl -sf -o /dev/null "http://127.0.0.1:$port/index.html" && break
  sleep 0.25
done

node scripts/check-walkthroughs.mjs "http://127.0.0.1:$port"
