#!/usr/bin/env bash
# Restart every live user-role slk session in the current herdr session so
# they pick up the current source: stop all of them first, rebuild
# bin/slk-linux once while nothing executes it, then relaunch each pane.
# Judgment, gotchas, and when not to use this:
# plugins/slk-herdr/skills/slk-herdr/SKILL.md
#
# SLK_ROLE=agent (with SLK_STATE_VOL, as run-docker.sh takes them) points it
# at agent-role sessions instead, for testing this script in a herdr
# session of the agent's own (HERDR_SOCKET_PATH).
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd -P)
# The role's backend and slk binary, named as run-docker.sh names them; the
# role is user unless SLK_ROLE says otherwise, whatever CLAUDECODE says.
SLK_ROLE=${SLK_ROLE:-user}
. "$repo/tools/backend.sh"

# Discover the role's slk panes from each pane's foreground: a launch shows
# the run-docker.sh that started it, and that launcher's pid begins its exec
# id, the name of its launch directory under the role's $launches. That
# holds for a launch from before host-bridge.py attached it too, so a
# refresh moves those over. Only argv's script name is read, and python
# prints only pane ids and exec ids, so nothing else in a pane's argv (a
# permalink) or env can leak into transcripts.
panes=()
exec_ids=()
while IFS=$'\t' read -r pane ids; do
  panes+=("$pane")
  exec_ids+=($ids)
done < <(
  herdr workspace list | python3 -c '
import json, os, subprocess, sys

launches = sys.argv[1]
try:
    names = os.listdir(launches)
except FileNotFoundError:
    names = []

def run(*args):
    return json.loads(subprocess.check_output(("herdr",) + args))

for ws in json.load(sys.stdin)["result"]["workspaces"]:
    panes = run("pane", "list", "--workspace", ws["workspace_id"])["result"]["panes"]
    for pane in panes:
        pane_id = pane["pane_id"]
        try:
            info = run("pane", "process-info", "--pane", pane_id)
        except subprocess.CalledProcessError:
            continue
        for proc in info["result"]["process_info"].get("foreground_processes") or []:
            argv = proc.get("argv") or []
            if len(argv) > 1 and os.path.basename(argv[1]) == "run-docker.sh":
                # Every one: a reused pid can leave an old launch directory
                # under the same prefix, and stopping a launch that is gone
                # does nothing.
                ids = [n for n in names if n.startswith("%d-" % proc["pid"])]
                if ids:
                    print(pane_id, " ".join(ids), sep="\t")
                    break
' "$launches"
)

if [ "${#panes[@]}" -eq 0 ]; then
  echo "no live $role slk sessions found; nothing to restart" >&2
  echo "(a dead pane has no process to find; bring dead panes back with:" >&2
  echo "  SLK_ROLE=$role $repo/tools/run-docker.sh herdr reboot)" >&2
  exit 0
fi

# Refuse to eat a draft: the composer placeholder "(i to insert)" renders
# only while the input box is empty, so its absence means typed text or an
# overlay hiding the composer. Either way a human should look first.
blocked=()
for pane in "${panes[@]}"; do
  if ! herdr pane read "$pane" --source visible | grep -q "(i to insert)"; then
    blocked+=("$pane")
  fi
done
if [ "${#blocked[@]}" -gt 0 ]; then
  echo "aborting, composer not empty (or hidden) in: ${blocked[*]}" >&2
  echo "send or clear the draft there, then rerun" >&2
  exit 1
fi

# Stop them all: SIGINT to each launch inside the backend (slk's shutdown
# path; tools/stop-exec.sh returns once they are gone).
echo "stopping ${#panes[@]} slk session(s): ${panes[*]}" >&2
docker exec "$backend" sh "$repo/tools/stop-exec.sh" "${exec_ids[@]}"

# Wait for each pane's shell to be back in the foreground: pane run types
# into whatever is frontmost, so running it before run-docker.sh's cleanup
# traps finish would feed the command to the dying script instead.
for pane in "${panes[@]}"; do
  for _ in $(seq 1 20); do
    clear=$(herdr pane process-info --pane "$pane" | python3 -c '
import json, sys
procs = json.load(sys.stdin)["result"]["process_info"].get("foreground_processes") or []
print(all("run-docker.sh" not in (p.get("cmdline") or "") for p in procs))')
    [ "$clear" = "True" ] && break
    sleep 0.5
  done
done

# The relaunch is typed into each pane's shell, which picks its role from
# its own environment, and herdr pane shells have no CLAUDECODE: a role or
# state volume other than the default rides in the command, or an agent
# testing with SLK_ROLE=agent would relaunch its panes as the user role.
relaunch=$repo/tools/run-docker.sh
[ "$role" = user ] || relaunch="SLK_ROLE=$role $relaunch"
[ -z "${SLK_STATE_VOL:-}" ] || relaunch="SLK_STATE_VOL=$SLK_STATE_VOL $relaunch"

# With every session stopped, recreate the backend if run-docker.sh changed
# how it is made (only possible with nothing running in it), and build the
# binary once if sources are newer, before any relaunch executes it.
ensure_backend
ensure_binary

for pane in "${panes[@]}"; do
  herdr pane run "$pane" "$relaunch"
done
for pane in "${panes[@]}"; do
  herdr pane wait-output "$pane" --match "NORMAL" --timeout 60000 >/dev/null
done

echo "restarted ${#panes[@]} slk session(s): ${panes[*]}"
