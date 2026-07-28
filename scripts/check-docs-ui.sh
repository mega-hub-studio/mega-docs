#!/usr/bin/env bash
# Renders the guide, serves it, and measures it in a real browser.
#
# The rendering, serving, browser and cleanup are guide-rig.sh, shared with
# check-walkthroughs.sh — read that file for why each part of it is the shape it is. What is
# here is what makes this check this check: its ports and the script that does the measuring.
#
# Its own instance on its own port, like the other check: PinchTab commands act on an
# instance's current tab, so an editor or an MCP integration sharing the default instance
# navigates the tab out from under a measurement. Measured before this was added: 2 of 3 runs
# failed on a shared instance, 0 of 3 on a dedicated one.
set -euo pipefail
cd "$(dirname "$0")/.."

rig_name=check-ui
rig_portvar=PINCHTAB_PORT
port=${PORT_UI:-8123}
ptport=${PINCHTAB_PORT:-9871}
. scripts/guide-rig.sh

PINCHTAB_BIN="$PT" PINCHTAB_SERVER="http://127.0.0.1:$ptport" \
  node scripts/check-docs-ui.mjs "http://127.0.0.1:$port"
