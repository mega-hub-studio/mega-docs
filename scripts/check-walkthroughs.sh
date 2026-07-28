#!/usr/bin/env bash
# Renders the guide, serves it, and drives every diagram walkthrough in a real browser.
#
# Same rig as check-docs-ui.sh, because it is literally the same file now — guide-rig.sh.
# Different default ports, so the two checks can run back to back (or at once) without
# fighting over either one.
set -euo pipefail
cd "$(dirname "$0")/.."

rig_name=check-walkthroughs
rig_portvar=PINCHTAB_PORT_WT
port=${PORT_WT:-8125}
ptport=${PINCHTAB_PORT_WT:-9875}
. scripts/guide-rig.sh

PINCHTAB_BIN="$PT" PINCHTAB_SERVER="http://127.0.0.1:$ptport" \
  node scripts/check-walkthroughs.mjs "http://127.0.0.1:$port"
