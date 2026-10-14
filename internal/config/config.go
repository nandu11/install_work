package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server     ServerConfig      `json:"server"`
	Auth       AuthConfig        `json:"auth"`
	Audit      AuditConfig       `json:"audit"`
	Kubernetes KubernetesConfig  `json:"kubernetes"`
	SSH        SSHConfig         `json:"ssh"`
	Nodes      []NodeConfig      `json:"nodes"`
	Limits     LimitsConfig      `json:"limits"`
	Commands   map[string]string `json:"commands"`
}

type ServerConfig struct {
	Listen string `json:"listen"`
}

type AuthConfig struct {
	BearerToken string `json:"bearer_token"`
}

type AuditConfig struct {
	Path string `json:"path"`
}

type KubernetesConfig struct {
	Kubectl    string `json:"kubectl"`
	Kubeconfig string `json:"kubeconfig"`
	Context    string `json:"context"`
}

type SSHConfig struct {
	Binary                string `json:"binary"`
	User                  string `json:"user"`
	IdentityFile          string `json:"identity_file"`
	ConnectTimeout        string `json:"connect_timeout"`
	StrictHostKeyChecking string `json:"strict_host_key_checking"`
	KnownHostsFile        string `json:"known_hosts_file"`
}

type NodeConfig struct {
	Name  string `json:"name"`
	Host  string `json:"host"`
	Local bool   `json:"local"`
}

type LimitsConfig struct {
	CommandTimeout string `json:"command_timeout"`
	MaxOutputBytes int    `json:"max_output_bytes"`
	MaxLogLines    int    `json:"max_log_lines"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = "127.0.0.1:8787"
	}
	if c.Audit.Path == "" {
		c.Audit.Path = "/opt/readonly-k8s-mcp/logs/audit.jsonl"
	}
	if c.Kubernetes.Kubectl == "" {
		c.Kubernetes.Kubectl = "kubectl"
	}
	if c.SSH.Binary == "" {
		c.SSH.Binary = "ssh"
	}
	if c.SSH.User == "" {
		c.SSH.User = "codex-ro"
	}
	if c.SSH.ConnectTimeout == "" {
		c.SSH.ConnectTimeout = "5s"
	}
	if c.SSH.StrictHostKeyChecking == "" {
		c.SSH.StrictHostKeyChecking = "yes"
	}
	if c.Limits.CommandTimeout == "" {
		c.Limits.CommandTimeout = "15s"
	}
	if c.Limits.MaxOutputBytes == 0 {
		c.Limits.MaxOutputBytes = 200000
	}
	if c.Limits.MaxLogLines == 0 {
		c.Limits.MaxLogLines = 200
	}
	if c.Commands == nil {
		c.Commands = DefaultCommands()
	}
}

func (c Config) Validate() error {
	if c.Auth.BearerToken == "" {
		return errors.New("auth.bearer_token is required")
	}
	if c.Auth.BearerToken == "CHANGE_ME" {
		return errors.New("auth.bearer_token must be changed from the example value")
	}
	if _, err := time.ParseDuration(c.Limits.CommandTimeout); err != nil {
		return fmt.Errorf("limits.command_timeout: %w", err)
	}
	if _, err := time.ParseDuration(c.SSH.ConnectTimeout); err != nil {
		return fmt.Errorf("ssh.connect_timeout: %w", err)
	}
	seen := map[string]bool{}
	for _, node := range c.Nodes {
		if node.Name == "" {
			return errors.New("nodes[].name is required")
		}
		if strings.ContainsAny(node.Name, "/\\\x00") {
			return fmt.Errorf("node %q contains an invalid name character", node.Name)
		}
		if seen[node.Name] {
			return fmt.Errorf("duplicate node name %q", node.Name)
		}
		seen[node.Name] = true
		if !node.Local && node.Host == "" {
			return fmt.Errorf("node %q host is required unless local=true", node.Name)
		}
	}
	for name, command := range c.Commands {
		if name == "" || command == "" {
			return errors.New("commands map cannot contain empty names or commands")
		}
		if strings.ContainsAny(command, "\n\r;|&<>$`") {
			return fmt.Errorf("command %q contains a forbidden shell metacharacter", name)
		}
	}
	return nil
}

func DefaultCommands() map[string]string {
	return map[string]string{
		"hostnamectl":       "/usr/bin/hostnamectl",
		"uptime":            "/usr/bin/uptime",
		"memory":            "/usr/bin/free -m",
		"filesystems":       "/usr/bin/df -hT -x tmpfs -x devtmpfs",
		"block_devices":     "/usr/bin/lsblk -f",
		"process_snapshot":  "/usr/bin/ps aux --sort=-%mem",
		"listening_ports":   "/usr/bin/ss -lntup",
		"kubelet_status":    "/usr/bin/systemctl status kubelet --no-pager -l",
		"containerd_status": "/usr/bin/systemctl status containerd --no-pager -l",
		"docker_status":     "/usr/bin/systemctl status docker --no-pager -l",
		"crictl_ps":         "/usr/bin/crictl ps -a",
		"docker_ps":         "/usr/bin/docker ps -a --no-trunc",
	}
}

func (c Config) CommandTimeout() time.Duration {
	d, _ := time.ParseDuration(c.Limits.CommandTimeout)
	return d
}

func (c Config) SSHConnectTimeoutSeconds() string {
	d, _ := time.ParseDuration(c.SSH.ConnectTimeout)
	seconds := int(d.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%d", seconds)
}

func (c Config) NodeByName(name string) (NodeConfig, bool) {
	for _, node := range c.Nodes {
		if node.Name == name {
			return node, true
		}
	}
	return NodeConfig{}, false
}
