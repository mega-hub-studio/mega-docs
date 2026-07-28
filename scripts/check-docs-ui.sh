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
# There was a second guard here, `pinchtab doctor`, meant to turn "installed but no browser"
# into a skip. It never fired, in either direction: 0.13.2 has no `doctor` — the command is
# `health` — and **no pinchtab command can be gated on its exit code anyway**. Measured:
# `pinchtab bogus-subcommand` exits 0, and `health` against a refused connection exits 0
# while printing the refusal. So the guard passed on every machine, including the ones it
# was written to skip.
#
# Deleted rather than rewritten against `health`, because the only honest detector is the
# thing this script already does below: start an instance and see whether an id comes back.
# A box with no pinchtab at all still skips, at the guard above.
if [ -z "$(ls -A web/vendor 2>/dev/null)" ]; then
  make --no-print-directory vendor >/dev/null
fi

port=${PORT_UI:-8123}
ptport=${PINCHTAB_PORT:-9871}
dir=$(mktemp -d)
# `wait` after the kill, both silenced together: bash notices a SIGTERMed background job on
# its own and prints `line 36: 12018 Terminated: 15 python3 -m http.server …` — after
# `DOCS: PASS`, on stderr, as the last line of a green run. Reaping it here is what stops the
# notice, and it only shows up when something follows the kill (stopping the browser does),
# which is why it reads as a failure on this machine and not on the one it was written on.
cleanup() {
  rm -rf "$dir"
  [ -n "${srv:-}" ] && { kill "$srv"; wait "$srv"; } 2>/dev/null || true
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

# Free the port first. An instance left behind by an interrupted run still holds it, and
# `instance start` then fails — which used to mean this check *skipped*, with a reason nobody
# reads, which is the exact failure mode the whole rewrite was about.
"$PT" --server "http://127.0.0.1:$ptport" instance stop >/dev/null 2>&1 || true
inst=$("$PT" instance start --port "$ptport" --mode headless 2>/dev/null \
  | sed -n 's/.*"id": *"\([^"]*\)".*/\1/p' | head -1)
# This is the check's only real detector — see the deleted `doctor` guard above — so it names
# both reasons an id can fail to come back, not just the one that happened first here.
if [ -z "$inst" ]; then
  echo "  FAILED check-ui: pinchtab started no instance on :$ptport." >&2
  echo "  Either it has no browser (\`pinchtab health\`, \`pinchtab instances\`)," >&2
  echo "  or that port is taken — set PINCHTAB_PORT to a free one." >&2
  exit 1
fi

PINCHTAB_BIN="$PT" PINCHTAB_SERVER="http://127.0.0.1:$ptport" \
  node scripts/check-docs-ui.mjs "http://127.0.0.1:$port"
