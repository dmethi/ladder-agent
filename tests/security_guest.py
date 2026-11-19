#!/usr/bin/env python3
"""Run as guest root. Canary addresses exist only on this VM's loopback device."""
import http.server
import json
import socket
import subprocess
import threading


class Canary(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"guest-only-canary")

    def log_message(self, *_):
        pass


def curl(url, worker=False, extra=()):
    command = ["curl", "--noproxy", "*", "--silent", "--show-error", "--max-time", "4",
               "--output", "/dev/null", "--write-out", "%{http_code}", *extra, url]
    if worker:
        command = ["runuser", "-u", "ladderworker", "--", *command]
    return subprocess.run(command, capture_output=True, text=True, timeout=6,
                          env={"PATH": "/usr/bin:/bin:/usr/sbin:/sbin"})


aliases = ["10.254.254.254", "100.64.254.254", "169.254.169.254", "192.0.2.1", "8.8.4.4"]
servers = []
try:
    subprocess.run(["nft", "add", "element", "inet", "ladder_agent", "host_v4", "{ 8.8.4.4 }"], check=True)
    for address in aliases:
        subprocess.run(["ip", "addr", "add", address + "/32", "dev", "lo"], check=True)
    server = http.server.ThreadingHTTPServer(("0.0.0.0", 80), Canary)
    servers.append(server)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    class V6Server(http.server.ThreadingHTTPServer):
        address_family = socket.AF_INET6
    server6 = V6Server(("::1", 80), Canary)
    servers.append(server6)
    threading.Thread(target=server6.serve_forever, daemon=True).start()
    results = []
    for address in ["127.0.0.1", *aliases, "[::1]"]:
        url = "http://" + address + "/"
        control, blocked = curl(url), curl(url, worker=True)
        assert control.returncode == 0 and control.stdout == "200", (address, "canary not reachable", control.stderr)
        assert blocked.returncode != 0 and blocked.stdout == "000", (address, "worker reached forbidden canary")
        results.append({"destination": address, "adminControl": "reachable", "worker": "blocked"})
    public_ip = socket.getaddrinfo("example.com", 443, socket.AF_INET, socket.SOCK_STREAM)[0][4][0]
    public = curl("https://example.com/", worker=True, extra=("--resolve", "example.com:443:" + public_ip))
    assert public.returncode == 0 and public.stdout != "000", ("public-web control failed", public.stderr)
    results.append({"destination": "https://example.com", "worker": "reachable", "status": public.stdout})
    forbidden_port = subprocess.run(["runuser", "-u", "ladderworker", "--", "python3", "-c",
        "import socket; socket.create_connection(('1.1.1.1', 22), timeout=2)"], capture_output=True, timeout=4)
    assert forbidden_port.returncode != 0, "worker reached arbitrary public port"
    results.append({"destination": "public TCP port 22", "worker": "blocked"})
    print(json.dumps(results, indent=2))
finally:
    for server in servers:
        server.shutdown()
        server.server_close()
    for address in aliases:
        subprocess.run(["ip", "addr", "del", address + "/32", "dev", "lo"], capture_output=True)
    subprocess.run(["nft", "delete", "element", "inet", "ladder_agent", "host_v4", "{ 8.8.4.4 }"], capture_output=True)
