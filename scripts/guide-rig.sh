# The rig both browser checks run on: render the guide, serve it, put a browser in front of
# it, and clean up after. Sourced, never executed — `check-docs-ui.sh` and
# `check-walkthroughs.sh` set four variables, source this, and are then three lines each.
#
# It exists because it was already written twice. Every fix in this file had to be applied to
# both copies — the reaped `kill`, the deleted `doctor` guard, the readiness wait, the stale
# instance, the error message that carries pinchtab's own words — five times over one session,
# which is rule 17's drift arriving on schedule rather than in theory.
#
# The caller sets, before sourcing:
#   rig_name     what to call this check in a message  (check-ui)
#   port         the http port for the rendered guide  (8123)
#   ptport       the port for this run's browser       (9871)
#   rig_portvar  the env var that moves ptport         (PINCHTAB_PORT)
#
# It leaves behind: PT, $dir (rendered and served on $port), $inst (a browser, running),
# PINCHTAB_BIN and PINCHTAB_SERVER exported for the node script, and an EXIT trap that stops
# all of it.
#
# Skips (0) rather than fails when the tooling is not installed: PinchTab and a browser are
# not dependencies of this product, and `make check` must stay runnable on a box with neither.
# It skips *honestly* — the version before this gated on a hardcoded
# /opt/node22/lib/node_modules/playwright/index.mjs, which existed on one machine, so
# everywhere else the checks skipped in a way that reads exactly like a pass.

PT=$(command -v pinchtab || true)
if ! command -v node >/dev/null 2>&1 || [ -z "$PT" ]; then
  echo "  skipped $rig_name (needs node + pinchtab on PATH — npm i -g pinchtab)"
  exit 0
fi
# There was a second guard here, `pinchtab doctor`, meant to turn "installed but no browser"
# into a skip. It never fired, in either direction: 0.13.2 has no `doctor` — the command is
# `health` — and **no pinchtab command can be gated on its exit code anyway**. Measured:
# `pinchtab bogus-subcommand` exits 0, and `health` against a refused connection exits 0 while
# printing the refusal. So the guard passed on every machine, including the ones it was
# written to skip.
#
# Deleted rather than rewritten against `health`, because the only honest detector is what
# this file already does below: start an instance and see whether an id comes back.
if [ -z "$(ls -A web/vendor 2>/dev/null)" ]; then
  make --no-print-directory vendor >/dev/null
fi

dir=$(mktemp -d)
# `wait` after the kill, both silenced together: bash notices a SIGTERMed background job on its
# own and prints `line 36: 12018 Terminated: 15 python3 -m http.server …` — after the verdict,
# on stderr, as the last line of a green run. Reaping it here is what stops the notice, and it
# only shows up when something follows the kill (stopping the browser does), which is why it
# appears under macOS's bash 3.2 and not on the box this was written on.
#
# The browser stop is checked rather than fired and forgotten. One `instance stop` issued the
# moment the driver exits reports success and leaves the instance `running` — reproduced with
# `bash -x`, then stopped by hand a second later with the same command and the same id. So
# every run leaked a headless browser, and the next one met `409 port already reserved`, which
# is the failure the `stale` sweep below had to be written for. Both stay: the sweep is the
# backstop for a run that was interrupted before its trap, this is the one that keeps a normal
# run from needing it.
cleanup() {
  rm -rf "$dir"
  [ -n "${srv:-}" ] && { kill "$srv"; wait "$srv"; } 2>/dev/null || true
  for _ in $(seq 1 6); do
    [ -n "${inst:-}" ] || break
    "$PT" instance stop "$inst" >/dev/null 2>&1 || true
    "$PT" instances 2>/dev/null | grep -q "^$inst" || break
    sleep 0.5
  done
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

# Free the port first: an instance left behind by an interrupted run still holds it, and
# `instance start` then answers `409 port already reserved`.
#
# This used to be `instance stop` with `--server` naming the port and no id — which the CLI
# answers `Error: accepts 1 arg(s), received 0`, silenced by the `2>/dev/null` next to it. So
# the line that existed to prevent the 409 had never once run, and the 409 it was written for
# is exactly what turned up the day the failure message started quoting pinchtab. `instances`
# is where the id lives; stopping by id works.
stale=$("$PT" instances 2>/dev/null | awk -v p="$ptport" '$2 == p { print $1 }')
[ -n "$stale" ] && "$PT" instance stop "$stale" >/dev/null 2>&1 || true

# stderr kept, not `2>/dev/null`, because this is the check's only real detector — see the
# deleted `doctor` guard above — and the guess it printed in place of pinchtab's own words
# sent one debugging session after a port that was free the whole time.
started=$("$PT" instance start --port "$ptport" --mode headless 2>&1)
inst=$(printf '%s' "$started" | sed -n 's/.*"id": *"\([^"]*\)".*/\1/p' | head -1)
if [ -z "$inst" ]; then
  echo "  FAILED $rig_name: pinchtab started no instance on :$ptport — it said:" >&2
  printf '%s\n' "$started" | sed 's/^/    /' >&2
  echo "  A browser it cannot reach and a port already taken both land here" >&2
  echo "  (\`pinchtab instances\`); $rig_portvar moves this check to a free one." >&2
  exit 1
fi

# `instance start` answers `"status": "starting"` and returns — the browser is not up yet. The
# driver's boot navigation therefore raced it and lost about as often as not: one
# `Error 500: navigate: context canceled` on stderr per run, retried by open() and green
# afterwards, which is the worst kind of line to leave in a gate's output. `instances` prints
# the same id as `running` once it is, so this waits for the fact instead of retrying past it.
for _ in $(seq 1 60); do
  "$PT" instances 2>/dev/null | grep -q "^$inst.*running" && break
  sleep 0.25
done

# Nothing is exported from here, and PINCHTAB_SERVER least of all: the caller puts it on the
# node command line as a prefix, so it reaches the driver and nothing else. In the environment
# it retargets *every* pinchtab call, and an instance's own server does not serve the instance
# API — the EXIT trap's `instance stop` answers `404 page not found`, says so into /dev/null,
# and leaks a headless browser per run, which the next run meets as `409 port already
# reserved`. One exported convenience, one trap broken eighty lines away.
