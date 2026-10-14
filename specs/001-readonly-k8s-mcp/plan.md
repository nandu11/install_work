# Implementation Plan

## Architecture

Local Codex connects over VPN to http://JUMP_HOST_IP:8787/mcp. The Go service runs on a jump host under systemd, validates a Bearer token, handles MCP JSON-RPC methods, and maps tool calls to fixed command functions.

Kubernetes reads call kubectl get with fixed resource allowlists through a dedicated low-privilege get/list/watch kubeconfig. Node reads call SSH to codex-ro@master or codex-ro@worker and then a sudoers-allowlisted read command.

## Technology

- Go 1.26
- Standard library only
- systemd deployment
- JSON config and JSONL audit log

## Validation

- go test ./...
- Manual MCP initialize and tools/list HTTP calls
- Representative rejected unauthenticated request
