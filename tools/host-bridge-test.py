#!/usr/bin/env python3
# Tests host-bridge.py's anthropic-token endpoint against a fake `ant` on
# PATH, and its link-open spool against a fake `open`. Run by hand:
# python3 tools/host-bridge-test.py
import importlib.util
import json
import os
import shutil
import subprocess
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.request
from http.server import HTTPServer

here = os.path.dirname(os.path.realpath(__file__))
spec = importlib.util.spec_from_file_location("host_bridge", os.path.join(here, "host-bridge.py"))
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)

# The fake ant logs each run's argv and whether a key or token reached it,
# then answers as $FAKE_ANT_MODE says.
FAKE_ANT = """#!/bin/sh
echo "$* key=${ANTHROPIC_API_KEY:+set} token=${ANTHROPIC_AUTH_TOKEN:+set}" >>"$FAKE_ANT_LOG"
case "$FAKE_ANT_MODE" in
  ok) printf '{"version":"1.0","type":"oauth_token","access_token":"fake-access","expires_at":%s,"refresh_token":"fake-refresh"}\\n' "$FAKE_ANT_EXPIRES" ;;
  logged-out) echo 'not logged in (profile "default"): no credentials' >&2; exit 1 ;;
  api-key) echo '{"version":"1.0","type":"api_key"}' ;;
esac
"""


class AnthropicTokenEndpoint(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        ant = os.path.join(self.dir, "ant")
        with open(ant, "w") as f:
            f.write(FAKE_ANT)
        os.chmod(ant, 0o755)
        self.log = os.path.join(self.dir, "log")
        self.saved_env = dict(os.environ)
        os.environ.update(PATH=self.dir + ":/usr/bin:/bin", FAKE_ANT_LOG=self.log, FAKE_ANT_MODE="ok",
                          FAKE_ANT_EXPIRES=str(int(time.time()) + 3600),
                          ANTHROPIC_API_KEY="host-key", ANTHROPIC_AUTH_TOKEN="host-token")
        self.server = HTTPServer(("127.0.0.1", 0), bridge.AnthropicTokenHandler)
        self.server.tokens = bridge.AnthropicToken()
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        os.environ.clear()
        os.environ.update(self.saved_env)

    def get(self, path="/token"):
        try:
            with urllib.request.urlopen("http://127.0.0.1:%d%s" % (self.server.server_port, path)) as resp:
                return resp.status, resp.read().decode()
        except urllib.error.HTTPError as e:
            return e.code, e.read().decode()

    def runs(self):
        try:
            with open(self.log) as f:
                return f.read().splitlines()
        except FileNotFoundError:
            return []

    def test_serves_the_access_token_alone(self):
        status, body = self.get()
        self.assertEqual(status, 200)
        self.assertEqual(json.loads(body), {"access_token": "fake-access",
                                            "expires_at": int(os.environ["FAKE_ANT_EXPIRES"])})
        self.assertNotIn("fake-refresh", body)

    def test_ant_runs_without_the_hosts_key_or_token(self):
        self.get()
        self.assertEqual(self.runs(), ["auth print-credentials key= token="])

    def test_reuses_a_token_until_near_expiry(self):
        self.get()
        self.get()
        self.assertEqual(len(self.runs()), 1, "a token with an hour left: ant ran again")
        os.environ["FAKE_ANT_EXPIRES"] = str(int(time.time()) + bridge.TOKEN_REFRESH_SECS - 5)
        self.server.tokens = bridge.AnthropicToken()
        self.get()
        self.get()
        self.assertEqual(len(self.runs()), 3, "a token inside the refresh window: ant did not run again")

    def test_a_relogin_takes_effect_after_the_reuse_window(self):
        self.get()
        self.server.tokens.fetched_at -= bridge.TOKEN_REUSE_SECS
        self.get()
        self.assertEqual(len(self.runs()), 2, "past the reuse window: ant did not run again")

    def test_not_logged_in(self):
        os.environ["FAKE_ANT_MODE"] = "logged-out"
        status, body = self.get()
        self.assertEqual(status, 503)
        self.assertIn("not logged in", body)
        self.assertIn("ant auth login", body)
        # A failure is not kept: logging in fixes the next ask.
        os.environ["FAKE_ANT_MODE"] = "ok"
        self.assertEqual(self.get()[0], 200)

    def test_profile_without_an_access_token(self):
        os.environ["FAKE_ANT_MODE"] = "api-key"
        status, body = self.get()
        self.assertEqual(status, 503)
        self.assertIn("no OAuth access token", body)

    def test_ant_missing(self):
        os.environ["PATH"] = "/usr/bin:/bin"
        status, body = self.get()
        self.assertEqual(status, 503)
        self.assertIn("ant is not installed", body)

    def test_other_paths_404(self):
        self.assertEqual(self.get("/")[0], 404)


class SpoolWatch(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.spool = os.path.join(self.dir, "spool")
        os.mkdir(self.spool)
        self.log = os.path.join(self.dir, "opened")
        fake_open = os.path.join(self.dir, "open")
        with open(fake_open, "w") as f:
            f.write('#!/bin/sh\necho "$1" >>"%s"\n' % self.log)
        os.chmod(fake_open, 0o755)
        self.saved_path = os.environ["PATH"]
        os.environ["PATH"] = self.dir + ":" + self.saved_path
        self.watcher = threading.Thread(target=bridge.watch_spool, args=(self.spool,), daemon=True)
        self.watcher.start()

    def tearDown(self):
        os.environ["PATH"] = self.saved_path
        shutil.rmtree(self.dir, ignore_errors=True)

    def opened(self, url, within):
        """Whether url reached `open` within that many seconds."""
        deadline = time.monotonic() + within
        while time.monotonic() < deadline:
            try:
                with open(self.log) as f:
                    if url in f.read().splitlines():
                        return True
            except FileNotFoundError:
                pass
            time.sleep(0.005)
        return False

    def test_opens_each_url_spool_open_drops(self):
        for url in ("https://example.com/a", "https://example.com/b"):
            subprocess.run([os.path.join(here, "spool-open"), url], check=True,
                           env=dict(os.environ, SLK_SPOOL_DIR=self.spool))
            self.assertTrue(self.opened(url, within=2), url + " was never opened")
        self.assertEqual(os.listdir(self.spool), [])

    def test_ends_once_the_spool_is_gone(self):
        os.rmdir(self.spool)
        self.watcher.join(1)
        self.assertFalse(self.watcher.is_alive())


if __name__ == "__main__":
    unittest.main()
