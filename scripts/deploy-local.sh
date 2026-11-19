#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
command -v go >/dev/null
command -v limactl >/dev/null
mkdir -p bin
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o bin/ladder-agent-linux ./cmd/ladder-agent
go build -trimpath -o bin/ladder-agent ./cmd/ladder-agent
bin/ladder-agent network-policy > bin/host-network.json
python3 - <<'PY'
import ipaddress, json
from pathlib import Path
data = json.loads(Path('bin/host-network.json').read_text())
rules = Path('deploy/firewall.nft').read_text()
for version in (4, 6):
    networks = [ipaddress.ip_network(x) for x in data['networks'] if ipaddress.ip_network(x).version == version]
    networks = list(ipaddress.collapse_addresses(networks))
    if networks:
        rules += '\nadd element inet ladder_agent host_v' + str(version) + ' { ' + ', '.join(map(str, networks)) + ' }\n'
Path('bin/firewall.nft').write_text(rules)
Path('bin/host-network.json').chmod(0o600)
PY
if limactl list --format '{{.Name}}' 2>/dev/null | grep -qx ladder-agent; then
  limactl start --tty=false ladder-agent
else
  limactl start --name ladder-agent --tty=false deploy/lima.yaml
fi
limactl list ladder-agent --json | python3 -c '
import json, sys
x = json.load(sys.stdin)
c = x["config"]
assert c.get("plain") is True and not c.get("mounts"), "VM host sharing is enabled"
assert c["networkOpts"]["hostLoopback"] is False, "VM host-loopback access is enabled"
assert c["ssh"]["forwardAgent"] is False, "SSH agent forwarding is enabled"
assert c["hostResolver"]["enabled"] is False and c["propagateProxyEnv"] is False, "VM inherits host DNS/proxy"
assert x["sshAddress"] == "127.0.0.1", "VM ingress is not loopback-only"
'
limactl copy --backend=scp bin/ladder-agent-linux ladder-agent:/tmp/ladder-agent-new
limactl copy --backend=scp bin/firewall.nft deploy/ladder-agent.service deploy/ladder-agent-firewall.service ladder-agent:/tmp/
limactl shell --workdir / ladder-agent sudo sh -eu -c '
  install -d -m 0755 /etc/ladder-agent
  nft --check --file /tmp/firewall.nft
  install -m 0644 /tmp/firewall.nft /etc/ladder-agent/firewall.nft
  install -m 0644 /tmp/ladder-agent.service /tmp/ladder-agent-firewall.service /etc/systemd/system/
  systemctl stop ladder-agent.service 2>/dev/null || true
  install -m 0755 /tmp/ladder-agent-new /usr/local/bin/ladder-agent
  nft delete table inet ladder_agent 2>/dev/null || true
  systemctl daemon-reload
  systemctl restart ladder-agent-firewall.service
  systemctl enable ladder-agent-firewall.service ladder-agent.service
  systemctl restart ladder-agent.service
  systemctl is-active --quiet ladder-agent.service
  rm -f /tmp/ladder-agent-new /tmp/firewall.nft /tmp/ladder-agent.service /tmp/ladder-agent-firewall.service
'
printf '%s\n' 'Ladder Agent is running in the dedicated VM. Test with bin/ladder-agent fetch https://example.com/'
