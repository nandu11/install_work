# Git-managed contents

This repository tracks the readonly Kubernetes MCP service source and deploy templates.

## Include

- Go source:
  - `cmd/`
  - `internal/`
  - `go.mod`
- Config templates:
  - `config.example.json`
  - `release/intranet-deploy/config.example.json`
  - `release/intranet-deploy/config.k8s-cluster-example.json`
- Deployment templates and scripts:
  - `deploy/`
  - `release/intranet-deploy/*.service`
  - `release/intranet-deploy/*.yaml`
  - `release/intranet-deploy/*.sudoers`
  - `scripts/`
- Documentation and specs:
  - `README.md`
  - `docs/`
  - `specs/`

## Exclude

Do not commit generated binaries, kubeconfig files, MCP tokens, SSH private keys, known_hosts files, logs, audit output, or local handoff notes.

The deploy host currently stores generated runtime files under `/opt/readonly-k8s-mcp`; those files are operational state, not source.
