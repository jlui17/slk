#!/usr/bin/env python3
# The host side of one slk launch (tools/run-docker.sh), in one process so
# a pane holds just this beside its launcher shell.
#
# It runs slk in the role's shared backend container through the Docker
# Engine API, on the unix socket the docker CLI would use: it creates the
# exec, relays this terminal to it in raw mode, passes resizes on, and
# exits with slk's exit code.
#
# It leaves the pane's session, so a closing pane can't kill it before it
# has stopped slk: closing a herdr tab sends the pane SIGHUP, then SIGTERM,
# then SIGKILL half a second in, faster than slk shuts down, and an exec'd
# process outlives its client however the client goes. Once the launcher
# exits or the pane's terminal goes away, it stops the launch's processes
# in the backend (tools/stop-exec.sh), hands slk's debug log to
# $SLK_LOG_DIR, and removes the launch directory. Out of the session, the
# kernel sends it no SIGWINCH; the launcher forwards its own. On start it
# also removes launch directories that earlier launches left behind.
#
# It also serves what the container can't reach on its own, the first three
# at host.docker.internal:<port>, which it adds to the exec's environment:
#
#   herdr      TCP -> unix-socket forwarder for the herdr socket. Docker
#              Desktop does not forward bind-mounted unix sockets across the
#              VM boundary, so this relays to the real socket on the host.
#   clipboard  HTTP server for Ctrl+V smart paste: the container cannot see
#              the macOS clipboard, so it asks here for whatever image or
#              text the host clipboard holds, read through osascript.
#   anthropic-token
#              HTTP server for the model calls' credential when no key is
#              configured: GET /token answers with the host's ant login
#              access token (`ant auth print-credentials`), so the refresh
#              token stays on the host. SLK_ANTHROPIC_TOKEN_ADDR.
#   spool      Opens on the host each URL tools/spool-open drops into the
#              launch directory: the container has no browser.
#
# Usage: host-bridge.py <launcher-pid> <container> <exec-id> <launch-dir>
#            [herdr=<socket-path>] [clipboard] [anthropic-token] [spool]
#            -- [-w <dir>] [-e NAME[=value]]... <command>...
# An -e NAME without a value passes this process's own, so a secret stays
# out of argv.
#
# Stdlib only: python3 and osascript are Apple-signed, so Santa's
# binary-killing policy doesn't apply.
import fcntl
import hashlib
import http.client
import json
import os
import select
import shutil
import signal
import socket
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time
import tty
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import quote


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


class TokenUnavailable(Exception):
    """Why the host has no ant login token to serve."""


# A token is fetched again once it has less than this left: ant itself
# refreshes in a token's last two minutes, so that fetch gets the new one.
TOKEN_REFRESH_SECS = 90
# A token is reused for this long at most, so a new `ant auth login` or a
# logout reaches slk within a minute or two (slk reuses one as long).
TOKEN_REUSE_SECS = 60


def ant_access_token():
    """The active ant profile's access token and its expiry (unix seconds),
    from `ant auth print-credentials`, which refreshes a token near its
    expiry. The refresh token never leaves this function. An exported key
    or token would override the profile, so ant runs without them: this
    serves the ant login alone."""
    env = {k: v for k, v in os.environ.items() if k not in ("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN")}
    try:
        result = subprocess.run(["ant", "auth", "print-credentials"], capture_output=True, env=env, timeout=30)
    except FileNotFoundError:
        raise TokenUnavailable("ant is not installed on the host")
    except subprocess.TimeoutExpired:
        raise TokenUnavailable("ant auth print-credentials timed out")
    if result.returncode != 0:
        reason = result.stderr.decode(errors="replace").strip().splitlines()
        raise TokenUnavailable("ant auth print-credentials failed: %s (run `ant auth login` on the host)"
                               % (reason[0] if reason else "exit %d" % result.returncode))
    try:
        creds = json.loads(result.stdout)
        return {"access_token": creds["access_token"], "expires_at": int(creds["expires_at"])}
    except (ValueError, KeyError, TypeError):
        raise TokenUnavailable("the active ant profile has no OAuth access token (run `ant auth login` on the host)")


class AnthropicToken:
    """The ant login's access token, reused for TOKEN_REUSE_SECS and until
    TOKEN_REFRESH_SECS before it expires. A failure is never kept: a later
    ask runs ant again."""

    def __init__(self, fetch=ant_access_token):
        self.fetch = fetch
        self.lock = threading.Lock()
        self.token = None
        self.fetched_at = 0.0

    def get(self):
        with self.lock:
            if (self.token is None or time.monotonic() - self.fetched_at >= TOKEN_REUSE_SECS
                    or self.token["expires_at"] - time.time() < TOKEN_REFRESH_SECS):
                self.token = None
                self.token = self.fetch()
                self.fetched_at = time.monotonic()
            return self.token

    def warm(self):
        try:
            self.get()
        except TokenUnavailable:
            pass


class AnthropicTokenHandler(BaseHTTPRequestHandler):
    """GET /token: 200 with {"access_token", "expires_at"}, or 503 with why
    there is none, in plain text."""

    def do_GET(self):
        if self.path != "/token":
            self.send_error(404)
            return
        try:
            body, status, content_type = json.dumps(self.server.tokens.get()).encode(), 200, "application/json"
        except TokenUnavailable as e:
            body, status, content_type = str(e).encode(), 503, "text/plain; charset=utf-8"
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
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


def docker_socket():
    """The unix socket of the docker host the CLI would use, and where that
    came from, for messages: DOCKER_HOST, else the DOCKER_CONTEXT or
    config.json's current context, read from the context store the way the
    CLI reads it."""
    host = os.environ.get("DOCKER_HOST")
    source = "DOCKER_HOST"
    if not host:
        config_dir = os.environ.get("DOCKER_CONFIG") or os.path.expanduser("~/.docker")
        name = os.environ.get("DOCKER_CONTEXT")
        source = "docker context '%s' (DOCKER_CONTEXT)" % name
        if not name:
            try:
                with open(os.path.join(config_dir, "config.json")) as f:
                    name = json.load(f).get("currentContext")
            except (OSError, ValueError):
                pass
            name = name or "default"
            source = "docker context '%s'" % name
        host = "unix:///var/run/docker.sock"
        if name != "default":
            meta = os.path.join(config_dir, "contexts", "meta",
                                hashlib.sha256(name.encode()).hexdigest(), "meta.json")
            try:
                with open(meta) as f:
                    host = json.load(f)["Endpoints"]["docker"]["Host"]
            except (OSError, ValueError, KeyError):
                sys.exit("slk: can't find %s in %s" % (source, config_dir))
    if not host.startswith("unix://"):
        sys.exit("slk: can't attach to docker at %s (%s): run-docker.sh needs a unix socket" % (host, source))
    path = host[len("unix://"):]
    return path, "%s (%s)" % (path, source)


# Every Engine API call but the attached exec's stream is cut off after
# CALL_SECS, so a hung daemon can't hold this process forever.
CALL_SECS = 20


class EngineError(Exception):
    """The daemon answered with an error status."""

    def __init__(self, status, message):
        super().__init__(message)
        self.status = status


# What a failed Engine API call raises: an error answer, or no usable
# answer at all (no socket, a dropped connection, a timeout).
DOCKER_ERRORS = (EngineError, OSError, http.client.HTTPException, ValueError)


class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, path, timeout):
        super().__init__("docker", timeout=timeout)
        self.unix_path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.unix_path)


def engine(sock_path, method, path, body=None, timeout=CALL_SECS):
    """One Engine API call: its decoded JSON answer, None when empty."""
    conn = UnixHTTPConnection(sock_path, timeout)
    try:
        conn.request(method, path, None if body is None else json.dumps(body),
                     {"Content-Type": "application/json"})
        resp = conn.getresponse()
        data = resp.read()
    finally:
        conn.close()
    answer = json.loads(data) if data.strip() else None
    if resp.status >= 400:
        raise EngineError(resp.status, (answer or {}).get("message") or "HTTP %d" % resp.status)
    return answer


def exec_create(sock_path, container, spec, timeout=CALL_SECS):
    return engine(sock_path, "POST", "/containers/%s/exec" % quote(container), spec, timeout)["Id"]


def exec_start(sock_path, exec_id, body, timeout=CALL_SECS):
    """Starts an exec on a hijacked connection: once the daemon answers
    101, the socket is the exec's raw stream both ways. Returns the socket,
    its timeout still set, and any stream bytes that arrived with the
    response head."""
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        s.settimeout(timeout)
        s.connect(sock_path)
        payload = json.dumps(body).encode()
        s.sendall(("POST /exec/%s/start HTTP/1.1\r\nHost: docker\r\n"
                   "Content-Type: application/json\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n"
                   "Content-Length: %d\r\n\r\n" % (exec_id, len(payload))).encode() + payload)
        buf = b""
        while b"\r\n\r\n" not in buf:
            chunk = s.recv(65536)
            if not chunk:
                raise ConnectionError("docker closed the connection before starting the exec")
            buf += chunk
        head, rest = buf.split(b"\r\n\r\n", 1)
        status = int(head.split(b" ", 2)[1])
        if status not in (101, 200):
            raise EngineError(status, head.split(b"\r\n", 1)[0].decode(errors="replace"))
    except BaseException:
        s.close()
        raise
    return s, rest


def run(sock_path, container, cmd, timeout):
    """Runs cmd in container without a tty, as `docker exec` does, and
    returns its exit code and stdout, all within timeout seconds."""
    deadline = time.monotonic() + timeout

    def left():
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise socket.timeout("docker took over %ds" % timeout)
        return remaining

    exec_id = exec_create(sock_path, container, {"AttachStdout": True, "Cmd": cmd}, left())
    s, stream = exec_start(sock_path, exec_id, {"Detach": False, "Tty": False}, left())
    with s:
        while True:
            s.settimeout(left())
            chunk = s.recv(65536)
            if not chunk:
                break
            stream += chunk
    while True:
        info = engine(sock_path, "GET", "/exec/%s/json" % exec_id, timeout=left())
        if not info["Running"]:
            break
        time.sleep(0.05)
    # Without a tty the stream comes in frames: a byte naming the stream (1
    # is stdout), three of padding, a big-endian length, then the data.
    out = b""
    while len(stream) >= 8:
        size = struct.unpack(">I", stream[4:8])[0]
        if stream[0] == 1:
            out += stream[8:8 + size]
        stream = stream[8 + size:]
    return info["ExitCode"], out.decode(errors="replace")


def sweep_launch_dirs(sock_path, container, root, own):
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
    # or any docker error (a paused or unreachable backend) leaves the
    # running ids unknown, and nothing is swept.
    try:
        code, out = run(sock_path, container, ["sh", "-c",
                        "cat /proc/[0-9]*/environ 2>/dev/null | tr '\\0' '\\n' | grep '^SLK_EXEC_ID=' || true"],
                        CALL_SECS)
    except DOCKER_ERRORS:
        return
    if code != 0:
        return
    ids = {line.split("=", 1)[1] for line in out.split()}
    for name in stale:
        if name not in ids:
            shutil.rmtree(os.path.join(root, name), ignore_errors=True)


# One stop call gets STOP_CALL_SECS (stop-exec.sh itself takes at most
# ~5s), and a failed one is tried again every STOP_RETRY_SECS, through a
# paused, busy or hung docker. Out of the pane's session, nothing else
# would ever end this process, so after REAP_DEADLINE_SECS it gives up and
# exits, leaving the launch directory to a later launch's sweep.
STOP_CALL_SECS = 20
STOP_RETRY_SECS = 3
REAP_DEADLINE_SECS = 120
STOP_EXEC = os.path.join(os.path.dirname(os.path.realpath(__file__)), "stop-exec.sh")


def stop_once(sock_path, container, exec_id, timeout):
    """One try at tools/stop-exec.sh for exec_id in the backend: whether the
    launch is now known to be stopped."""
    try:
        return run(sock_path, container, ["sh", STOP_EXEC, exec_id], timeout)[0] == 0
    except EngineError:
        pass
    except DOCKER_ERRORS:
        return False
    # The daemon refused the exec. A backend that is gone or not running
    # runs nothing of the launch; a paused one gets tried again.
    try:
        return not engine(sock_path, "GET", "/containers/%s/json" % quote(container),
                          timeout=timeout)["State"]["Running"]
    except EngineError as e:
        return e.status == 404
    except DOCKER_ERRORS:
        return False


def stop_launch(sock_path, container, exec_id):
    """Stops the launch in the backend, trying until it succeeds or the
    deadline passes; returns whether it succeeded. stop-exec.sh signals a
    run only once, however often it runs."""
    deadline = time.monotonic() + REAP_DEADLINE_SECS
    while not stop_once(sock_path, container, exec_id, min(STOP_CALL_SECS, deadline - time.monotonic())):
        if time.monotonic() + STOP_RETRY_SECS >= deadline:
            return False
        time.sleep(STOP_RETRY_SECS)
    return True


def parse_exec(args):
    """The exec's -w <dir> and -e NAME[=value] options, then its command. A
    bare NAME takes this process's value, and is dropped when unset, as the
    docker CLI does."""
    workdir, env = "", []
    while args and args[0] in ("-w", "-e"):
        flag, value, args = args[0], args[1], args[2:]
        if flag == "-w":
            workdir = value
        elif "=" in value:
            env.append(value)
        elif value in os.environ:
            env.append(value + "=" + os.environ[value])
    return workdir, env, args


def winsize(fd):
    rows, cols = struct.unpack("hhhh", fcntl.ioctl(fd, termios.TIOCGWINSZ, b"\0" * 8))[:2]
    return rows, cols


def write_all(fd, data):
    view = memoryview(data)
    while view:
        view = view[os.write(fd, view):]


# Docker watches an attached tty exec's input for a detach sequence, ctrl-p
# ctrl-q unless told otherwise, and holds back each key that could begin it
# until the next one arrives, so slk's ctrl+p would reach it a key late.
# Detaching can't be turned off, only moved, so it moves to control keys
# slk binds to nothing and no paste carries: one of them held back costs
# nothing, and nobody types all four in a row.
DETACH_KEYS = "ctrl-^,ctrl-\\,ctrl-@,ctrl-_"


ALT_ON, ALT_OFF = b"\x1b[?1049h", b"\x1b[?1049l"


def alt_screen_after(was, data):
    """Whether the terminal is on the alternate screen once data is written
    to it: was, unless data switches it; None while nothing has switched."""
    on, off = data.rfind(ALT_ON), data.rfind(ALT_OFF)
    return was if on == off else on > off


def attach(sock_path, container, spec, launcher, stop):
    """Runs the exec spec in container over the Engine API, with this
    process's stdin/stdout as its terminal, until the exec's stream ends;
    returns the exec instance's id, and whether slk left the alternate
    screen it had entered, which it does only on its own way out. A docker
    error before the stream starts is raised.

    The terminal is in raw mode, so a Ctrl+C reaches slk as a byte, never as
    a signal here. Without a terminal on stdin (an agent shell), slk still
    gets a pty, with its stdin detached. When the pane's terminal goes away
    or the launcher exits, it stops relaying and calls stop(), which stops
    the launch; slk's exit then ends the stream."""
    tty_in = os.isatty(0)
    size = winsize(1) if os.isatty(1) else None
    instance = exec_create(sock_path, container, dict(
        spec, AttachStdin=tty_in, AttachStdout=True, AttachStderr=True, Tty=True, DetachKeys=DETACH_KEYS))

    wake_r, wake_w = os.pipe()
    signal.signal(signal.SIGWINCH, lambda *_: os.write(wake_w, b"w"))

    def watch_launcher():
        wait_for_exit(launcher)
        os.write(wake_w, b"p")
    threading.Thread(target=watch_launcher, daemon=True).start()

    # A resize is an Engine API call, which a busy docker can hold for
    # seconds, so it goes out from a thread of its own, never from the
    # relay. Each call sends the size the terminal has by then, so resizes
    # that pile up while one is in flight go out as one, the latest. A call
    # docker never answered is sent again a second later.
    resized = threading.Event()

    def send_resizes():
        while True:
            resized.wait()
            resized.clear()
            try:
                rows, cols = winsize(1)
            except OSError:
                continue
            try:
                engine(sock_path, "POST", "/exec/%s/resize?h=%d&w=%d" % (instance, rows, cols))
            except EngineError:
                pass
            except DOCKER_ERRORS:
                time.sleep(1)
                resized.set()

    saved = termios.tcgetattr(0) if tty_in else None

    def restore():
        nonlocal saved
        if saved:
            try:
                termios.tcsetattr(0, termios.TCSADRAIN, saved)
            except termios.error:
                pass
            saved = None

    try:
        if saved:
            tty.setraw(0)
        start = {"Detach": False, "Tty": True}
        if size:
            start["ConsoleSize"] = list(size)
        s, early = exec_start(sock_path, instance, start)
        s.settimeout(None)
        if size:
            threading.Thread(target=send_resizes, daemon=True).start()
        out_ok = True
        reading = tty_in
        # The last bytes seen, for a switch split across two reads.
        alt, tail = alt_screen_after(None, early), early[-(len(ALT_ON) - 1):]
        if early:
            write_all(1, early)
        with s:
            while True:
                ready, _, _ = select.select([s, wake_r] + ([0] if reading else []), [], [])
                if s in ready:
                    try:
                        data = s.recv(65536)
                    except OSError:
                        data = b""
                    if not data:
                        break
                    alt = alt_screen_after(alt, tail + data)
                    tail = (tail + data)[-(len(ALT_ON) - 1):]
                    if out_ok:
                        try:
                            write_all(1, data)
                        except OSError:
                            # The pane's terminal is gone: stop slk, drain its
                            # shutdown output into nothing.
                            out_ok = reading = False
                            stop()
                if reading and 0 in ready:
                    try:
                        data = os.read(0, 65536)
                    except OSError:
                        data = b""
                    if not data:
                        out_ok = reading = False
                        stop()
                    else:
                        try:
                            s.sendall(data)
                        except OSError:
                            break
                if wake_r in ready:
                    for event in os.read(wake_r, 64):
                        if event == ord("w") and out_ok and size:
                            resized.set()
                        elif event == ord("p"):
                            # The launcher is gone and its shell owns the
                            # terminal again.
                            restore()
                            out_ok = reading = False
                            stop()
    finally:
        restore()
    return instance, alt is False


def exit_code(sock_path, instance, timeout=CALL_SECS):
    """The exec instance's exit code once docker has it (the stream can end
    a moment before), or None while it still runs a second on."""
    for _ in range(10):
        info = engine(sock_path, "GET", "/exec/%s/json" % instance, timeout=timeout)
        if not info["Running"]:
            return info["ExitCode"]
        time.sleep(0.1)
    return None


# How long docker gets to say how slk exited once slk quit on its own.
QUIT_CALL_SECS = 3


def how_it_ended(sock_path, where, container, started, instance, quit_itself):
    """slk's exit code once the exec's stream ended without a stop from
    here, or why the session was lost instead. When slk quit on its own
    (quit_itself: it left the alternate screen), docker gets
    QUIT_CALL_SECS per call, and when it doesn't answer, that quit is
    trusted as a clean one, exit code 0: docker going quiet after slk has
    gone loses nothing."""
    timeout = QUIT_CALL_SECS if quit_itself else CALL_SECS
    try:
        code = exit_code(sock_path, instance, timeout)
        # A backend going down kills slk with it, and docker reports the
        # backend down a moment later (a restart, back up a moment after
        # that), so a killed or still running slk gets a second's watch.
        down = None
        checks = 10 if code is None or code > 128 else 1
        for i in range(checks):
            state = engine(sock_path, "GET", "/containers/%s/json" % quote(container), timeout=timeout)["State"]
            if state["StartedAt"] != started:
                down = "restarted"
                break
            if not state["Running"]:
                down = "stopped"
            if i + 1 < checks:
                time.sleep(0.1)
        if down:
            return None, "%s %s" % (container, down)
        if code is None:
            return None, "its connection to docker dropped"
        return code, None
    except EngineError as e:
        if e.status == 404:
            return None, "%s was removed" % container
        return None, "docker failed: %s" % e
    except DOCKER_ERRORS as e:
        if quit_itself:
            return 0, None
        return None, "docker stopped answering at %s: %s" % (where, getattr(e, "strerror", None) or e)


# What slk turns on (alt screen, cell-motion mouse in SGR encoding, focus
# reports, bracketed paste, keyboard enhancements, a hidden cursor) and
# undoes on its way out, undone here when it was cut off instead.
TERMINAL_RESET = (b"\x1b[?1049l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1004l\x1b[?2004l"
                  b"\x1b[<u\x1b[>4;0m\x1b[0m\x1b[?25h")


def relaunch_command():
    role = ["%s=%s" % (k, os.environ[k]) for k in ("SLK_ROLE", "SLK_STATE_VOL") if os.environ.get(k)]
    return " ".join(role + [os.path.join(os.path.dirname(os.path.realpath(__file__)), "run-docker.sh")])


def main():
    launcher = int(sys.argv[1])
    container, exec_id, launch_dir = sys.argv[2:5]
    args = sys.argv[5:]
    split = args.index("--")
    bridges, (workdir, env, cmd) = args[:split], parse_exec(args[split + 1:])
    os.setsid()
    for sig in (signal.SIGHUP, signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, signal.SIG_IGN)
    sock_path, where = docker_socket()
    threading.Thread(target=sweep_launch_dirs, daemon=True,
                     args=(sock_path, container, os.path.dirname(launch_dir), exec_id)).start()
    for arg in bridges:
        if arg.startswith("herdr="):
            server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            server.bind(("127.0.0.1", 0))
            server.listen()
            threading.Thread(target=serve_herdr, args=(server, arg[len("herdr="):]), daemon=True).start()
            env += ["HERDR_ENV=1", "SLK_HERDR_ADDR=host.docker.internal:%d" % server.getsockname()[1]]
            env += ["%s=%s" % (k, os.environ[k]) for k in ("HERDR_PANE_ID", "HERDR_TAB_ID", "HERDR_WORKSPACE_ID")
                    if os.environ.get(k)]
        elif arg == "clipboard":
            server = HTTPServer(("127.0.0.1", 0), ClipboardHandler)
            threading.Thread(target=server.serve_forever, daemon=True).start()
            env.append("SLK_CLIPBOARD_ADDR=host.docker.internal:%d" % server.server_port)
        elif arg == "anthropic-token":
            server = HTTPServer(("127.0.0.1", 0), AnthropicTokenHandler)
            server.tokens = AnthropicToken()
            # Fetched now, while slk starts, so slk's first ask finds it ready.
            threading.Thread(target=server.tokens.warm, daemon=True).start()
            threading.Thread(target=server.serve_forever, daemon=True).start()
            env.append("SLK_ANTHROPIC_TOKEN_ADDR=host.docker.internal:%d" % server.server_port)
        elif arg == "spool":
            threading.Thread(target=watch_spool, args=(launch_dir,), daemon=True).start()
        else:
            sys.exit("host-bridge: unknown bridge %r" % arg)

    # Once: stop_launch SIGINTs slk, which ends the exec's stream.
    stopper = []
    stopped = []

    def stop():
        if not stopper:
            stopper.append(threading.Thread(target=lambda: stopped.append(stop_launch(sock_path, container, exec_id)),
                                            daemon=True))
            stopper[0].start()

    try:
        started = engine(sock_path, "GET", "/containers/%s/json" % quote(container))["State"]["StartedAt"]
        instance, quit_itself = attach(sock_path, container, {"Cmd": cmd, "Env": env, "WorkingDir": workdir}, launcher, stop)
    except EngineError as e:
        sys.exit("slk: docker can't start slk in %s: %s" % (container, e))
    except DOCKER_ERRORS as e:
        sys.exit("slk: can't reach docker at %s: %s" % (where, getattr(e, "strerror", None) or e))

    if stopper:
        # The pane or its launcher went away: nobody is left to tell.
        stopper[0].join()
        ok = stopped[0]
        try:
            code = exit_code(sock_path, instance)
        except DOCKER_ERRORS:
            code = None
    else:
        code, lost = how_it_ended(sock_path, where, container, started, instance, quit_itself)
        if lost:
            try:
                if os.isatty(1):
                    os.write(1, TERMINAL_RESET)
                sys.stderr.write("slk: this session ended because %s. Relaunch with: %s\n" % (lost, relaunch_command()))
            except OSError:
                pass
        # Whatever the launch left running goes too: slk itself when only
        # its stream dropped, else nothing, which returns at once.
        ok = stop_once(sock_path, container, exec_id, QUIT_CALL_SECS if quit_itself else STOP_CALL_SECS)
        if not ok:
            # Docker may be back in seconds with slk still running in it:
            # the pane gets its prompt back now, and a child keeps trying.
            if os.fork():
                os._exit(1 if code is None else code)
            devnull = os.open(os.devnull, os.O_RDWR)
            for fd in (0, 1, 2):
                os.dup2(devnull, fd)
            ok = stop_launch(sock_path, container, exec_id)
    if code is None:
        code = 1
    if not ok:
        # The backend never answered: a later launch's sweep removes the
        # launch directory once it is sure nothing runs there.
        sys.exit(code)
    log_dir = os.environ.get("SLK_LOG_DIR")
    if log_dir:
        try:
            shutil.move(os.path.join(launch_dir, "slk-debug.log"), os.path.join(log_dir, "slk-debug.log"))
        except OSError:
            pass
    shutil.rmtree(launch_dir, ignore_errors=True)
    sys.exit(code)


if __name__ == "__main__":
    main()
