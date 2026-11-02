#!/usr/bin/env python3
# Host-side bridges for one slk container, in one process so a launch pays
# Python's startup once. The container reaches each at
# host.docker.internal:<port>:
#
#   herdr      TCP -> unix-socket forwarder for the herdr socket. Docker
#              Desktop does not forward bind-mounted unix sockets across the
#              VM boundary, so this relays to the real socket on the host.
#   clipboard  HTTP server for Ctrl+V smart paste: the container cannot see
#              the macOS clipboard, so it asks here for whatever image or
#              text the host clipboard holds, read through osascript.
#
# Usage: host-bridge.py <parent-pid> [herdr=<socket-path>] [clipboard]
# Prints one line naming each bridge's port ("herdr=<port> clipboard=<port>")
# once all are listening, then exits when <parent-pid> does.
#
# Stdlib only: python3 and osascript are Apple-signed, so Santa's
# binary-killing policy doesn't apply.
import os
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


def main():
    parent = int(sys.argv[1])
    ports = []
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
        else:
            sys.exit("host-bridge: unknown bridge %r" % arg)
    print(" ".join(ports), flush=True)

    # Exit with the launcher, even if its EXIT trap never fires.
    while True:
        time.sleep(3)
        try:
            os.kill(parent, 0)
        except OSError:
            return


if __name__ == "__main__":
    main()
