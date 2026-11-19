# Ladder Agent

Read `docs/VISION.md`, `docs/FLOWS.md`, `docs/modules/_index.md`, then `SCRATCHPAD.md`.
Build authority: Studio Linear STU-143. Apply canonical skills from
`/Users/dhruv/Documents/GitHub/shared-agent-config/skills/`.

This fork replaces upstream's browser proxy with an agent-only fetch service.
Never restore upstream public HTML-serving routes, remote rules, cookie inputs,
host mounts, forwarded agents or host-network access. The worker runs as an
unprivileged user in a dedicated VM. Firewall policy is outside worker control.
Application checks and OS isolation must both hold. Returned content is untrusted.

Security changes require boundary tests and real VM verification. Run
`scripts/ci-local.sh` before opening a PR. No GitHub-hosted workflows or automatic
merge. Never claim a deployment or isolation check passed without real evidence.
