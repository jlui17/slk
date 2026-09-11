#!/usr/bin/env python3
# Host-side clipboard server for slk running in docker. The container cannot
# see the macOS clipboard, so Ctrl+V smart paste asks this bridge at
# host.docker.internal:<port> for whatever image or text the host clipboard
# holds. Reads go through osascript, which is Apple-signed, so Santa's
# binary-killing policy doesn't apply. Stdlib only, like herdr-bridge.py.
import os
import signal
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer


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
                sys.stderr.write(f"clipboard-bridge: osascript: {result.stderr.decode(errors='replace').strip()}\n")
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


class Handler(BaseHTTPRequestHandler):
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


def watch_parent(pid):
    while True:
        time.sleep(3)
        try:
            os.kill(pid, 0)
        except OSError:
            os.kill(os.getpid(), signal.SIGTERM)
            return


def main():
    server = HTTPServer(("127.0.0.1", 0), Handler)
    print(server.server_port, flush=True)
    threading.Thread(target=watch_parent, args=(int(sys.argv[1]),), daemon=True).start()
    server.serve_forever()


if __name__ == "__main__":
    main()
