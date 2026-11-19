# Flows

## Fetch public content
- Trigger: CLI or MCP `fetch_url` with URL and text/html/json format.
- Expected: bounded payload, final URL, upstream status and `trust: untrusted`.
- Constraints: VISION.md; no local direct-fetch fallback.
- Status: works.
- Evidence: `tests/mcp_smoke.py` passed against the installed host adapter and real
  VM service, including a cold VM restart. Public HTTPS returned expected text;
  CLI and MCP returned structured results with an enforced untrusted label.

## Refuse prohibited destinations
- Trigger: local/private/reserved target, forbidden scheme/port, DNS rebinding or redirect.
- Expected: blocked result; no connection to prohibited destination.
- Constraints: actual dial uses checked IP; every DNS answer and redirect checked.
- Status: works.
- Evidence: `go test -race ./...` covers private/mixed DNS, a changed private answer
  on redirect, exact IP dialing, private redirects, invalid ports/schemes, IPv4
  mapped addresses and IPv6 reserved ranges. Real MCP refuses loopback/metadata.
  A forged stale host-network fingerprint is refused before the guest request.

## Contain compromised fetch worker
- Trigger: direct network attempts under the worker identity, bypassing URL checks.
- Expected: public web works; host/LAN/tailnet/metadata and arbitrary ports fail.
- Constraints: guest firewall outside worker control; no host mounts or credentials.
- Status: works.
- Evidence: `scripts/verify-isolation.sh` passed before and after cold restart.
  Guest administrator reached canaries; worker identity could not reach loopback,
  private, CGNAT, metadata, reserved, IPv6 loopback or an excluded public canary.
  Public HTTPS succeeded; public port 22 was denied. `tests/host_boundary.py`
  proved the Mac loopback canary unreachable from the VM, no host mounts, and
  no SSH-agent forwarding. This bypasses application validation deliberately.

## Fail within resource limits
- Trigger: oversized/compressed response, slow server or excess concurrency.
- Expected: typed failure without partial payload presented as complete.
- Constraints: decoded size, total deadline, concurrency and guest resource limits.
- Status: works.
- Evidence: race-enabled tests reject raw/gzip oversized payloads and partial error
  responses, enforce two concurrent requests, and exercise an actual slow HTTP
  server with a parent deadline. Live systemd settings confirm worker UID,
  empty capability set, NoNewPrivileges, protected home/system and 256 MiB ceiling.

## Scope of verification

Checked 2026-11-19. Local CI passed; OSV found no known advisory matches in the
11-module dependency set. Codex and Claude Code MCP entries point to the installed
adapter. User LaunchAgent successfully starts the VM; cold restart verified guest
firewall/service ordering and preserved isolation. A macOS logout/reboot itself
was not performed. No claim of immunity to guest-kernel or hypervisor exploits,
agent prompt injection, or all publisher anti-bot mechanisms.
