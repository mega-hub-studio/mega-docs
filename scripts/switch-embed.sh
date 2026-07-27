#!/bin/sh
# Move embeddings to a different provider, safely.
#
#   make switch-embed                                  # this repo, no service
#   DIR=/opt/knowledge SERVICE=knowledge make switch-embed
#
# Edit the target .env first — EMBED_BASE_URL / EMBED_MODEL / EMBED_DIM are the
# source of truth, and this script only does the two things that file cannot: put
# the secret in without it reaching your shell history, and rebuild the index,
# because vectors from two models are not comparable.
#
# The key is validated against the new endpoint *before* the index is dropped: a
# typo must not cost you a working database. Needs curl. DIR defaults to the repo,
# so it is the same command in dev and on a host; SERVICE is optional and only
# stops/starts systemd when set.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIR=${DIR:-$ROOT}
SERVICE=${SERVICE:-}
ENV_FILE="$DIR/.env"

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
fail() {
	printf '\n\033[31m✗ %s\033[0m\n' "$*" >&2
	exit 1
}

# Same rules as config.loadDotEnv: KEY=VALUE, quotes stripped, comments ignored.
envval() { sed -n "s/^$1=//p" "$ENV_FILE" | tail -1 | sed 's/^["'\'']//; s/["'\'']$//'; }

[ -f "$ENV_FILE" ] || fail "no $ENV_FILE — copy .env.example to .env and set the embedding provider first"

EMBED_URL=$(envval EMBED_BASE_URL)
EMBED_MODEL=$(envval EMBED_MODEL)
EMBED_DIM=$(envval EMBED_DIM)
DB=$(envval DB_PATH)
DB=${DB:-knowledge.db}
DOCS=${DOCS:-$DIR/docs}
INGEST="$DIR/bin/ingest"

[ -n "$EMBED_URL" ] || fail "EMBED_BASE_URL is empty in $ENV_FILE — embeddings share the chat provider, so there is no separate key to set"
[ -x "$INGEST" ] || fail "no $INGEST — run make build (and copy bin/ to $DIR) first"
[ -d "$DOCS" ] || fail "no $DOCS — set DOCS=<dir> to point at the corpus"

printf 'API key for %s: ' "$EMBED_URL"
stty -echo 2>/dev/null || true
read -r KEY
stty echo 2>/dev/null || true
printf '\n'
[ -n "$KEY" ] || fail "no key entered — nothing changed"

say "1/5  checking the key against $EMBED_URL/embeddings"
# Not one pipeline: a pipeline's status is the *last* command's, so piping curl
# straight into the counter would report wc's success and blame a rejected key on
# a zero-width vector.
RESP=$(curl -fsS -m 30 "$EMBED_URL/embeddings" \
	-H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
	-d "{\"model\":\"$EMBED_MODEL\",\"input\":\"ping\"}") ||
	fail "the endpoint rejected the key (or is unreachable) — nothing changed"
case $RESP in
*'"embedding"'*) ;;
*) fail "no embedding in the response — is $EMBED_MODEL served by $EMBED_URL? nothing changed" ;;
esac
# Width, counted without a JSON parser, so this script needs nothing but curl.
# Commas + 1, not `tr ',' '\n' | wc -l`: a response body has no trailing newline
# and sed does not add one, so counting lines is short by exactly one.
COMMAS=$(printf '%s' "$RESP" | sed 's/.*"embedding":\[//; s/\].*//' | tr -cd ',' | wc -c | tr -d ' ')
WIDTH=$((COMMAS + 1))
[ "$WIDTH" = "$EMBED_DIM" ] ||
	fail "$EMBED_MODEL returned ${WIDTH}-dim vectors but EMBED_DIM says $EMBED_DIM — fix $ENV_FILE; nothing changed"
printf '      ok, %s-dim\n' "$WIDTH"

say "2/5  writing EMBED_API_KEY into $ENV_FILE"
# Written through a temp file at mode 600, so a partial write cannot land and the
# key is never briefly world-readable.
TMP=$(mktemp)
chmod 600 "$TMP"
sed "s|^EMBED_API_KEY=.*|EMBED_API_KEY=$KEY|" "$ENV_FILE" >"$TMP"
grep -q '^EMBED_API_KEY=' "$TMP" || printf 'EMBED_API_KEY=%s\n' "$KEY" >>"$TMP"
mv "$TMP" "$ENV_FILE"
chmod 600 "$ENV_FILE"

say "3/5  dropping the old index ($DIR/$DB)"
[ -n "$SERVICE" ] && sudo systemctl stop "$SERVICE"
(cd "$DIR" && rm -f "$DB" "$DB-wal" "$DB-shm")

say "4/5  re-ingesting $DOCS at $EMBED_DIM dim"
# Run from DIR so .env and DB_PATH resolve the way the service sees them. The stored
# document path needs no massaging here: ingest stores it relative to CORPUS_DIR, so
# an absolute argument produces the same identity as a relative one (cmd/ingest.docPath).
(cd "$DIR" && "$INGEST" "$DOCS") || fail "ingest failed — the index is now empty; fix and re-run $INGEST $DOCS"

if [ -z "$SERVICE" ]; then
	printf '\n\033[32m✓ embeddings now %s (%s) — start the server when you are ready\033[0m\n' "$EMBED_MODEL" "$EMBED_URL"
	exit 0
fi

say "5/5  restarting $SERVICE"
sudo systemctl start "$SERVICE"
PORT=$(envval PORT)
PORT=${PORT:-8080}
i=0
until curl -fsS --noproxy '*' "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -gt 40 ] && fail "$SERVICE did not come up — journalctl -u $SERVICE -n 30"
	sleep 0.25
done
curl -fsS --noproxy '*' "http://127.0.0.1:$PORT/api/corpus"
printf '\n\033[32m✓ embeddings now %s (%s)\033[0m\n' "$EMBED_MODEL" "$EMBED_URL"
