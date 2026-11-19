#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
test -x bin/ladder-agent
test -f bin/host-network.json
adapter_destination="$HOME/.local/bin/ladder-agent"
if test -e "$adapter_destination"; then
  go version -m "$adapter_destination" | grep -q 'github.com/dmethi/ladder-agent/cmd/ladder-agent' || {
    printf '%s\n' 'Refusing to replace an unrelated ladder-agent executable.' >&2
    exit 1
  }
fi
mkdir -p "$HOME/.local/bin"
install -m 0755 bin/ladder-agent "$adapter_destination"
install -m 0600 bin/host-network.json "$HOME/.local/bin/host-network.json"
python3 - "$adapter_destination" <<'PY'
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys

adapter = sys.argv[1]
for agent, probe, add in [
    ('codex', ['codex', 'mcp', 'get', 'ladder-agent', '--json'],
     ['codex', 'mcp', 'add', 'ladder-agent', '--', adapter, 'mcp']),
    ('claude', ['claude', 'mcp', 'get', 'ladder-agent'],
     ['claude', 'mcp', 'add', '--scope', 'user', '--transport', 'stdio', 'ladder-agent', '--', adapter, 'mcp']),
]:
    if not shutil.which(agent):
        continue
    existing = subprocess.run(probe, capture_output=True, text=True, timeout=15)
    if existing.returncode == 0:
        if adapter not in existing.stdout:
            raise SystemExit('Refusing to replace an unrelated ' + agent + ' MCP entry.')
        print(agent + ': existing Ladder Agent entry retained')
    else:
        subprocess.run(add, check=True, timeout=20)

launch_agents = Path.home() / 'Library/LaunchAgents'
logs = Path.home() / 'Library/Logs/LadderAgent'
launch_agents.mkdir(parents=True, exist_ok=True)
logs.mkdir(parents=True, exist_ok=True)
path = launch_agents / 'com.dmethi.ladder-agent.plist'
arguments = ['/opt/homebrew/bin/limactl', 'start', '--tty=false', 'ladder-agent']
if path.exists():
    existing = plistlib.loads(path.read_bytes())
    if existing.get('ProgramArguments') != arguments:
        raise SystemExit('Refusing to replace an unrelated LaunchAgent.')
path.write_bytes(plistlib.dumps({
    'Label': 'com.dmethi.ladder-agent',
    'ProgramArguments': arguments,
    'RunAtLoad': True,
    'StandardOutPath': str(logs / 'vm.log'),
    'StandardErrorPath': str(logs / 'vm.log'),
    'EnvironmentVariables': {'PATH': '/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin'},
}))
domain = 'gui/' + str(os.getuid())
subprocess.run(['launchctl', 'bootout', domain + '/com.dmethi.ladder-agent'], capture_output=True)
subprocess.run(['launchctl', 'bootstrap', domain, str(path)], check=True)
print('Installed CLI, agent MCP entries, and VM startup at login.')
PY
