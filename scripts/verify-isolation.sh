#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
limactl copy --backend=scp tests/security_guest.py ladder-agent:/tmp/ladder-security-check.py
limactl shell --workdir / ladder-agent sudo python3 /tmp/ladder-security-check.py
limactl shell --workdir / ladder-agent sudo rm /tmp/ladder-security-check.py
