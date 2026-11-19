#!/usr/bin/env python3
"""Prove host-loopback blocking with a host-only canary, not a household service."""
import http.server
import json
import subprocess
import threading
import urllib.request

hits = []


class Canary(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        hits.append(1)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"host-canary")

    def log_message(self, *_):
        pass


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Canary)
threading.Thread(target=server.serve_forever, daemon=True).start()
try:
    port = server.server_port
    with urllib.request.urlopen("http://127.0.0.1:" + str(port), timeout=2) as response:
        assert response.read() == b"host-canary"
    probe = subprocess.run(["limactl", "shell", "--workdir", "/", "ladder-agent", "python3", "-c",
        "import socket; socket.create_connection(('192.168.5.2', " + str(port) + "), timeout=2)"],
        capture_output=True, timeout=5)
    assert probe.returncode != 0 and len(hits) == 1, "guest reached host-loopback canary"
    config = json.loads(subprocess.check_output(["limactl", "list", "ladder-agent", "--json"]))["config"]
    assert config["plain"] and not config.get("mounts") and not config["ssh"]["forwardAgent"]
    types = subprocess.check_output(["limactl", "shell", "--workdir", "/", "ladder-agent",
                                     "findmnt", "-rn", "-o", "FSTYPE"], text=True).splitlines()
    assert not set(types) & {"virtiofs", "9p", "fuse.sshfs"}, types
    print(json.dumps({"hostCanary": "reachable from host; blocked from guest", "hostMounts": "absent", "agentForwarding": "disabled"}))
finally:
    server.shutdown()
    server.server_close()
