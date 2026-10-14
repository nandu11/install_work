# 使用 readonlyK8s MCP 启动 Codex

这份文档说明如何启动 Codex，让新的 Codex 会话加载 readonlyK8s MCP 服务，并出现下面这些工具：

- cluster_overview
- k8s_get
- node_list
- node_snapshot
- node_journal

## 服务地址

readonlyK8s 是一个远程 streamable HTTP MCP 服务。Codex 需要访问：

~~~text
http://10.0.0.20:8787/mcp
~~~

服务端要求每个 MCP 请求都带 Bearer Token：

~~~text
Authorization: Bearer <token>
~~~

## 推荐的 Codex MCP 配置

现在不需要手动提前配置 Codex MCP。启动脚本已经内置了 MCP 服务地址，并会从环境变量或本机 token 文件读取 token，然后在启动 Codex 时通过 -c 注入下面两项：

~~~text
mcp_servers.readonlyK8s.url = "http://10.0.0.20:8787/mcp"
mcp_servers.readonlyK8s.http_headers.Authorization = "Bearer <token>"
~~~

可以用下面命令检查脚本注入后的配置：

~~~bash
./scripts/start-codex-readonly-k8s mcp get readonlyK8s
~~~

预期形态类似：

~~~text
readonlyK8s
  enabled: true
  transport: streamable_http
  url: http://10.0.0.20:8787/mcp
  http_headers: Authorization=*****
~~~

## 用脚本启动 Codex

在仓库根目录执行：

~~~bash
./scripts/start-codex-readonly-k8s
~~~

这个脚本会自己准备 READONLY_K8S_MCP_TOKEN、MCP 服务 URL 和 Codex MCP 配置，然后执行：

~~~bash
exec codex \
  --no-daemon \
  --enable mcp_2026_07_28 \
  -c "mcp_servers.\${server_name}.url=\"\${server_url}\"" \
  -c "mcp_servers.\${server_name}.bearer_token_env_var=\"\"" \
  -c "mcp_servers.\${server_name}.http_headers.Authorization=\"\${auth_header}\"" \
  "$@"
~~~

脚本后面的参数会原样传给 Codex。例如：

~~~bash
./scripts/start-codex-readonly-k8s mcp get readonlyK8s
~~~

## Token 来源

启动脚本会按下面顺序寻找 token：

1. 当前环境变量 READONLY_K8S_MCP_TOKEN。
2. READONLY_K8S_MCP_TOKEN_FILE 指定的文件。
3. 默认本机 token 文件 /tmp/readonly-k8s-mcp-10.0.0.20.token。

### 使用环境变量

~~~bash
export READONLY_K8S_MCP_TOKEN='REPLACE_WITH_LONG_RANDOM_TOKEN'
./scripts/start-codex-readonly-k8s
~~~

### 使用 token 文件

文件里只写原始 token，不要写 Bearer。

~~~bash
READONLY_K8S_MCP_TOKEN_FILE=/path/to/readonly-k8s-mcp.token \
  ./scripts/start-codex-readonly-k8s
~~~

### 使用默认 token 文件

当前部署已把 10.0.0.20 上新生成的 MCP token 同步到本机默认 token 文件。直接执行下面命令即可启动 Codex：

~~~bash
./scripts/start-codex-readonly-k8s
~~~

如果以后需要临时覆盖 token，优先使用 READONLY_K8S_MCP_TOKEN 或 READONLY_K8S_MCP_TOKEN_FILE。

## 服务连通性验证

下面这些命令需要在启动 Codex 的同一台机器上执行。

先确认健康检查能访问：

~~~bash
curl -i http://10.0.0.20:8787/healthz
~~~

再确认不带认证的 MCP 请求会被拒绝：

~~~bash
curl -i \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  http://10.0.0.20:8787/mcp
~~~

预期结果是 HTTP 401。

然后验证带认证的 MCP 初始化：

~~~bash
curl -i \
  -X POST \
  -H "Authorization: Bearer $READONLY_K8S_MCP_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  http://10.0.0.20:8787/mcp
~~~

预期返回里包含：

~~~json
{
  "serverInfo": {
    "name": "readonly-k8s-mcp"
  }
}
~~~

## 常见问题

如果 codex mcp list 能看到 readonlyK8s，但对话里没有出现 cluster_overview 等工具，需要用启动脚本重新打开一个新的 Codex 会话。MCP 工具目录是在会话初始化时加载的。

如果 codex mcp get readonlyK8s 没有显示 streamable_http，重新执行 codex mcp add readonlyK8s --url ... 创建 HTTP MCP 配置。

如果 codex mcp get readonlyK8s 里 bearer_token_env_var 显示成一长串 token，说明配置把 token 值误写成了环境变量名。必须用启动脚本启动 Codex，脚本会清空 bearer_token_env_var，并直接注入 http_headers.Authorization。

如果认证已经正确但工具仍不出现，检查 mcp_2026_07_28 feature 是否为 true。启动脚本已经带上 --enable mcp_2026_07_28。

如果仍然没有工具，确认是否绕开了旧的共享 app-server daemon。启动脚本已经带上 --no-daemon，避免旧 daemon 忽略新 MCP 配置或 feature。

如果服务返回 401，检查 READONLY_K8S_MCP_TOKEN 是否和跳板机上 MCP 服务配置里的 auth.bearer_token 一致。

如果服务无法连接，检查 VPN、跳板机防火墙，以及 readonly-k8s-mcp 是否正在监听 10.0.0.20:8787。

如果从 IDE 启动 Codex，要确认 IDE 进程能继承 READONLY_K8S_MCP_TOKEN。最简单的验证方式是在终端里先 export 环境变量，再从同一个终端启动 IDE。
