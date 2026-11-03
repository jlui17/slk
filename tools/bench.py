#!/usr/bin/env python3
# Baseline performance numbers for slk as it runs on this machine: launch
# latency and where it goes, boot cost on a warm vs cold cache.db, idle CPU
# and memory of one instance, and the docker-wrapped build/test loop.
#
# Usage: tools/bench.py [-n runs] [launch|cold|steady|build ...]  (default: all, 5 runs)
#
# Each launch is the real path a user takes: `tools/run-docker.sh` in the
# agent role, inside a 50x200 pty so the TUI draws real frames, bounded
# by SLK_TIMEOUT so it shuts down cleanly and writes its API request tally.
# The launcher runs under `bash -x` and every trace line is timestamped as
# it arrives, which splits the launch into its steps without editing the
# launcher (macOS bash 3.2 has no sub-second clock of its own, hence
# Python). slk's own steps come from its SLK_DEBUG log, whose timestamps
# share the host clock (the container gets the host TZ). First frame is the
# first time the status bar's mode label ("NORMAL") reaches the pty.
#
# The herdr bridge starts as it does in a herdr pane, but against a socket
# that doesn't exist, so a bench run never reaches Justin's herdr.
# Terminal probes go unanswered in this pty, so each costs its full 200ms
# timeout; the per-run probe time is printed so it can be read apart.
#
# Runs get a state volume of their own, slk-bench-state, copied fresh from
# the agent sandbox (config, tokens, cache.db, image cache) at the start and
# removed at the end, so other agents' sessions can't skew its cache and
# `cold` can delete its cache.db (never the image cache) before each run
# without pulling one out from under anyone. Other slk instances still share
# the CPU; the header line counts them. Debug logs are kept only when a run
# fails. The launcher trace holds the Anthropic key, so it never hits disk.
#
# The bench volume gets a backend container of its own (run-docker.sh),
# BACKEND, removed with the volume (and its launch directory).
import argparse
import datetime
import fcntl
import json
import os
import pty
import re
import select
import shutil
import statistics
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
IMAGE = "slk-go:1.26"
SEED_VOL = "slk-agent-state"
VOL = "slk-bench-state"
BACKEND = "slk-agent-backend-" + VOL  # run-docker.sh's name for it
STAMP = os.path.join(os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache"),
                     "slk-sandbox", "seeded-" + VOL)
SLK_DATA = "/state/xdg/data/slk"
WINDOW = 20  # seconds per launch; the request tally covers this window
PAUSE = 5  # seconds between launches, to go easy on Slack


def slk_instances(names):
    """How many slk processes run in these containers, read from the host
    (docker top), so a shared backend counts each of its panes."""
    count = 0
    for name in names:
        out = subprocess.run(["docker", "top", name, "-o", "pid,comm"], capture_output=True, text=True).stdout
        count += sum(1 for line in out.splitlines() if line.split()[-1:] in (["slk-linux"], ["slk-linux-agent"]))
    return count


def vol_sh(script, *mounts):
    """Run a shell snippet against the bench state volume in a throwaway container."""
    subprocess.run(["docker", "run", "--rm", "--label", "slk.role=agent", "-v", VOL + ":/state"]
                   + [a for m in mounts for a in ("-v", m)] + [IMAGE, "sh", "-c", script], check=True)


def seed_volume():
    """Make VOL a fresh copy of the agent sandbox, already marked seeded for run-docker.sh."""
    subprocess.run(["docker", "rm", "-f", BACKEND], capture_output=True)
    subprocess.run(["docker", "volume", "rm", VOL], capture_output=True)
    subprocess.run(["docker", "volume", "create", VOL], check=True, stdout=subprocess.DEVNULL)
    vol_sh("cp -a /seed/. /state/", SEED_VOL + ":/seed:ro")
    # run-docker.sh skips its seeder when this stamp matches the volume, so
    # the first run isn't charged for it (and never mounts the user sandbox).
    created = subprocess.run(["docker", "volume", "inspect", "-f", "{{ .CreatedAt }}", VOL],
                             capture_output=True, text=True, check=True).stdout.strip()
    os.makedirs(os.path.dirname(STAMP), exist_ok=True)
    with open(STAMP, "w") as f:
        f.write(created + "\n")


def log_time(line):
    return datetime.datetime.strptime(line[:26], "%Y/%m/%d %H:%M:%S.%f").timestamp()


def launch(window, sampler=None):
    """One run of tools/run-docker.sh in a sized pty; returns its timeline in seconds."""
    logdir = tempfile.mkdtemp(prefix="slk-bench.", dir="/tmp")
    env = dict(os.environ, SLK_ROLE="agent", SLK_STATE_VOL=VOL, SLK_DEBUG="1", SLK_LOG_DIR=logdir,
               SLK_TIMEOUT=str(window), HERDR_ENV="1", HERDR_PANE_ID="bench",
               HERDR_SOCKET_PATH=os.path.join(logdir, "no-herdr.sock"))
    for k in ("HERDR_TAB_ID", "HERDR_WORKSPACE_ID"):
        env.pop(k, None)
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 50, 200, 0, 0))
    t0 = time.time()
    p = subprocess.Popen(["/bin/bash", "-x", os.path.join(REPO, "tools/run-docker.sh")],
                         stdin=slave, stdout=slave, stderr=subprocess.PIPE,
                         env=env, start_new_session=True)
    os.close(slave)
    trace, tail, frame, errbuf = [], b"", None, b""
    fds = [master, p.stderr.fileno()]
    while fds:
        ready, _, _ = select.select(fds, [], [], 1.0)
        for fd in ready:
            try:
                data = os.read(fd, 65536)
            except OSError:  # the pty reports EIO once the launcher exits
                data = b""
            now = time.time() - t0
            if not data:
                fds.remove(fd)
            elif fd == master:
                if frame is None and b"NORMAL" in tail + data:
                    frame = now
                tail = data[-8:]
            else:
                errbuf += data
                while b"\n" in errbuf:
                    line, errbuf = errbuf.split(b"\n", 1)
                    line = line.decode(errors="replace")
                    trace.append((now, line))
                    m = re.match(r"container: (\S+)$", line)
                    if m and sampler:
                        threading.Thread(target=sampler, args=(m.group(1),), daemon=True).start()
    p.wait()
    os.close(master)

    def at(pattern):
        return next((t for t, l in trace if re.match(pattern, l)), None)

    r = {"rc": p.returncode, "first_frame": frame,
         "stamp_check": at(r"\+ ensure_backend$"),
         "rebuilt": any("go build" in l for _, l in trace)}
    binl, tz, run = (at(r"\+ ensure_binary$"), at(r"\++ readlink /etc/localtime"),
                     at(r"\+ python3 \S+/host-bridge\.py "))
    if None in (r["stamp_check"], binl, tz, run):
        print("bench: launcher trace incomplete (rc=%s), tail:" % p.returncode, file=sys.stderr)
        for _, line in trace[-10:]:
            if "API_KEY" not in line:
                print("  " + line[:200], file=sys.stderr)
        return r
    r["backend"] = binl - r["stamp_check"]
    r["staleness_check"] = tz - binl
    r["shell_other"] = run - binl - r["staleness_check"]
    r["shell_total"] = run

    hellos, probes, teams = [], 0.0, 0
    started = protocol = None
    try:
        with open(os.path.join(logdir, "slk-debug.log"), errors="replace") as f:
            for line in f:
                if "debug session started" in line:
                    started = log_time(line) - t0
                elif "image protocol: " in line:
                    protocol = log_time(line) - t0
                elif re.search(r"(png|sixel) probe: .*elapsed_ms=(\d+)", line):
                    probes += int(re.search(r"elapsed_ms=(\d+)", line).group(1)) / 1000
                elif "registered team" in line:
                    teams += 1
                elif "[ws] hello: connected" in line:
                    hellos.append(log_time(line) - t0)
                elif line.startswith("API requests:"):
                    r["requests"] = int(line.split()[2])
                if "Ratelimited" in line:
                    r["ratelimited"] = r.get("ratelimited", 0) + 1
    except FileNotFoundError:
        pass
    r.setdefault("ratelimited", 0)
    if started is not None:
        r["bridge_to_main"] = started - run
    if protocol is not None:
        r["init_and_probes"] = protocol - started
        r["probes"] = probes
        if frame is not None:
            r["probes_to_frame"] = frame - protocol
    if hellos:
        r["first_connected"] = hellos[0]
    if teams and len(hellos) >= teams:
        r["all_connected"] = hellos[teams - 1]

    ok = p.returncode == 124 and frame is not None and "requests" in r
    if ok:
        shutil.rmtree(logdir)
    else:
        print("bench: run failed (rc=%s, frame=%s); log kept at %s" % (p.returncode, frame, logdir),
              file=sys.stderr)
    return r


ROWS = [
    ("stamp check (docker volume inspect)", "stamp_check"),
    ("backend check (docker inspect)", "backend"),
    ("binary staleness check", "staleness_check"),
    ("other launcher shell", "shell_other"),
    ("= launcher before host-bridge.py", "shell_total"),
    ("host-bridge.py start -> slk main()", "bridge_to_main"),
    ("slk init incl. terminal probes", "init_and_probes"),
    ("  of which probe timeouts", "probes"),
    ("probes done -> first frame", "probes_to_frame"),
    ("= FIRST FRAME (from invoke)", "first_frame"),
    ("first workspace connected", "first_connected"),
    ("ALL WORKSPACES CONNECTED", "all_connected"),
    ("API requests in %ds window" % WINDOW, "requests"),
    ("ws Ratelimited errors", "ratelimited"),
]


def summarize(title, runs):
    print("\n%s (%d runs)%s" % (title, len(runs), " median / min / max".rjust(52 - len(title))))
    for label, key in ROWS:
        vals = [r[key] for r in runs if r.get(key) is not None]
        if not vals:
            print("  %-38s n/a" % label)
            continue
        fmt = "%9d" if isinstance(vals[0], int) else "%9.3f"
        line = "  %-38s" % label + "".join(fmt % v for v in
                                           (statistics.median(vals), min(vals), max(vals)))
        if len(vals) < len(runs):
            line += "   (%d of %d runs)" % (len(vals), len(runs))
        print(line)
    if any(r["rebuilt"] for r in runs):
        print("  note: a run rebuilt bin/slk-linux-agent; its staleness step includes the build")


def bench_launches(title, n, before=None):
    runs = []
    for i in range(n):
        if before:
            before()
        r = launch(WINDOW)
        print("bench: %s run %d/%d: %s" % (title, i + 1, n, json.dumps(
            {k: round(v, 3) if isinstance(v, float) else v for k, v in r.items()})), file=sys.stderr)
        runs.append(r)
        time.sleep(PAUSE)
    summarize(title, runs)


def bench_cold(n):
    bench_launches("launch, cold cache.db", n, before=lambda: vol_sh("rm -f %s/cache.db*" % SLK_DATA))


def bench_steady(settle=30, span=120, every=10):
    samples = []

    def sampler(name):
        time.sleep(settle)
        for _ in range(span // every + 1):
            out = subprocess.run(["docker", "exec", name, "sh", "-c",
                                  "head -1 /sys/fs/cgroup/cpu.stat; ps -C slk-linux-agent -o rss=;"
                                  " cat /sys/fs/cgroup/memory.current"],
                                 capture_output=True, text=True).stdout.split()
            if len(out) == 4:
                samples.append((time.time(), int(out[1]), int(out[2]), int(out[3])))
            time.sleep(every)

    launch(settle + span + every + 10, sampler=sampler)
    print("\nsteady state, one agent instance, %ds after a %ds settle (sample every %ds)"
          % (span, settle, every))
    if len(samples) < 2:
        print("  no samples (did the instance start?)")
    else:
        cpu = [100 * (b[1] - a[1]) / ((b[0] - a[0]) * 1e6) for a, b in zip(samples, samples[1:])]
        avg = 100 * (samples[-1][1] - samples[0][1]) / ((samples[-1][0] - samples[0][0]) * 1e6)
        rss = [s[2] / 1024 for s in samples]
        cg = [s[3] / 2**20 for s in samples]
        print("  CPU %% of one core: avg %.2f, per-%ds median %.2f min %.2f max %.2f"
              % (avg, every, statistics.median(cpu), min(cpu), max(cpu)))
        print("  slk RSS MiB:       median %.1f min %.1f max %.1f" % (statistics.median(rss), min(rss), max(rss)))
        print("  cgroup memory MiB: median %.1f min %.1f max %.1f  (docker stats' number; whole container, includes page cache)"
              % (statistics.median(cg), min(cg), max(cg)))
    names = subprocess.run(["docker", "ps", "--filter", "label=slk.role=user", "--format", "{{.Names}}"],
                           capture_output=True, text=True).stdout.split()
    if names:
        out = subprocess.run(["docker", "stats", "--no-stream", "--format",
                              "{{.Name}} {{.CPUPerc}} {{.MemUsage}}"] + names,
                             capture_output=True, text=True).stdout
        print("\nlive user instances (docker stats --no-stream, read-only), %d running in %d container(s):"
              % (slk_instances(names), len(names)))
        print("  " + out.strip().replace("\n", "\n  "))


def bench_build(n):
    go = os.path.join(REPO, "tools/go.sh")
    cmds = [
        ("tools/go.sh version  (docker overhead)", [go, "version"]),
        ("tools/go.sh build ./...", [go, "build", "./..."]),
        ("tools/go.sh build -o bin/slk-linux-agent", [go, "build", "-ldflags=-s -w", "-trimpath",
                                                      "-o", "bin/slk-linux-agent", "./cmd/slk"]),
        ("tools/go.sh test -count=1 ./cmd/slk", [go, "test", "-count=1", "./cmd/slk"]),
    ]
    print("\nbuild/test loop, warm caches (%d runs after one warmup)%s"
          % (n, "median / min / max".rjust(19)))
    for label, cmd in cmds:
        times = []
        for i in range(n + 1):
            start = time.monotonic()
            subprocess.run(cmd, cwd=REPO, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if i:
                times.append(time.monotonic() - start)
        print("  %-42s %9.3f%9.3f%9.3f" % (label, statistics.median(times), min(times), max(times)))


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("-n", type=int, default=5, help="runs per measurement (default 5)")
    ap.add_argument("sections", nargs="*", help="launch, cold, steady, build (default: all)")
    args = ap.parse_args()
    sys.stdout.reconfigure(line_buffering=True)  # keep results in order with stderr progress
    all_sections = ["launch", "cold", "steady", "build"]
    sections = args.sections or all_sections
    if set(sections) - set(all_sections):
        ap.error("unknown section in %s" % sections)

    def running(*filters):
        return subprocess.run(["docker", "ps", "--format", "{{.Names}}"] + [a for f in filters for a in ("--filter", f)],
                              capture_output=True, text=True).stdout.split()

    if slk_instances(running("volume=" + VOL)):
        sys.exit("bench: another bench is running (%s uses %s)" % (" ".join(running("volume=" + VOL)), VOL))
    print("bench: %s, %s cores, docker %s; alongside %d user and %d other agent slk instances" % (
        subprocess.run(["sysctl", "-n", "machdep.cpu.brand_string"], capture_output=True, text=True).stdout.strip(),
        os.cpu_count(),
        subprocess.run(["docker", "version", "--format", "{{.Server.Version}}"], capture_output=True, text=True).stdout.strip(),
        slk_instances(running("label=slk.role=user")), slk_instances(running("label=slk.role=agent"))))
    if set(sections) & {"launch", "cold", "steady"}:
        seed_volume()
        try:
            if "launch" in sections:
                bench_launches("launch, warm cache.db", args.n)
            if "cold" in sections:
                bench_cold(args.n)
            if "steady" in sections:
                bench_steady()
        finally:
            subprocess.run(["docker", "rm", "-f", BACKEND], capture_output=True)
            subprocess.run(["docker", "volume", "rm", VOL], capture_output=True)
            os.remove(STAMP)
            shutil.rmtree(os.path.join(os.path.dirname(STAMP), BACKEND), ignore_errors=True)
    if "build" in sections:
        bench_build(args.n)


if __name__ == "__main__":
    main()
