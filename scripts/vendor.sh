#!/bin/sh
# Vendor the front-end's CDN assets into web/vendor/ so the binary can serve them
# itself (ASSET_BASE=/vendor) — for networks with no egress to a CDN.
#
# Reads web/vendor.sha384: one "<pkg>@<ver>/<path>  sha384-<base64>" per line.
# Fetches each package's tarball straight from the npm registry (not the CDN —
# same bytes, one less party to trust), extracts only the listed files, and
# refuses any file whose digest doesn't match.
#
# Needs curl, tar and openssl. No Node.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
MANIFEST="$ROOT/web/vendor.sha384"
OUT="$ROOT/web/vendor"
REGISTRY=${NPM_REGISTRY:-https://registry.npmjs.org}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

[ -f "$MANIFEST" ] || { echo "vendor: missing $MANIFEST" >&2; exit 1; }

sri() { printf 'sha384-%s' "$(openssl dgst -sha384 -binary "$1" | openssl base64 -A)"; }

# tarball for pkg@ver, downloaded and unpacked once per run
fetch_pkg() {
	spec=$1 pkg=${1%@*} ver=${1##*@}
	[ -d "$TMP/$spec" ] || {
		echo "  ↓ $pkg@$ver"
		mkdir -p "$TMP/$spec"
		curl -fsSL --retry 3 --retry-delay 2 \
			"$REGISTRY/$pkg/-/$pkg-$ver.tgz" -o "$TMP/$spec.tgz"
		tar xzf "$TMP/$spec.tgz" -C "$TMP/$spec"
	}
}

count=0 failed=0
echo "vendor: reading web/vendor.sha384"
while read -r path want || [ -n "$path" ]; do
	case $path in ''|\#*) continue ;; esac
	[ -n "${want:-}" ] || { echo "vendor: no digest for $path" >&2; failed=1; continue; }

	spec=${path%%/*}   # pkg@ver
	inner=${path#*/}   # path inside the tarball, under package/
	fetch_pkg "$spec"

	src="$TMP/$spec/package/$inner"
	[ -f "$src" ] || { echo "  ✗ $path — not in the tarball" >&2; failed=1; continue; }

	got=$(sri "$src")
	if [ "$got" != "$want" ]; then
		echo "  ✗ $path — digest mismatch" >&2
		echo "      manifest: $want" >&2
		echo "      tarball:  $got" >&2
		failed=1
		continue
	fi

	mkdir -p "$OUT/$(dirname "$path")"
	cp "$src" "$OUT/$path"
	count=$((count + 1))
done <"$MANIFEST"

[ "$failed" -eq 0 ] || {
	echo "vendor: FAILED — nothing is trusted until every digest matches" >&2
	exit 1
}

# Prune whatever the manifest no longer pins. Without this, every version bump
# leaves the old tree behind — and it gets embedded into the binary forever.

if [ -d "$OUT" ]; then
	find "$OUT" -type f ! -name .gitkeep | while IFS= read -r f; do
		rel=${f#"$OUT"/}
		grep -q "^$rel " "$MANIFEST" || { rm -f "$f"; echo "  − $rel (no longer pinned)"; }
	done
	# drop directories the pruning emptied
	find "$OUT" -mindepth 1 -type d -empty -delete
fi

echo "vendor: $count files verified into web/vendor/"
echo "        rebuild to embed them (make build), then run with ASSET_BASE=/vendor"
