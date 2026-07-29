#!/usr/bin/env bash
# One copy of the corpus a day, off this machine's disk, taken while the service keeps
# answering. `make backup` runs it by hand; a systemd timer runs it nightly (Deploy page).
#
# Three things here are not obvious, and each is a failure this shape avoids:
#
#   sqlite3 .backup, not cp.  With WAL on, the file on disk is missing whatever is still in
#   knowledge.db-wal — 2.4 MB of it on this host — so a `cp` taken under load produces a
#   database that opens cleanly and is quietly short of the last writes. `.backup` is
#   SQLite's online backup API: a consistent snapshot with the server still running, which
#   is why nothing here stops the unit.
#
#   Verify, then publish.  The snapshot lands next to the database, gets an
#   integrity_check, and only then is copied to DEST. A truncated file sitting at DEST
#   under today's name is worse than no backup at all, because it reads like one.
#
#   DEST is outside this machine's disk.  On WSL2 the distro's filesystem *is* a .vhdx file
#   inside Windows, so a copy beside the database shares its single point of failure — and
#   losing the database loses the corpus (invariant 1). The default is the Windows
#   filesystem for that reason. `Users/Public` because it exists on every Windows install
#   and needs no administrator to write to; C:\ itself refuses.
#
# Restoring is a `cp` back and a restart — the file is a plain database, not an archive,
# which is why nothing here compresses it.
set -euo pipefail
cd "$(dirname "$0")/.."

command -v sqlite3 >/dev/null || {
	echo "  backup: sqlite3 is not installed (apt install sqlite3)" >&2; exit 1; }

# .env is what the server reads, so it is where the path comes from — a database path
# written twice is a backup that verifies against a file nothing serves. The fallback is
# internal/config's own default.
DB=$(sed -n 's/^DB_PATH=//p' .env 2>/dev/null | tail -1)
DB=${DB:-knowledge.db}
DEST=${DEST:-/mnt/c/Users/Public/knowledge-backups}
KEEP=${KEEP:-7}

[ -f "$DB" ] || { echo "  backup: no database at $DB" >&2; exit 1; }

# Refuse rather than create the tree: DEST's parent missing means this is not the machine
# the default was written for (a Mac has no /mnt/c), and a backup written somewhere nobody
# looks is the failure this script exists to prevent.
[ -d "$(dirname "$DEST")" ] || {
	echo "  backup: $(dirname "$DEST") does not exist — set DEST to a directory outside this machine's disk" >&2
	exit 1; }
mkdir -p "$DEST"

stamp=$(date +%F)
snapshot="$DB.snapshot"
trap 'rm -f "$snapshot"' EXIT

sqlite3 "$DB" ".backup '$snapshot'"
check=$(sqlite3 "$snapshot" 'PRAGMA integrity_check;')
[ "$check" = ok ] || { echo "  backup: the snapshot failed integrity_check: $check" >&2; exit 1; }

cp "$snapshot" "$DEST/knowledge-$stamp.db"

# Retention by count and not by age: `find -mtime +7` empties the directory after a week
# the machine was off, which is exactly when the backups matter. Newest kept.
ls -1t "$DEST"/knowledge-*.db | tail -n "+$((KEEP + 1))" | xargs -r rm --

echo "  backed up $DB → $DEST/knowledge-$stamp.db ($(du -h "$DEST/knowledge-$stamp.db" | cut -f1)), keeping $KEEP"
