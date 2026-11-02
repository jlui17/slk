#!/usr/bin/env bash
# Restart every live user-role slk session in the current herdr session so
# they pick up the current source: stop all of them first, then relaunch
# each pane, letting the first relaunch rebuild bin/slk-linux while nothing
# executes it. Judgment, gotchas, and when not to use this:
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
# run-docker.sh's `docker exec` into the role's backend, carrying the
# launch's SLK_EXEC_ID and running the role's slk binary (a tools/go.sh run
# in a pane is a `docker exec` into the same backend, running go). A pane
# still running a launch from before the
# shared backend shows its `docker run` with the slk.role label and the
# container name instead; refresh stops those too, and relaunches every
# pane into the backend, which is how live sessions move over. Parsing
# stays in python and prints only pane id, kind and exec id or container
# name, so an env value in that argv can't leak into transcripts.
panes=()
kinds=()
targets=()
while IFS=$'\t' read -r pane kind target; do
  panes+=("$pane")
  kinds+=("$kind")
  targets+=("$target")
done < <(
  herdr workspace list | python3 -c '
import json, subprocess, sys

role, backend, slk_bin = sys.argv[1:]

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
            if backend in argv and any(a.endswith("/bin/" + slk_bin) for a in argv):
                ids = [a.split("=", 1)[1] for a in argv if a.startswith("SLK_EXEC_ID=")]
                if ids:
                    print(pane_id, "exec", ids[0], sep="\t")
                    break
            if "slk.role=" + role in argv and "--name" in argv:
                print(pane_id, "run", argv[argv.index("--name") + 1], sep="\t")
                break
' "$role" "$backend" "$bin_name"
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
# path; tools/stop-exec.sh returns once they are gone), docker stop
# (SIGTERM, which slk handles the same way) for a container of its own.
echo "stopping ${#panes[@]} slk session(s): ${panes[*]}" >&2
exec_ids=()
containers=()
for i in "${!panes[@]}"; do
  case "${kinds[$i]}" in
    exec) exec_ids+=("${targets[$i]}") ;;
    run) containers+=("${targets[$i]}") ;;
  esac
done
if [ "${#exec_ids[@]}" -gt 0 ]; then
  docker exec "$backend" sh "$repo/tools/stop-exec.sh" "${exec_ids[@]}"
fi
if [ "${#containers[@]}" -gt 0 ]; then
  docker stop "${containers[@]}" >/dev/null
fi

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

# First relaunch alone: it rebuilds the binary when sources are newer, and
# the build must finish before any other launch executes the file. It also
# recreates the backend if run-docker.sh changed how it is made, which it
# can only do with nothing running in it.
first=${panes[0]}
rest=("${panes[@]:1}")
echo "relaunching $first (rebuilds the binary if sources changed)..." >&2
herdr pane run "$first" "$relaunch"
herdr pane wait-output "$first" --match "NORMAL" --timeout 180000 >/dev/null

for pane in ${rest[@]+"${rest[@]}"}; do
  herdr pane run "$pane" "$relaunch"
done
for pane in ${rest[@]+"${rest[@]}"}; do
  herdr pane wait-output "$pane" --match "NORMAL" --timeout 60000 >/dev/null
done

echo "restarted ${#panes[@]} slk session(s): ${panes[*]}"
