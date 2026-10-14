# readonly-k8s-mcp

Readonly MCP server for Codex to inspect a private Kubernetes cluster and Linux nodes through a VPN-only HTTP endpoint.

The service runs on a bastion or jump host, exposes POST /mcp, validates a static Bearer token, advertises read-only MCP tools, runs only fixed kubectl get and fixed OS inspection commands, and writes JSONL audit records for auth failures, tool calls, and command execution.

## Tools

- cluster_overview: nodes, namespaces, pods, workloads, services, ingresses, recent events
- k8s_get: constrained kubectl get for allowed resources
- node_list: configured master/worker inventory
- node_snapshot: one configured read-only OS command on one node
- node_journal: bounded journalctl read for kubelet/containerd/docker/sshd

## Build

    go test ./...
    go build -buildvcs=false -o dist/readonly-k8s-mcp ./cmd/readonly-k8s-mcp

## Codex config

Add the remote MCP server to Codex:

    codex mcp add readonlyK8s --url http://JUMP_HOST_IP:8787/mcp

Then add the Authorization header in ~/.codex/config.toml:

    [mcp_servers.readonlyK8s]
    url = "http://JUMP_HOST_IP:8787/mcp"
    headers = { Authorization = "Bearer REPLACE_WITH_LONG_RANDOM_TOKEN" }

OpenAI's current MCP documentation shows Codex can add streamable HTTP MCP servers with codex mcp add NAME --url URL and that the endpoint is typically /mcp.

## Deployment outline

Ready-to-copy intranet deployment files are in release/intranet-deploy. The directory contains the binary, config template, systemd unit, sudoers example, Kubernetes RBAC example, and a Chinese manual deployment guide with explicit commands.

1. Build and copy the binary to /opt/readonly-k8s-mcp/readonly-k8s-mcp on the jump host.
2. Create system user readonly-k8s-mcp.
3. Keep MCP-owned runtime files under /opt/readonly-k8s-mcp: binary, config.json, kubeconfig, id_ed25519, known_hosts, and logs/audit.jsonl.
4. Create a low-privilege Kubernetes ServiceAccount and install its kubeconfig at /opt/readonly-k8s-mcp/kubeconfig.
5. Create /opt/readonly-k8s-mcp/config.json from config.example.json, replacing token, nodes, kubeconfig path/context, SSH key path, and known hosts.
6. Install deploy/systemd/readonly-k8s-mcp.service.
7. Create codex-ro on every Kubernetes node and install a sudoers allowlist aligned with the configured commands.
8. Bind the service to a VPN-only IP or firewall the port so it is reachable only through VPN.

## Security model

The service intentionally does not expose arbitrary shell execution. Kubernetes access is constrained to read-only get/list/watch RBAC, while MCP tools only expose fixed kubectl get workflows for now. Node access is constrained to configured node names and configured command names. The SSH account should be non-root and restricted by sudoers. Audit logs are append-only JSONL at the service level; use filesystem permissions or log shipping if you need stronger tamper resistance.
