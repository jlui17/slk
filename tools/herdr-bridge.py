#!/usr/bin/env python3
# TCP -> unix-socket forwarder for the herdr socket. Docker Desktop does not
# forward bind-mounted unix sockets across the VM boundary, so the container
# reaches herdr via host.docker.internal:<port> and this bridge relays to the
# real socket on the host. Stdlib only: python3 is Apple-signed, so Santa's
# binary-killing policy doesn't apply.
import os
import signal
import socket
import sys
import threading
import time


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


def handle(tcp_conn, unix_path):
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


def watch_parent(pid):
    while True:
        time.sleep(3)
        try:
            os.kill(pid, 0)
        except OSError:
            os.kill(os.getpid(), signal.SIGTERM)
            return


def main():
    unix_path = sys.argv[1]
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.bind(("127.0.0.1", 0))
    server.listen()
    print(server.getsockname()[1], flush=True)

    if len(sys.argv) > 2:
        pid = int(sys.argv[2])
        threading.Thread(target=watch_parent, args=(pid,), daemon=True).start()

    while True:
        conn, _ = server.accept()
        threading.Thread(target=handle, args=(conn, unix_path), daemon=True).start()


if __name__ == "__main__":
    main()
