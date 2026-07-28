#!/usr/bin/env bash
# Renders the guide, serves it, and drives every diagram walkthrough in a real browser.
#
# Same shape as check-docs-ui.sh, and the same reasoning — see its header for why the browser
# instance is started here on its own port rather than shared. Different default ports so the
# two checks can run back to back (or at once) without fighting over either one.
set -euo pipefail
cd "$(dirname "$0")/.."

PT=$(command -v pinchtab || true)
if ! command -v node >/dev/null 2>&1 || [ -z "$PT" ]; then
  echo "  skipped check-walkthroughs (needs node + pinchtab on PATH — npm i -g pinchtab)"
  exit 0
fi
if ! "$PT" doctor >/dev/null 2>&1; then
  echo "  skipped check-walkthroughs (pinchtab has no browser — run \`pinchtab doctor\`)"
  exit 0
fi
if [ -z "$(ls -A web/vendor 2>/dev/null)" ]; then
  make --no-print-directory vendor >/dev/null
fi

port=${PORT_WT:-8125}
ptport=${PINCHTAB_PORT_WT:-9875}
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

# See check-docs-ui.sh: free the port first, and treat a failure to start as a failure rather
# than a skip. A missing browser is a skip; a busy port is a broken run.
"$PT" --server "http://127.0.0.1:$ptport" instance stop >/dev/null 2>&1 || true
inst=$("$PT" instance start --port "$ptport" --mode headless 2>/dev/null \
  | sed -n 's/.*"id": *"\([^"]*\)".*/\1/p' | head -1)
if [ -z "$inst" ]; then
  echo "  FAILED check-walkthroughs: pinchtab could not start an instance on :$ptport." >&2
  echo "  Set PINCHTAB_PORT_WT to a free port, or stop what is on that one." >&2
  exit 1
fi

PINCHTAB_BIN="$PT" PINCHTAB_SERVER="http://127.0.0.1:$ptport" \
  node scripts/check-walkthroughs.mjs "http://127.0.0.1:$port"
