package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"readonly-k8s-mcp/internal/audit"
	"readonly-k8s-mcp/internal/config"
)

type Result struct {
	Stdout      string
	Stderr      string
	ExitCode    int
	Duration    time.Duration
	OutputBytes int
}

type Runner struct {
	cfg   config.Config
	audit *audit.FileLogger
}

func New(cfg config.Config, auditLog *audit.FileLogger) *Runner {
	return &Runner{cfg: cfg, audit: auditLog}
}

func (r *Runner) Kubectl(ctx context.Context, requestID, remoteAddr string, args ...string) (Result, error) {
	base := []string{}
	if r.cfg.Kubernetes.Kubeconfig != "" {
		base = append(base, "--kubeconfig", r.cfg.Kubernetes.Kubeconfig)
	}
	if r.cfg.Kubernetes.Context != "" {
		base = append(base, "--context", r.cfg.Kubernetes.Context)
	}
	full := append(base, args...)
	return r.run(ctx, audit.Event{
		RequestID:  requestID,
		RemoteAddr: remoteAddr,
		Method:     "command",
		Tool:       "kubectl",
		Target:     "cluster",
		Allowed:    true,
		Args:       map[string]any{"argv": append([]string{r.cfg.Kubernetes.Kubectl}, full...)},
	}, r.cfg.Kubernetes.Kubectl, full...)
}

func (r *Runner) NodeCommand(ctx context.Context, requestID, remoteAddr string, node config.NodeConfig, commandName string) (Result, error) {
	command, ok := r.cfg.Commands[commandName]
	if !ok {
		return Result{}, fmt.Errorf("unknown command %q", commandName)
	}
	argv := strings.Fields(command)
	if len(argv) == 0 {
		return Result{}, fmt.Errorf("command %q is empty", commandName)
	}
	if node.Local {
		localArgs := append([]string{"-n"}, argv...)
		return r.run(ctx, audit.Event{
			RequestID:  requestID,
			RemoteAddr: remoteAddr,
			Method:     "command",
			Tool:       commandName,
			Target:     node.Name,
			Allowed:    true,
			Args:       map[string]any{"mode": "local", "argv": append([]string{"sudo"}, localArgs...)},
		}, "sudo", localArgs...)
	}

	remoteCommand := "sudo -n " + shellJoin(argv)
	sshArgs := r.sshArgs(node.Host, remoteCommand)
	return r.run(ctx, audit.Event{
		RequestID:  requestID,
		RemoteAddr: remoteAddr,
		Method:     "command",
		Tool:       commandName,
		Target:     node.Name,
		Allowed:    true,
		Args:       map[string]any{"mode": "ssh", "host": node.Host, "argv": argv},
	}, r.cfg.SSH.Binary, sshArgs...)
}

func (r *Runner) NodeJournal(ctx context.Context, requestID, remoteAddr string, node config.NodeConfig, unit string, lines int) (Result, error) {
	if lines < 1 {
		lines = 50
	}
	if lines > r.cfg.Limits.MaxLogLines {
		lines = r.cfg.Limits.MaxLogLines
	}
	if node.Local {
		return r.run(ctx, audit.Event{
			RequestID:  requestID,
			RemoteAddr: remoteAddr,
			Method:     "command",
			Tool:       "node_journal",
			Target:     node.Name,
			Allowed:    true,
			Args:       map[string]any{"mode": "local", "unit": unit, "lines": lines},
		}, "sudo", "-n", "journalctl", "-u", unit, "--no-pager", "-n", fmt.Sprintf("%d", lines))
	}
	command := fmt.Sprintf("journalctl -u %s --no-pager -n %d", shellWord(unit), lines)
	sshArgs := r.sshArgs(node.Host, "sudo -n "+command)
	return r.run(ctx, audit.Event{
		RequestID:  requestID,
		RemoteAddr: remoteAddr,
		Method:     "command",
		Tool:       "node_journal",
		Target:     node.Name,
		Allowed:    true,
		Args:       map[string]any{"mode": "ssh", "host": node.Host, "unit": unit, "lines": lines},
	}, r.cfg.SSH.Binary, sshArgs...)
}

func (r *Runner) run(ctx context.Context, event audit.Event, name string, args ...string) (Result, error) {
	timeout := r.cfg.CommandTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr limitBuffer
	stdout.max = r.cfg.Limits.MaxOutputBytes
	stderr.max = r.cfg.Limits.MaxOutputBytes / 4
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	duration := time.Since(start)

	result := Result{
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
		Duration:    duration,
		OutputBytes: stdout.Len() + stderr.Len(),
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("command timed out after %s", timeout)
	}
	if err != nil && result.ExitCode == 0 {
		result.ExitCode = -1
	}

	event.Success = err == nil
	event.DurationMS = duration.Milliseconds()
	event.OutputBytes = result.OutputBytes
	if err != nil {
		event.Error = err.Error()
	}
	r.audit.Write(event)
	return result, err
}

func (r *Runner) sshArgs(host string, remoteCommand string) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=" + r.cfg.SSHConnectTimeoutSeconds(),
		"-o", "StrictHostKeyChecking=" + r.cfg.SSH.StrictHostKeyChecking,
	}
	if r.cfg.SSH.KnownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+r.cfg.SSH.KnownHostsFile)
	}
	if r.cfg.SSH.IdentityFile != "" {
		args = append(args, "-i", r.cfg.SSH.IdentityFile)
	}
	target := r.cfg.SSH.User + "@" + host
	return append(args, target, remoteCommand)
}

type limitBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return len(p), nil
	}
	remaining := b.max - b.Buffer.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func shellWord(s string) string {
	if s == "" {
		return "''"
	}
	return shellSingleQuote(s)
}

func shellJoin(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, shellSingleQuote(arg))
	}
	return strings.Join(quoted, " ")
}
