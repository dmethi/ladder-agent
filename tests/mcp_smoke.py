#!/usr/bin/env python3
"""Exercise the real host adapter, guest relay, service and network fetch."""
import json
import selectors
import subprocess
import sys
from pathlib import Path

process = subprocess.Popen([sys.argv[1], "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
selector = selectors.DefaultSelector()
selector.register(process.stdout, selectors.EVENT_READ)


def send(message):
    process.stdin.write(json.dumps(message) + "\n")
    process.stdin.flush()


def receive(request_id):
    while True:
        if not selector.select(35):
            raise AssertionError("MCP response timed out")
        line = process.stdout.readline()
        assert line, "MCP server exited"
        message = json.loads(line)
        if message.get("id") == request_id:
            assert "error" not in message, message
            return message["result"]


try:
    send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
        "protocolVersion": "2025-11-25", "capabilities": {},
        "clientInfo": {"name": "ladder-security-smoke", "version": "1"}}})
    receive(1)
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    send({"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
    tools = receive(2)["tools"]
    assert [tool["name"] for tool in tools] == ["fetch_url"], tools
    for request_id, url, expected in [(3, "https://example.com/", "ok"),
                                      (4, "http://127.0.0.1/", "blocked"),
                                      (5, "http://169.254.169.254/", "blocked")]:
        send({"jsonrpc": "2.0", "id": request_id, "method": "tools/call", "params": {
            "name": "fetch_url", "arguments": {"url": url, "format": "text"}}})
        result = receive(request_id)["structuredContent"]
        assert result["outcome"] == expected and result["trust"] == "untrusted", result
        if expected == "ok":
            assert "Example Domain" in result["body"], result
        print(json.dumps({"url": url, "outcome": result["outcome"], "trust": result["trust"]}))
    policy_path = Path(sys.argv[1]).resolve().parent / "host-network.json"
    saved = policy_path.read_bytes()
    try:
        policy = json.loads(saved)
        policy["fingerprint"] = "stale-policy"
        policy_path.write_text(json.dumps(policy))
        send({"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {
            "name": "fetch_url", "arguments": {"url": "https://example.com/", "format": "text"}}})
        result = receive(6)["structuredContent"]
        assert result["outcome"] == "blocked" and "network changed" in result["reason"], result
        print(json.dumps({"staleHostPolicy": "blocked before guest request"}))
    finally:
        policy_path.write_bytes(saved)
finally:
    process.stdin.close()
    try:
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
    selector.close()
