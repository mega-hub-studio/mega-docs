#!/bin/sh
# Commit whatever landed in the corpus, and push it if there is a remote.
#
#   DIR=/opt/knowledge/docs sh scripts/corpus-sync.sh
#
# Two things write into the corpus while the app runs — a BA confirming an answer
# and someone importing a document — and both are just files. This turns them into
# history, so `git diff` is the review trail the deploy page promises and the folder
# is a real backup rather than a directory nobody has a copy of.
#
# Run it from a systemd .path unit (see the Deploy page) rather than a timer: there
# is nothing to do until a file actually changes, and a timer would wake up to find
# that out.
#
# Safe to run concurrently with the app: git only reads the working tree, and a
# document being rewritten mid-commit lands in the next run instead of this one.
set -eu

DIR=${DIR:-/opt/knowledge/docs}
REMOTE=${REMOTE:-origin}
BRANCH=${BRANCH:-main}

[ -d "$DIR/.git" ] || { echo "not a git repository: $DIR (git init it first)" >&2; exit 1; }
cd "$DIR"

commit_once() {
	git add -A
	# Nothing staged is the normal case — a .path unit fires on every write,
	# including the ones git does not care about. Say nothing, so the journal
	# stays readable.
	git diff --cached --quiet && return 1

	# What changed, in the subject line, because "sync" as a commit message makes
	# the history unreadable exactly when someone is auditing what a BA published.
	FILES=$(git diff --cached --name-only)
	N=$(printf '%s\n' "$FILES" | wc -l | tr -d ' ')
	FIRST=$(printf '%s\n' "$FILES" | head -1)
	if [ "$N" = 1 ]; then SUBJECT="corpus: $FIRST"; else SUBJECT="corpus: $FIRST and $((N - 1)) more"; fi

	git -c user.name="${GIT_NAME:-knowledge-engine}" \
		-c user.email="${GIT_EMAIL:-knowledge@localhost}" \
		commit -q -m "$SUBJECT" -m "$FILES"
	echo "committed: $SUBJECT"
	return 0
}

# Settle, then look again. A .path unit stops watching while the service it
# triggered is running, so a file written during this run raises no event and would
# sit uncommitted until the *next* unrelated change — which is exactly what an
# import of several files does. Looping until the tree is clean closes that window
# from inside, without a schedule.
COMMITTED=1
i=0
while [ "$i" -lt 5 ]; do
	commit_once || break
	COMMITTED=0
	i=$((i + 1))
	sleep 2
done
[ "$COMMITTED" = 0 ] || exit 0

# Pushing is best-effort on purpose. The commit is the durable part; a push that
# fails because the box is offline must not leave the working tree dirty or make
# the unit look broken — the next change pushes both.
if git remote get-url "$REMOTE" >/dev/null 2>&1; then
	if git push -q "$REMOTE" "HEAD:$BRANCH" 2>/dev/null; then
		echo "pushed to $REMOTE/$BRANCH"
	else
		echo "commit kept locally: push to $REMOTE/$BRANCH failed" >&2
	fi
fi
