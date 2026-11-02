#!/bin/sh
# Runs inside a role's backend container (tools/backend.sh): stops what
# each named run started there, found by the SLK_EXEC_ID=<id> that
# run-docker.sh and go.sh put in the exec's environment and every child
# inherits. The run's root process (slk itself, or the timeout(1) wrapping
# it) gets one SIGINT, which quits slk through its shutdown path. With -g
# every process gets it, as a terminal's Ctrl+C does (go.sh: `go run` and
# `go test` don't pass it on to the program they run). Whatever is left
# after 5s gets SIGKILL. Exits once every named run is gone.
#
# A run gets its SIGINT once, however many stoppers race (a pane's launcher
# and its host bridge both stop it): bubbletea stops listening after the
# first, so a second SIGINT kills slk outright, mid-shutdown. The first
# stopper claims the run with a marker directory; the rest only wait, and
# SIGKILL what outlives their own 5s.
#
# Usage: stop-exec.sh [-g] <exec-id>...
set -u

all_get_int=
if [ "${1:-}" = -g ]; then
  all_get_int=1
  shift
fi

pids() {
  for id in "$@"; do
    grep -lzx "SLK_EXEC_ID=$id" /proc/[0-9]*/environ 2>/dev/null
  done | cut -d/ -f3
}

for id in "$@"; do
  run=$(pids "$id")
  [ -n "$run" ] || continue
  mkdir "/tmp/stop-exec-$id" 2>/dev/null || continue
  for p in $run; do
    if [ -z "$all_get_int" ]; then
      parent=$(ps -o ppid= -p "$p" | tr -d ' ')
      case " $(echo $run) " in
        *" $parent "*) continue ;;
      esac
    fi
    kill -INT "$p" 2>/dev/null
  done
done
for _ in $(seq 50); do
  [ -n "$(pids "$@")" ] || break
  sleep 0.1
done
left=$(pids "$@")
[ -z "$left" ] || kill -KILL $left 2>/dev/null
for id in "$@"; do
  rmdir "/tmp/stop-exec-$id" 2>/dev/null
done
exit 0
