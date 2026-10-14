# Feature Specification: Readonly K8S MCP

## User Story

As an operator using local Codex over VPN, I want a jump-host MCP service to collect read-only Kubernetes and Linux node information, so Codex can help diagnose production state without receiving root access or mutation capability.

## Requirements

- The MCP server exposes a streamable HTTP endpoint at /mcp.
- The endpoint requires a static Bearer token for every MCP request.
- The server provides no arbitrary command execution tool.
- Kubernetes tools only perform read-only kubectl get operations for an allowlisted resource set.
- Kubernetes access uses a dedicated low-privilege get/list/watch kubeconfig, not a cluster-admin kubeconfig.
- Node tools only target configured nodes.
- Worker node OS inspection uses the non-root codex-ro SSH account.
- Privileged node reads rely on sudoers allowlisting, not root SSH.
- Every auth failure, rejected tool call, and executed command is written to JSONL audit logs.
- Tool output is bounded by configurable size and timeout limits.

## Non-Goals

- Public internet exposure.
- Write, delete, restart, drain, cordon, patch, apply, exec, cp, or port-forward operations.
- Full multi-user OAuth/RBAC in the first version.
- DaemonSet-based node agents.
