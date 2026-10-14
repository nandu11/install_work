# readonly-k8s-mcp 内网部署包

这个目录用于内网部署 readonly-k8s-mcp。推荐整目录复制到跳板机，例如 /opt/readonly-k8s-mcp-deploy，然后在跳板机按下面的手动命令逐步安装。

## 目录内容

- readonly-k8s-mcp: MCP 服务端二进制
- config.example.json: 服务配置模板
- readonly-k8s-mcp.service: systemd 服务文件
- codex-ro.sudoers: master/worker 节点只读 sudoers 示例
- kubernetes-readonly-rbac.yaml: Kubernetes 只读 RBAC 示例
- config.k8s-cluster-example.json: 按当前集群节点清单生成的配置模板

## 部署目标

服务部署在跳板机，对 VPN 内网暴露 http://JUMP_HOST_IP:8787/mcp。本机 Codex 通过 Bearer Token 访问 MCP。MCP 在跳板机上使用低权限 kubeconfig 读取 Kubernetes 信息，并通过 codex-ro SSH 到 master/worker 节点执行固定只读命令。

如果跳板机本身也是 K8S 节点，例如使用 k8s-node01 作为跳板机，也可以部署。影响是：k8s-node01 重启、断网、被维护或资源打满时，MCP 服务也会不可用；同时 MCP 服务会占用这台节点少量 CPU、内存和 8787 端口。节点采集配置里仍建议把 k8s-node01 写成 local=false，让 MCP 像访问其他节点一样通过 SSH 登录 codex-ro@10.0.0.20 采集，这样所有节点都走同一套权限模型。不要为了 k8s-node01 改成 local=true，除非你额外给 readonly-k8s-mcp 服务用户配置了对应 sudoers 白名单。

## 前置条件

跳板机需要具备：

1. systemd
2. kubectl，并且运行 readonly-k8s-mcp 的系统用户能读取低权限 kubeconfig
3. ssh 客户端
4. 能从跳板机 SSH 到 master/worker 节点
5. VPN 或防火墙限制 8787 端口只对内网开放

master/worker 节点需要创建 codex-ro 用户，并安装 sudoers 白名单。不要使用 root SSH 作为 MCP 采集账号。

## 运行时依赖

readonly-k8s-mcp 启动和正常采集依赖下面这些条件：

- systemd：负责托管 readonly-k8s-mcp 服务。
- /opt/readonly-k8s-mcp/readonly-k8s-mcp：服务端二进制。
- /opt/readonly-k8s-mcp/config.json：服务配置，包含监听地址、Bearer Token、kubectl 路径、SSH 私钥路径、known_hosts、节点清单和命令白名单。
- /opt/readonly-k8s-mcp/logs/audit.jsonl：审计日志文件，服务运行用户需要能写入这个目录。
- kubectl：用于读取 Kubernetes API。运行服务的系统用户需要能读取 /opt/readonly-k8s-mcp/kubeconfig，且 Kubernetes 权限应限制为 get/list/watch。
- ssh：用于从跳板机连接 master/worker 节点。
- /opt/readonly-k8s-mcp/id_ed25519：MCP 服务访问 worker 的 SSH 私钥，文件权限建议 0600，属主建议 readonly-k8s-mcp。
- /opt/readonly-k8s-mcp/known_hosts：worker 节点主机指纹，建议开启 StrictHostKeyChecking=yes。
- codex-ro：每台节点上的非 root 采集账号。
- /etc/sudoers.d/codex-ro：每台节点上的 sudoers 白名单，只允许固定只读命令。
- 节点只读命令：hostnamectl、uptime、free、df、lsblk、ps、ss、systemctl status、journalctl、crictl 或 docker。
- 网络路径：本机 Codex 能通过 VPN 访问跳板机的 MCP 端口；跳板机能访问 Kubernetes API Server；跳板机能 SSH 到每台 master/worker 的 22 端口。

## 部署前检查

先在准备部署 MCP 的跳板机上做检查，确认这台主机具备运行条件。

1. 确认系统和 systemd 可用：

       uname -a
       command -v systemctl
       systemctl --version

2. 确认基础命令存在：

       command -v sudo
       command -v ssh
       command -v journalctl
       command -v ss

3. 确认 kubectl 存在。低权限 kubeconfig 创建完成后，再用 /opt/readonly-k8s-mcp/kubeconfig 验证能读取集群：

       command -v kubectl
       kubectl version --client
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig get nodes -o wide
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig get pods -A

4. 确认低权限 kubeconfig 只有只读能力。下面命令应该返回 yes：

       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i get nodes
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i list nodes
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i get pods -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i list pods -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i get events -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i list events -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i watch pods -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i watch events -A

   下面命令应该返回 no：

       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i delete pods -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i patch deployments -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i create pods -A
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig auth can-i delete nodes

   watch 属于只读观察权限，用于持续观察资源变化；它不允许创建、修改或删除资源。

5. 确认 8787 端口没有被占用。如果你计划改用其他端口，把命令里的 8787 换掉：

       ss -ltnp | grep ':8787' || true

6. 确认跳板机能解析并访问每台 master/worker。把 NODE_IP 换成真实地址：

       ping -c 3 NODE_IP
       nc -vz NODE_IP 22

   如果系统没有 nc，可以只做 SSH 测试。

7. 确认跳板机能用 MCP 专用 SSH 私钥登录 master/worker。把路径和 IP 换成真实值：

       ssh -i /opt/readonly-k8s-mcp/id_ed25519 \
         -o BatchMode=yes \
         -o StrictHostKeyChecking=yes \
         codex-ro@NODE_IP /usr/bin/hostnamectl

8. 在每台节点确认只读命令路径存在。sudoers 和 config.json 必须使用这些真实路径：

       command -v hostnamectl
       command -v uptime
       command -v free
       command -v df
       command -v lsblk
       command -v ps
       command -v ss
       command -v systemctl
       command -v journalctl
       command -v crictl || true
       command -v docker || true

9. 在每台节点确认 codex-ro 的 sudoers 只允许白名单命令。安装 sudoers 后执行：

       sudo visudo -cf /etc/sudoers.d/codex-ro
       sudo -l -U codex-ro

   检查输出里不应该出现 ALL 或通用 shell 权限，例如 /bin/sh、/bin/bash、vim、tee、rm、mv、chmod、chown、systemctl restart、kubectl apply、kubectl delete。

10. 确认部署目录里的二进制能在跳板机上运行：

       ./readonly-k8s-mcp -h

   能打印 usage 或参数说明即可。不要在配置还没改好时直接启动服务。

## 创建 K8S 低权限账号

最终放在跳板机上的 /opt/readonly-k8s-mcp/kubeconfig，不是手写出来的，也不要用集群管理员 kubeconfig 直接放过去。它应该由一个 Kubernetes ServiceAccount 的 token 生成。

这一段分两台机器：

- 管理机：能用 kubectl 管理集群、具备创建 ServiceAccount、ClusterRole、ClusterRoleBinding 的权限。它可以是 K8S master，也可以是你的运维电脑，关键是 kubectl 当前上下文有集群管理员权限。
- 跳板机：最终运行 readonly-k8s-mcp 的机器。它不需要保存集群管理员 kubeconfig，只保存下面生成的低权限 kubeconfig。

生成流程是：在管理机创建低权限 ServiceAccount -> 在管理机导出 readonly-k8s-mcp.kubeconfig 文件 -> 通过 scp 传到跳板机的 /opt/readonly-k8s-mcp/kubeconfig。

1. 在管理机确认当前 kubectl 是管理员上下文：

       kubectl config current-context
       kubectl auth can-i create serviceaccount -n kube-system
       kubectl auth can-i create clusterrole
       kubectl auth can-i create clusterrolebinding

   上面三个 can-i 应该返回 yes。如果返回 no，说明这台机器当前 kubeconfig 权限不够，不能执行账号创建。

2. 在管理机进入部署包目录。这个目录里需要有 kubernetes-readonly-rbac.yaml：

       cd /path/to/intranet-deploy
       ls kubernetes-readonly-rbac.yaml

   如果部署包只在跳板机上，可以先从本机复制这一个 YAML 到管理机：

       scp release/intranet-deploy/kubernetes-readonly-rbac.yaml root@ADMIN_HOST:/tmp/kubernetes-readonly-rbac.yaml

   然后在管理机进入对应目录：

       cd /tmp

3. 在管理机应用只读 RBAC：

       kubectl apply -f kubernetes-readonly-rbac.yaml

4. 在管理机确认 ServiceAccount 和权限已创建：

       kubectl -n kube-system get serviceaccount readonly-k8s-mcp
       kubectl get clusterrole readonly-k8s-mcp
       kubectl get clusterrolebinding readonly-k8s-mcp

5. 在管理机创建 token。Kubernetes 1.24 及以上推荐用 create token：

       TOKEN="$(kubectl -n kube-system create token readonly-k8s-mcp --duration=8760h)"

   如果你的集群不支持 --duration，执行：

       TOKEN="$(kubectl -n kube-system create token readonly-k8s-mcp)"

   这个 token 只属于 readonly-k8s-mcp ServiceAccount，不是管理员 token。

6. 在管理机取当前集群 API Server 地址和 CA：

       CLUSTER_NAME="$(kubectl config view --minify -o jsonpath='{.contexts[0].context.cluster}')"
       SERVER="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
       kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d > ca.crt

7. 在管理机生成低权限 kubeconfig。生成后的文件名是当前目录下的 readonly-k8s-mcp.kubeconfig：

       kubectl config set-cluster readonly-k8s-mcp-cluster \
         --server="$SERVER" \
         --certificate-authority=ca.crt \
         --embed-certs=true \
         --kubeconfig=readonly-k8s-mcp.kubeconfig
        
       kubectl config set-credentials readonly-k8s-mcp \
         --token="$TOKEN" \
         --kubeconfig=readonly-k8s-mcp.kubeconfig
        
       kubectl config set-context readonly-k8s-mcp \
         --cluster=readonly-k8s-mcp-cluster \
         --user=readonly-k8s-mcp \
         --kubeconfig=readonly-k8s-mcp.kubeconfig
        
       kubectl config use-context readonly-k8s-mcp \
         --kubeconfig=readonly-k8s-mcp.kubeconfig

8. 在管理机确认文件已经生成，并检查里面不包含管理员用户：

       ls -l readonly-k8s-mcp.kubeconfig
       kubectl config get-contexts --kubeconfig=readonly-k8s-mcp.kubeconfig

9. 在管理机用这个低权限 kubeconfig 验证权限。下面命令应该成功：

       kubectl --kubeconfig=readonly-k8s-mcp.kubeconfig get nodes
       kubectl --kubeconfig=readonly-k8s-mcp.kubeconfig get pods -A
       kubectl --kubeconfig=readonly-k8s-mcp.kubeconfig get events -A
       kubectl --kubeconfig=readonly-k8s-mcp.kubeconfig auth can-i watch pods -A
       kubectl --kubeconfig=readonly-k8s-mcp.kubeconfig auth can-i watch events -A

   下面命令应该失败：

       kubectl --kubeconfig=readonly-k8s-mcp.kubeconfig delete pod SOME_POD -n SOME_NAMESPACE
       kubectl --kubeconfig=readonly-k8s-mcp.kubeconfig patch deployment SOME_DEPLOYMENT -n SOME_NAMESPACE -p '{}'

10. 在跳板机创建服务用户和配置目录。如果前面的安装步骤已经创建过，可以跳过这两条：

       ssh root@JUMP_HOST_IP 'useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin readonly-k8s-mcp || true'
       ssh root@JUMP_HOST_IP 'install -d -m 0750 -o root -g readonly-k8s-mcp /opt/readonly-k8s-mcp'

11. 从管理机把低权限 kubeconfig 传到跳板机：

       scp readonly-k8s-mcp.kubeconfig root@JUMP_HOST_IP:/opt/readonly-k8s-mcp/kubeconfig

   传输完成后，跳板机上的最终文件就是：

       /opt/readonly-k8s-mcp/kubeconfig

   readonly-k8s-mcp 服务启动后会按 config.json 里的 kubernetes.kubeconfig 读取它。

12. 在跳板机设置权限：

       sudo chown root:readonly-k8s-mcp /opt/readonly-k8s-mcp/kubeconfig
       sudo chmod 0640 /opt/readonly-k8s-mcp/kubeconfig

13. 在跳板机用低权限 kubeconfig 再验证一次：

       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig get nodes
       sudo -u readonly-k8s-mcp kubectl --kubeconfig=/opt/readonly-k8s-mcp/kubeconfig get pods -A

## 安装步骤

1. 复制部署包到跳板机：

       scp -r release/intranet-deploy root@JUMP_HOST_IP:/opt/readonly-k8s-mcp-deploy

2. 在跳板机进入部署目录：

       cd /opt/readonly-k8s-mcp-deploy

3. 创建系统用户：

       sudo useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin readonly-k8s-mcp || true

4. 创建配置和日志目录：

       sudo install -d -m 0750 -o root -g readonly-k8s-mcp /opt/readonly-k8s-mcp
       sudo install -d -m 0750 -o readonly-k8s-mcp -g readonly-k8s-mcp /opt/readonly-k8s-mcp/logs

5. 安装二进制和 systemd 服务文件：

       sudo install -m 0755 -o root -g root ./readonly-k8s-mcp /opt/readonly-k8s-mcp/readonly-k8s-mcp
       sudo install -m 0644 -o root -g root ./readonly-k8s-mcp.service /etc/systemd/system/readonly-k8s-mcp.service

6. 安装配置模板。首次安装时执行；如果已经有生产配置，不要覆盖。

   通用模板：

       sudo install -m 0640 -o root -g readonly-k8s-mcp ./config.example.json /opt/readonly-k8s-mcp/config.json

   如果部署的是你当前这套 8 节点集群，可以改用已经填好节点清单的模板：

       sudo install -m 0640 -o root -g readonly-k8s-mcp ./config.k8s-cluster-example.json /opt/readonly-k8s-mcp/config.json

7. 编辑配置：

       sudo vi /opt/readonly-k8s-mcp/config.json

   Bearer Token 可以在跳板机上生成：

       openssl rand -hex 32
       
       REPLACE_WITH_LONG_RANDOM_TOKEN

   必须修改：

   - auth.bearer_token: 换成长随机 token
   - server.listen: 推荐绑定跳板机 VPN 网卡 IP，例如 10.8.0.10:8787
   - kubernetes.kubectl: 跳板机上 kubectl 的真实路径
   - kubernetes.kubeconfig: /opt/readonly-k8s-mcp/kubeconfig
   - kubernetes.context: 需要指定 context 时填写
   - ssh.identity_file: readonly-k8s-mcp 服务访问 worker 的私钥路径
   - ssh.known_hosts_file: SSH known_hosts 文件路径
   - nodes: 填所有 master/worker 的 name、host、local。MCP 不部署在 K8S 节点上时，通常所有节点都写 local=false。

   当前集群节点清单已经写入 config.k8s-cluster-example.json：

       k8s-master01 10.0.0.20
       k8s-node01   10.0.0.21

8. 在跳板机准备 SSH 材料。

   如果还没有 MCP 专用 SSH key，在跳板机生成一对新的 key：

       sudo ssh-keygen -t ed25519 -f /opt/readonly-k8s-mcp/id_ed25519 -N '' -C readonly-k8s-mcp
       sudo chown readonly-k8s-mcp:readonly-k8s-mcp /opt/readonly-k8s-mcp/id_ed25519
       sudo chmod 0600 /opt/readonly-k8s-mcp/id_ed25519
       sudo chmod 0644 /opt/readonly-k8s-mcp/id_ed25519.pub

   如果你已有专用私钥文件，可以直接安装到应用目录：

       sudo install -m 0600 -o readonly-k8s-mcp -g readonly-k8s-mcp ./id_ed25519 /opt/readonly-k8s-mcp/id_ed25519

   打印公钥，下一步要写入每台 K8S 节点的 /home/codex-ro/.ssh/authorized_keys：

       sudo cat /opt/readonly-k8s-mcp/id_ed25519.pub

   把所有 K8S 节点 host key 写入 known_hosts：

       for ip in 10.0.0.20 10.0.0.21; do
         ssh-keyscan -H "$ip"
       done | sudo tee /opt/readonly-k8s-mcp/known_hosts >/dev/null
       sudo chown root:readonly-k8s-mcp /opt/readonly-k8s-mcp/known_hosts
       sudo chmod 0640 /opt/readonly-k8s-mcp/known_hosts

   这条命令只在跳板机执行一次，不需要到每台节点执行。它做的是：从当前 K8S 节点的 22 端口读取 SSH 服务端公钥指纹，然后写入 /opt/readonly-k8s-mcp/known_hosts。它不会登录节点，不需要节点密码，也不会修改任何节点文件。

   known_hosts 的作用是让 MCP 后续 SSH 到节点时校验目标主机身份。因为配置里启用了 StrictHostKeyChecking=yes，如果某台节点没有写入 known_hosts，MCP 连接那台节点时会失败，通常会看到 Host key verification failed 或 No ED25519 host key is known 之类的错误。其他已经写入 known_hosts 的节点不受影响。

   如果某台节点当时不通、22 端口没开、IP 写错、SSH 服务没启动，ssh-keyscan 可能不会为它写入记录。可以单独补扫那一台：

       ssh-keyscan -H 10.0.0.21 | sudo tee -a /opt/readonly-k8s-mcp/known_hosts >/dev/null

   补扫后可以检查 known_hosts 大概有多少行：

       sudo wc -l /opt/readonly-k8s-mcp/known_hosts

   如果节点重装系统或 SSH host key 变化，旧记录会导致 SSH 校验失败。处理方式是删除该节点旧记录后重新 ssh-keyscan。生产环境建议先人工确认这台机器确实是预期节点，再更新 known_hosts。

9. 在每台 K8S 节点创建 codex-ro。

   这一步是在 master/worker 节点上做。codex-ro 是节点采集账号：允许 SSH 登录，但 sudo 只能执行固定只读命令。

   不需要先把 codex-ro.sudoers 复制到节点。按下面方式操作：

   第一步，在跳板机执行，复制输出的整行公钥：

       sudo cat /opt/readonly-k8s-mcp/id_ed25519.pub

   输出会类似这样：

       ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx readonly-k8s-mcp

   第二步，登录到每台 K8S 节点，例如 k8s-master01、k8s-node01、k8s-node02。然后复制下面整段命令执行，它会在当前目录生成 setup-codex-ro-node.sh 并执行。注意把最后一行命令里的 REPLACE_WITH_PUBLIC_KEY 换成刚才复制的整行公钥：

       cat > setup-codex-ro-node.sh <<'EOF'
       #!/usr/bin/env bash
       set -euo pipefail
       pubkey="$1"
       
       id codex-ro >/dev/null 2>&1 || useradd --system --create-home --home-dir /home/codex-ro --shell /bin/bash codex-ro
       passwd -l codex-ro >/dev/null || true
       
       install -d -m 0700 -o codex-ro -g codex-ro /home/codex-ro/.ssh
       echo "$pubkey" > /home/codex-ro/.ssh/authorized_keys
       chown codex-ro:codex-ro /home/codex-ro/.ssh/authorized_keys
       chmod 0600 /home/codex-ro/.ssh/authorized_keys
       
       cat >/etc/sudoers.d/codex-ro <<'SUDOERS'
       Defaults:codex-ro !requiretty
       
       Cmnd_Alias CODEX_RO_SYSTEM = \
         /usr/bin/hostnamectl, \
         /usr/bin/uptime, \
         /usr/bin/free -m, \
         /usr/bin/df -hT -x tmpfs -x devtmpfs, \
         /usr/bin/lsblk -f, \
         /usr/bin/ps aux --sort=-%mem, \
         /usr/bin/ss -lntup, \
         /usr/bin/systemctl status kubelet --no-pager -l, \
         /usr/bin/systemctl status containerd --no-pager -l, \
         /usr/bin/systemctl status docker --no-pager -l, \
         /usr/bin/crictl ps -a, \
         /usr/bin/docker ps -a --no-trunc, \
         /usr/bin/journalctl -u kubelet --no-pager -n *, \
         /usr/bin/journalctl -u containerd --no-pager -n *, \
         /usr/bin/journalctl -u docker --no-pager -n *, \
         /usr/bin/journalctl -u sshd --no-pager -n *
       
       codex-ro ALL=(root) NOPASSWD: CODEX_RO_SYSTEM
       SUDOERS
       
       chmod 0440 /etc/sudoers.d/codex-ro
       visudo -cf /etc/sudoers.d/codex-ro
       EOF
       chmod +x setup-codex-ro-node.sh
       sudo ./setup-codex-ro-node.sh 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPNYHXlNjTFIxK5CYTrzQwx9MYQ4sEAOw82n4nFW2Cjn readonly-k8s-mcp'

10. 验证节点 SSH 和 sudo 白名单。

   在每台节点检查 sudoers：

       sudo -l -U codex-ro

   在跳板机测试每个节点。把 NODE_IP 换成真实 IP：

       ssh -i /opt/readonly-k8s-mcp/id_ed25519 \
         -o BatchMode=yes \
         -o StrictHostKeyChecking=yes \
         codex-ro@NODE_IP sudo -n /usr/bin/hostnamectl

   sudo -l -U codex-ro 输出里不应该出现 ALL、/bin/sh、/bin/bash、rm、mv、chmod、chown、systemctl restart、kubectl apply、kubectl delete。sudoers 里的命令路径必须和 /opt/readonly-k8s-mcp/config.json 里的 commands 保持一致。

11. 重新加载 systemd：

       sudo systemctl daemon-reload

12. 启动服务：

       sudo systemctl enable readonly-k8s-mcp
       sudo systemctl restart readonly-k8s-mcp

13. 查看服务状态：

       sudo systemctl --no-pager --full status readonly-k8s-mcp
       sudo journalctl -u readonly-k8s-mcp --no-pager -n 80

14. 停止服务时执行：

       sudo systemctl stop readonly-k8s-mcp

15. 在本机 Codex 配置 MCP：

       codex mcp add readonlyK8s --url http://JUMP_HOST_IP:8787/mcp

   然后在 ~/.codex/config.toml 里加入 header：

       [mcp_servers.readonlyK8s]
       url = "http://10.0.0.20:8787/mcp"
       headers = { Authorization = "Bearer REPLACE_WITH_LONG_RANDOM_TOKEN" }

## 验证

在 VPN 内机器上执行：

       curl -i http://JUMP_HOST_IP:8787/healthz

认证测试：

       curl -s http://JUMP_HOST_IP:8787/mcp \
         -H 'Authorization: Bearer REPLACE_WITH_LONG_RANDOM_TOKEN' \
         -H 'Content-Type: application/json' \
         -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'

预期返回包含 serverInfo.name = readonly-k8s-mcp。

## 审计日志

默认审计日志路径：

       /opt/readonly-k8s-mcp/logs/audit.jsonl

每条记录包含时间、请求 ID、远端地址、工具名、目标节点、参数摘要、是否允许、是否成功、耗时、输出大小和错误信息。

## 升级

把新的 readonly-k8s-mcp 二进制复制到部署目录后手动更新：

       sudo install -m 0755 -o root -g root ./readonly-k8s-mcp /opt/readonly-k8s-mcp/readonly-k8s-mcp
       sudo systemctl daemon-reload
       sudo systemctl restart readonly-k8s-mcp

不要覆盖已经存在的 /opt/readonly-k8s-mcp/config.json，除非你明确要替换生产配置。

## 回滚

如果升级前保留了旧二进制，可以复制回 /opt/readonly-k8s-mcp/readonly-k8s-mcp，然后重启：

       sudo systemctl restart readonly-k8s-mcp

如果需要停止服务：

       sudo systemctl stop readonly-k8s-mcp
