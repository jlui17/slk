#!/usr/bin/env bash
# Restart every live user-role slk session in the current herdr session so
# they pick up the current source: stop all their containers first, then
# relaunch each pane, letting the first relaunch rebuild bin/slk-linux while
# nothing executes it. Judgment, gotchas, and when not to use this:
# .claude/skills/refresh-sessions/SKILL.md
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)

# Discover user slk panes: the pane's foreground holds run-docker.sh's
# docker run carrying the slk.role=user label, and the container name rides
# in the same argv. Parsing stays in python and prints only pane id +
# container name — the argv also carries credentials (-e ANTHROPIC_API_KEY)
# and must never be echoed.
panes=()
containers=()
while IFS=$'\t' read -r pane container; do
  panes+=("$pane")
  containers+=("$container")
done < <(
  herdr workspace list | python3 -c '
import json, subprocess, sys

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
            if "slk.role=user" in argv and "--name" in argv:
                print(pane_id, argv[argv.index("--name") + 1], sep="\t")
                break
'
)

if [ "${#panes[@]}" -eq 0 ]; then
  echo "no live user slk sessions found; nothing to restart" >&2
  echo "(a dead pane has no process to find — relaunch it by hand:" >&2
  echo "  herdr pane run <pane_id> $repo/tools/run-docker.sh)" >&2
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

echo "stopping ${#panes[@]} slk session(s): ${panes[*]}" >&2
docker stop "${containers[@]}" >/dev/null

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

# First relaunch alone: it rebuilds bin/slk-linux when sources are newer,
# and the build must finish before any other container executes the file.
first=${panes[0]}
rest=("${panes[@]:1}")
echo "relaunching $first (rebuilds the binary if sources changed)..." >&2
herdr pane run "$first" "$repo/tools/run-docker.sh"
herdr pane wait-output "$first" --match "NORMAL" --timeout 180000 >/dev/null

for pane in ${rest[@]+"${rest[@]}"}; do
  herdr pane run "$pane" "$repo/tools/run-docker.sh"
done
for pane in ${rest[@]+"${rest[@]}"}; do
  herdr pane wait-output "$pane" --match "NORMAL" --timeout 60000 >/dev/null
done

echo "restarted ${#panes[@]} slk session(s): ${panes[*]}"
