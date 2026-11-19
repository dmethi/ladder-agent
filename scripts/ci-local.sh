#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
test -z "$(gofmt -l cmd internal)"
go vet ./...
go test -race ./...
go build ./cmd/ladder-agent
rm -f ladder-agent
python3 -m py_compile tests/security_guest.py tests/host_boundary.py tests/mcp_smoke.py
limactl validate deploy/lima.yaml
