#!/bin/sh
# End-to-end check against a real provider: ingest a document, ask a question
# about a fact only that document contains, and verify the answer streams and
# cites it. This is the one test that proves the whole product works — retrieval,
# prompting, SSE, citations — rather than any single layer.
#
#   make smoke                                   # reads .env
#   PORT=9001 make smoke                         # if 8123 is taken
#
# Needs AI_API_KEY (in .env or the environment) plus curl. Uses a throwaway
# database and a throwaway docs directory; touches nothing of yours.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PORT=${PORT:-8123}
TMP=$(mktemp -d)
DB="$TMP/smoke.db"
LOG="$TMP/server.log"
SERVER_PID=""

cleanup() {
	[ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
	rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
fail() {
	printf '\n\033[31m✗ %s\033[0m\n' "$*" >&2
	[ -s "$LOG" ] && { echo "--- server log ---" >&2; cat "$LOG" >&2; }
	exit 1
}

# The provider key is read by the binary itself from .env, so it never appears on
# a command line (and never in this script's output).
if [ ! -f "$ROOT/.env" ] && [ -z "${AI_API_KEY:-}" ]; then
	fail "no AI_API_KEY: copy .env.example to .env and fill it in"
fi

# ── a document with a fact that exists nowhere else ─────────────────────────────
# If the answer contains it, retrieval genuinely fed the model this file.
MARKER="Zylkanite-7"
mkdir -p "$TMP/docs"
cat >"$TMP/docs/smoke-fixture.md" <<EOF
# Smoke Fixture

## Cache eviction policy
The knowledge engine evicts stale embeddings using the ${MARKER} strategy, which
retires a chunk after nine failed retrievals. ${MARKER} applies only to chunks
whose status is still draft.

## Unrelated section
Deployment ships a single Go binary with the frontend embedded.
EOF

say "1/5  building"
cd "$ROOT"
make build >"$TMP/build.log" 2>&1 || { cat "$TMP/build.log" >&2; fail "build failed"; }

say "2/5  ingesting the fixture (this calls /embeddings)"
# Not piped: a pipeline's status is the *last* command's, so `ingest | sed` would
# report sed's success and sail past a failed ingest.
if DB_PATH="$DB" ./bin/ingest "$TMP/docs" >"$TMP/ingest.log" 2>&1; then
	sed 's/^/      /' "$TMP/ingest.log"
else
	sed 's/^/      /' "$TMP/ingest.log" >&2
	fail "ingest failed — if it reported no /embeddings endpoint, set EMBED_BASE_URL (see .env.example)"
fi

say "3/5  starting the server on :$PORT"
DB_PATH="$DB" PORT="$PORT" ./bin/knowledge >"$LOG" 2>&1 &
SERVER_PID=$!
i=0
until curl -fsS --noproxy '*' "http://localhost:$PORT/api/health" >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -gt 40 ] && fail "server did not come up"
	kill -0 "$SERVER_PID" 2>/dev/null || fail "server exited"
	sleep 0.25
done
printf '      health ok\n'

say "4/5  checking the corpus is visible"
CORPUS=$(curl -fsS --noproxy '*' "http://localhost:$PORT/api/corpus")
echo "      $CORPUS" | cut -c1-160
echo "$CORPUS" | grep -q '"docs":[1-9]' || fail "corpus reports no documents — did ingest write to $DB?"
echo "$CORPUS" | grep -q 'smoke-fixture.md' || fail "the fixture is not listed in the corpus"

say "5/5  asking a question only that document can answer"
OUT="$TMP/answer.sse"
curl -fsS --noproxy '*' -N \
	-H 'Content-Type: application/json' \
	-d '{"question":"What is the cache eviction strategy and when does it retire a chunk?"}' \
	"http://localhost:$PORT/api/chat" >"$OUT" || fail "/api/chat request failed"

grep -q '^event: token' "$OUT" || fail "no token frames — the model streamed nothing"
grep -q '^event: citations' "$OUT" || fail "no citations frame"
grep -q 'smoke-fixture.md' "$OUT" || fail "the answer cited no source from the fixture"
grep -q '^event: done' "$OUT" || fail "the stream never completed (an error event instead?)"

TOKENS=$(grep -c '^event: token' "$OUT")
ANSWER=$(grep '^data: {"t":' "$OUT" | sed 's/^data: {"t":"//; s/"}$//' | tr -d '\n')

printf '      %s token frames\n' "$TOKENS"
printf '      answer: %s\n' "$(printf '%s' "$ANSWER" | cut -c1-220)"

# Grounding check: the marker exists only in the fixture, so quoting it back means
# the retrieved context actually reached the model.
if printf '%s' "$ANSWER" | grep -q "$MARKER"; then
	printf '\n\033[32m✓ grounded: the answer quotes a fact unique to the ingested document\033[0m\n'
else
	printf '\n\033[33m! the answer did not mention %s\033[0m\n' "$MARKER"
	printf '  Retrieval and citations worked, so the wiring is fine — the model just\n'
	printf '  paraphrased. Re-run, or try a stronger CHAT_MODEL if it keeps happening.\n'
fi

printf '\n\033[32m✓ smoke passed: ingest → retrieve → stream → cite\033[0m\n'
