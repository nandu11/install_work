# readonly-k8s-mcp

只读 MCP 服务，供 Codex 通过仅限 VPN 访问的 HTTP 端点，检查私有 Kubernetes 集群和 Linux 节点。

该服务运行在堡垒机（跳板机）上，对外暴露 POST /mcp 接口，使用静态 Bearer Token 进行鉴权，只提供只读 MCP 工具，仅执行固定的 kubectl get 命令和固定的操作系统检查命令，并为鉴权失败、工具调用和命令执行写入 JSONL 审计日志。

## 工具列表

- cluster_overview：节点、命名空间、Pod、工作负载、Service、Ingress、近期事件
- k8s_get：受约束的 kubectl get，仅允许访问白名单资源
- node_list：配置中的 master/worker 节点清单
- node_snapshot：在指定节点上执行一条配置好的只读 OS 命令
- node_journal：对 kubelet/containerd/docker/sshd 进行有限量的 journalctl 日志读取

## 构建

    go test ./...
    go build -buildvcs=false -o dist/readonly-k8s-mcp ./cmd/readonly-k8s-mcp

## Codex 配置

将远程 MCP 服务添加到 Codex：

    codex mcp add readonlyK8s --url http://JUMP_HOST_IP:8787/mcp

然后在 ~/.codex/config.toml 中添加 Authorization 请求头：

    [mcp_servers.readonlyK8s]
    url = "http://JUMP_HOST_IP:8787/mcp"
    headers = { Authorization = "Bearer REPLACE_WITH_LONG_RANDOM_TOKEN" }

根据 OpenAI 当前的 MCP 文档，Codex 可以通过 codex mcp add NAME --url URL 添加 streamable HTTP 类型的 MCP 服务，端点通常为 /mcp。

## 部署概要

可直接拷贝使用的内网部署文件位于 release/intranet-deploy 目录，其中包含二进制文件、配置模板、systemd unit、sudoers 示例、Kubernetes RBAC 示例，以及一份附带明确命令的中文手动部署指南。

1. 构建二进制文件，并拷贝到跳板机的 /opt/readonly-k8s-mcp/readonly-k8s-mcp。
2. 创建系统用户 readonly-k8s-mcp。
3. MCP 相关的运行时文件统一放在 /opt/readonly-k8s-mcp 目录下：二进制文件、config.json、kubeconfig、id_ed25519、known_hosts 以及 logs/audit.jsonl。
4. 创建一个低权限的 Kubernetes ServiceAccount，并将其 kubeconfig 安装到 /opt/readonly-k8s-mcp/kubeconfig。
5. 参照 config.example.json 创建 /opt/readonly-k8s-mcp/config.json，替换其中的 token、节点列表、kubeconfig 路径/context、SSH 密钥路径和 known_hosts。
6. 安装 deploy/systemd/readonly-k8s-mcp.service。
7. 在每个 Kubernetes 节点上创建 codex-ro 用户，并安装与所配置命令相对应的 sudoers 白名单。
8. 将服务绑定到仅 VPN 可达的 IP 地址，或通过防火墙限制端口，使其只能通过 VPN 访问。

## 安全模型

本服务刻意不提供任意 shell 执行能力。Kubernetes 访问被限制为只读的 get/list/watch RBAC 权限，目前 MCP 工具也只暴露固定的 kubectl get 流程。节点访问被限制在配置好的节点名称和命令名称范围内。SSH 账号应为非 root 用户，并通过 sudoers 加以限制。审计日志为服务层面的追加式 JSONL 文件；如需更强的防篡改能力，请配合文件系统权限或日志外送（log shipping）方案使用。
