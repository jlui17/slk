#!/usr/bin/env python3
# Host-side bridges for one slk launch, in one process so a launch pays
# Python's startup once. The container reaches the first two at
# host.docker.internal:<port>:
#
#   herdr      TCP -> unix-socket forwarder for the herdr socket. Docker
#              Desktop does not forward bind-mounted unix sockets across the
#              VM boundary, so this relays to the real socket on the host.
#   clipboard  HTTP server for Ctrl+V smart paste: the container cannot see
#              the macOS clipboard, so it asks here for whatever image or
#              text the host clipboard holds, read through osascript.
#   spool      Opens on the host each URL tools/spool-open drops into this
#              directory: the container has no browser.
#
# It also reaps a launch that `docker exec`s into a shared backend
# container (exec=<container>:<exec-id>:<launch-dir>): an exec'd process
# outlives its docker client however the pane goes away (closed, SIGHUP,
# the launcher SIGKILLed), so when <parent-pid> exits this stops that
# launch's processes in the container with tools/stop-exec.sh, hands slk's
# debug log to $SLK_LOG_DIR, and removes the launch's directory. To be there
# for that, it leaves the pane's session: closing a herdr tab sends the
# pane SIGHUP, then SIGTERM, then SIGKILL half a second in, faster than slk
# shuts down. On start it also removes launch directories that an earlier
# launch left behind.
#
# Usage: host-bridge.py <parent-pid> [herdr=<socket-path>] [clipboard]
#                       [spool=<dir>] [exec=<container>:<exec-id>:<dir>]
# Prints one line naming each bridge's port ("herdr=<port> clipboard=<port>",
# empty when there are none) once all are listening, then exits when
# <parent-pid> does.
#
# Stdlib only: python3 and osascript are Apple-signed, so Santa's
# binary-killing policy doesn't apply.
import os
import select
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer


def pump(src, dst):
    try:
        while True:
            data = src.recv(65536)
            if not data:
                break
            dst.sendall(data)
    except OSError:
        pass
    finally:
        # Half-close so the other pump sees EOF; full close after both ends
        # would cut off in-flight data in the opposite direction.
        try:
            dst.shutdown(socket.SHUT_WR)
        except OSError:
            pass


def relay(tcp_conn, unix_path):
    try:
        unix_conn = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        unix_conn.connect(unix_path)
    except OSError:
        tcp_conn.close()
        return
    t = threading.Thread(target=pump, args=(tcp_conn, unix_conn), daemon=True)
    t.start()
    pump(unix_conn, tcp_conn)
    t.join()
    tcp_conn.close()
    unix_conn.close()


def serve_herdr(server, unix_path):
    while True:
        conn, _ = server.accept()
        threading.Thread(target=relay, args=(conn, unix_path), daemon=True).start()


def clipboard_image():
    """PNG bytes, or None when the clipboard holds no image."""
    fd, path = tempfile.mkstemp(suffix=".png")
    os.close(fd)
    try:
        # stderr is discarded on success: osascript prints a harmless
        # "Error creating a JP2 color space" while converting.
        result = subprocess.run(
            ["osascript",
             "-e", f'set f to open for access POSIX file "{path}" with write permission',
             "-e", "set eof f to 0",
             "-e", "write (the clipboard as «class PNGf») to f",
             "-e", "close access f"],
            capture_output=True)
        if result.returncode != 0:
            # -1700 "Can't make some data into the expected type": no image.
            if b"-1700" not in result.stderr:
                sys.stderr.write(f"host-bridge: osascript: {result.stderr.decode(errors='replace').strip()}\n")
            return None
        with open(path, "rb") as f:
            return f.read()
    finally:
        os.unlink(path)


def clipboard_text():
    """UTF-8 bytes, or None when the clipboard holds no text."""
    result = subprocess.run(["osascript", "-e", "the clipboard as text"], capture_output=True)
    if result.returncode != 0:
        return None
    return result.stdout.removesuffix(b"\n")


class ClipboardHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/image":
            self.reply(clipboard_image(), "image/png")
        elif self.path == "/text":
            self.reply(clipboard_text(), "text/plain; charset=utf-8")
        else:
            self.send_error(404)

    def reply(self, body, content_type):
        if body is None:
            self.send_response(204)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        # Keep the parent shell's stderr quiet; the TUI owns that terminal.
        pass


def watch_spool(spool):
    while True:
        try:
            names = os.listdir(spool)
        except OSError:
            return
        for name in names:
            if not name.startswith("url-"):
                continue
            path = os.path.join(spool, name)
            try:
                with open(path) as f:
                    url = f.read().strip()
                os.unlink(path)
            except OSError:
                continue
            subprocess.run(["open", url], stdin=subprocess.DEVNULL, capture_output=True)
        time.sleep(0.3)


def alive(pid):
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        pass
    return True


def sweep_launch_dirs(container, root, own):
    """Removes launch directories left behind: the launcher is gone, nothing
    with that exec id runs in the backend, and the directory is a minute old,
    so the launch's own bridge is long done with it. Directory names are
    exec ids, <launcher pid>-<random>."""
    stale = []
    for name in os.listdir(root):
        path = os.path.join(root, name)
        try:
            launcher = int(name.split("-", 1)[0])
            old = time.time() - os.stat(path).st_mtime > 60
        except (ValueError, OSError):
            continue
        if name != own and old and not alive(launcher):
            stale.append(name)
    if not stale:
        return
    # Only SLK_EXEC_ID lines leave the container, never other env values.
    # The command exits 0 whenever it ran, matches or not, so any other exit
    # is docker failing (a paused or unreachable backend): the running ids
    # are then unknown, and nothing is swept.
    try:
        running = subprocess.run(
            ["docker", "exec", container, "sh", "-c",
             "cat /proc/[0-9]*/environ 2>/dev/null | tr '\\0' '\\n' | grep '^SLK_EXEC_ID=' || true"],
            stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=STOP_CALL_SECS)
    except subprocess.TimeoutExpired:
        return
    if running.returncode != 0:
        return
    ids = {line.split("=", 1)[1] for line in running.stdout.split()}
    for name in stale:
        if name not in ids:
            shutil.rmtree(os.path.join(root, name), ignore_errors=True)


# A docker call that hangs (an unresponsive daemon) gets cut off and tried
# again; stop-exec.sh itself takes at most ~5s. Out of the pane's session,
# nothing else would ever end this process, so after REAP_DEADLINE_SECS of
# an unreachable backend it gives up and exits, leaving the launch
# directory to a later launch's sweep.
STOP_CALL_SECS = 20
STOP_RETRY_SECS = 3
REAP_DEADLINE_SECS = 120


def stop_launch(container, exec_id):
    """Runs tools/stop-exec.sh for exec_id in the backend until one call
    succeeds, retrying through a paused, busy or hung docker until the
    deadline. stop-exec.sh signals a run only once, however often it runs.
    Returns whether a call succeeded."""
    stop = os.path.join(os.path.dirname(os.path.realpath(__file__)), "stop-exec.sh")
    deadline = time.monotonic() + REAP_DEADLINE_SECS
    while True:
        try:
            if subprocess.run(["docker", "exec", container, "sh", stop, exec_id],
                              stdin=subprocess.DEVNULL, capture_output=True,
                              timeout=min(STOP_CALL_SECS, deadline - time.monotonic())).returncode == 0:
                return True
        except subprocess.TimeoutExpired:
            pass
        if time.monotonic() + STOP_RETRY_SECS >= deadline:
            return False
        time.sleep(STOP_RETRY_SECS)


def wait_for_exit(pid):
    """Returns once pid has exited: at once through kqueue on macOS, within
    a second elsewhere."""
    if hasattr(select, "kqueue"):
        kq = select.kqueue()
        try:
            kq.control([select.kevent(pid, select.KQ_FILTER_PROC, select.KQ_EV_ADD,
                                      select.KQ_NOTE_EXIT)], 0)
        except ProcessLookupError:
            return
        kq.control(None, 1)
        return
    while True:
        try:
            os.kill(pid, 0)
        except OSError:
            return
        time.sleep(1)


def main():
    parent = int(sys.argv[1])
    ports = []
    reap = None
    for arg in sys.argv[2:]:
        if arg.startswith("herdr="):
            server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            server.bind(("127.0.0.1", 0))
            server.listen()
            threading.Thread(target=serve_herdr, args=(server, arg[len("herdr="):]), daemon=True).start()
            ports.append("herdr=%d" % server.getsockname()[1])
        elif arg == "clipboard":
            server = HTTPServer(("127.0.0.1", 0), ClipboardHandler)
            threading.Thread(target=server.serve_forever, daemon=True).start()
            ports.append("clipboard=%d" % server.server_port)
        elif arg.startswith("spool="):
            threading.Thread(target=watch_spool, args=(arg[len("spool="):],), daemon=True).start()
        elif arg.startswith("exec="):
            reap = arg[len("exec="):].split(":", 2)
            os.setsid()
            for sig in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
                signal.signal(sig, signal.SIG_IGN)
            container, exec_id, launch_dir = reap
            threading.Thread(target=sweep_launch_dirs, daemon=True,
                             args=(container, os.path.dirname(launch_dir), exec_id)).start()
        else:
            sys.exit("host-bridge: unknown bridge %r" % arg)
    print(" ".join(ports), flush=True)

    # Exit with the launcher, even if its EXIT trap never fires.
    wait_for_exit(parent)
    if reap:
        container, exec_id, launch_dir = reap
        if not stop_launch(container, exec_id):
            return
        # The launcher hands the log over itself when slk exits on its own;
        # this covers a launcher that was signalled or killed.
        log_dir = os.environ.get("SLK_LOG_DIR")
        if log_dir:
            try:
                shutil.move(os.path.join(launch_dir, "slk-debug.log"), log_dir)
            except OSError:
                pass
        shutil.rmtree(launch_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
