# Local public-web fetch for agents

Agents on this Mac can retrieve public HTTP(S) content as bounded, inert text,
HTML strings or JSON. Preserve Ladder's crawler user-agent technique. No active
web page is served to a browser. No caller may provide credentials, headers,
commands, filesystem paths or network policy.

The fetch process is assumed exploitable. A dedicated Linux VM has no shared host
files, forwarded SSH agent or host credentials. Guest firewall rules restrict the
unprivileged worker to public-web destinations and one fixed public DNS resolver.
Lima host-loopback access is disabled. The local CLI/MCP adapter relays only fetch
requests into a fixed guest command. No web API is exposed on the Mac or LAN.

Limits: 20 seconds total, five redirects, 2 MiB decoded response, two simultaneous
requests. The VM and systemd impose additional CPU, memory and process limits.
Block private, loopback, link-local, CGNAT/tailnet, multicast and reserved address
space in IPv4 and IPv6. Resolve and validate every destination and connect to that
validated address. Redirects repeat policy checks. TLS verification stays enabled.
Exclude all connected host networks as well, including globally routable ranges.
If the host's connected-network fingerprint changes, new adapter requests fail
closed until deployment refreshes the guest firewall and installed policy.

No browser renderer, authenticated sessions, cookies, database, cache, remote rules
or arbitrary proxy tunnel. Prompt injection in returned text remains possible;
the service labels all payloads untrusted. It does not confine the calling agent.
Host/VM administrators and unknown hypervisor exploits are outside this boundary.
