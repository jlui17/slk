#!/usr/bin/env bash
# Boot slk headlessly for a bounded window and assert the session was clean.
# This is the one machine-checkable end-to-end signal the TUI emits: a real
# boot against real Slack (the agent sandbox's seeded tokens), quit via
# SIGINT so the shutdown API tally is written, then assertions on the debug
# log. Near-read-only: boot fetches state and sends one conversations.mark
# for the restored channel (visible in the tally), nothing else.
#
# Usage: tools/smoke.sh [seconds]   (default 25)
#
# Asserts:
#   - slk survives the whole window (no early exit)
#   - exactly one "shutdown API request tally" line (clean shutdown)
#   - no "failed to connect" lines (every workspace connected)
#   - no reconnect catch-up passes (the socket stayed up)
# Prints the tally for eyeballing; keeps the log on failure.
set -euo pipefail

secs=${1:-25}
repo=$(cd "$(dirname "$0")/.." && pwd)
logdir=$(mktemp -d /tmp/slk-smoke.XXXXXX)

echo "smoke: booting slk for ${secs}s in the agent sandbox..." >&2
runlog="$logdir/run-docker.log"
rc=0
SLK_ROLE=agent SLK_DEBUG=1 SLK_LOG_DIR="$logdir" SLK_TIMEOUT="$secs" \
  "$repo/tools/run-docker.sh" >/dev/null 2>"$runlog" || rc=$?
grep '^container:' "$runlog" >&2 || true
# timeout(1) exits 124 only when the window elapsed and it fired SIGINT.
# Anything else — a docker/build failure, or slk quitting on its own
# before the window — is a failure of "slk survives the whole window".
if [ "$rc" -ne 124 ]; then
  echo "smoke: FAIL — slk exited $rc before the ${secs}s window elapsed; launcher output:" >&2
  tail -15 "$runlog" >&2
  echo "smoke: log kept at $logdir" >&2
  exit 1
fi

log="$logdir/slk-debug.log"
if [ ! -s "$log" ]; then
  echo "smoke: FAIL — no debug log at $log" >&2
  exit 1
fi

fail=0
tally=$(grep -c 'shutdown API request tally' "$log" || true)
if [ "$tally" -ne 1 ]; then
  echo "smoke: FAIL — expected exactly 1 shutdown tally, got $tally (unclean shutdown?)" >&2
  fail=1
fi
bad_connect=$(grep -c 'failed to connect' "$log" || true)
if [ "$bad_connect" -ne 0 ]; then
  echo "smoke: FAIL — $bad_connect workspace connect failure(s):" >&2
  grep 'failed to connect' "$log" >&2
  fail=1
fi
reconnects=$(grep -cE 'reconnect-sync|trigger=reconnect' "$log" || true)
if [ "$reconnects" -ne 0 ]; then
  echo "smoke: FAIL — $reconnects reconnect catch-up line(s), socket flapped:" >&2
  grep -E 'reconnect-sync|trigger=reconnect' "$log" | head -5 >&2
  fail=1
fi

echo "smoke: request tally:" >&2
sed -n '/shutdown API request tally/,$p' "$log" >&2

if [ "$fail" -ne 0 ]; then
  echo "smoke: FAIL — log kept at $log" >&2
  exit 1
fi
rm -rf "$logdir"
echo "smoke: OK — ${secs}s boot, clean shutdown, no reconnects" >&2
