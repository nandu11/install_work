package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"readonly-k8s-mcp/internal/config"
	"readonly-k8s-mcp/internal/runner"
)

type Registry struct {
	cfg    config.Config
	runner *runner.Runner
}

type Call struct {
	Name       string
	Arguments  json.RawMessage
	RequestID  string
	RemoteAddr string
}

func NewRegistry(cfg config.Config, commandRunner *runner.Runner) *Registry {
	return &Registry{cfg: cfg, runner: commandRunner}
}

func (r *Registry) ToolDefinitions() []map[string]any {
	return []map[string]any{
		r.tool("cluster_overview", "Collect read-only Kubernetes cluster overview: nodes, namespaces, workloads, services, ingresses, recent events.", objectSchema(map[string]any{})),
		r.tool("k8s_get", "Run a constrained read-only kubectl get for an allowed Kubernetes resource.", objectSchema(map[string]any{
			"resource":  enumSchema("nodes", "namespaces", "pods", "deployments", "statefulsets", "daemonsets", "services", "ingress", "events", "persistentvolumes", "persistentvolumeclaims"),
			"namespace": map[string]any{"type": "string", "description": "Optional namespace. Use empty string for all namespaces where supported."},
			"name":      map[string]any{"type": "string", "description": "Optional resource name."},
			"selector":  map[string]any{"type": "string", "description": "Optional label selector."},
		})),
		r.tool("node_list", "List configured Linux nodes available for read-only OS inspection.", objectSchema(map[string]any{})),
		r.tool("node_snapshot", "Run one configured read-only OS inspection command on one node.", objectSchema(map[string]any{
			"node":    enumSchema(r.nodeNames()...),
			"command": enumSchema(r.commandNames()...),
		})),
		r.tool("node_journal", "Read recent journalctl lines for an allowed service unit on one node.", objectSchema(map[string]any{
			"node":  enumSchema(r.nodeNames()...),
			"unit":  enumSchema("kubelet", "containerd", "docker", "sshd"),
			"lines": map[string]any{"type": "integer", "minimum": 1, "maximum": r.cfg.Limits.MaxLogLines},
		})),
	}
}

func (r *Registry) tool(name, description string, inputSchema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": inputSchema,
		"annotations": map[string]any{
			"readOnlyHint":  true,
			"openWorldHint": false,
		},
	}
}

func (r *Registry) Call(ctx context.Context, call Call) (string, map[string]any, error) {
	switch call.Name {
	case "cluster_overview":
		return r.clusterOverview(ctx, call)
	case "k8s_get":
		return r.k8sGet(ctx, call)
	case "node_list":
		return r.nodeList()
	case "node_snapshot":
		return r.nodeSnapshot(ctx, call)
	case "node_journal":
		return r.nodeJournal(ctx, call)
	default:
		return "", nil, fmt.Errorf("unknown tool %q", call.Name)
	}
}

func (r *Registry) clusterOverview(ctx context.Context, call Call) (string, map[string]any, error) {
	sections := [][]string{
		{"get", "nodes", "-o", "wide"},
		{"get", "namespaces"},
		{"get", "pods", "-A", "-o", "wide"},
		{"get", "deployments,statefulsets,daemonsets", "-A"},
		{"get", "services,ingress", "-A"},
		{"get", "events", "-A", "--sort-by=.lastTimestamp"},
	}
	var b strings.Builder
	for _, args := range sections {
		b.WriteString("$ kubectl " + strings.Join(args, " ") + "\n")
		res, err := r.runner.Kubectl(ctx, call.RequestID, call.RemoteAddr, args...)
		b.WriteString(formatResult(res, err))
		b.WriteString("\n")
	}
	return b.String(), map[string]any{"sections": len(sections)}, nil
}

func (r *Registry) k8sGet(ctx context.Context, call Call) (string, map[string]any, error) {
	var args struct {
		Resource  string `json:"resource"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Selector  string `json:"selector"`
	}
	if err := decode(call.Arguments, &args); err != nil {
		return "", nil, err
	}
	if !in(args.Resource, "nodes", "namespaces", "pods", "deployments", "statefulsets", "daemonsets", "services", "ingress", "events", "persistentvolumes", "persistentvolumeclaims") {
		return "", nil, fmt.Errorf("resource %q is not allowed", args.Resource)
	}
	if err := validateKubeArg("namespace", args.Namespace); err != nil {
		return "", nil, err
	}
	if err := validateKubeArg("name", args.Name); err != nil {
		return "", nil, err
	}
	if err := validateKubeArg("selector", args.Selector); err != nil {
		return "", nil, err
	}
	kargs := []string{"get", args.Resource}
	if args.Name != "" {
		kargs = append(kargs, args.Name)
	}
	if args.Namespace != "" {
		kargs = append(kargs, "-n", args.Namespace)
	} else if namespaced(args.Resource) {
		kargs = append(kargs, "-A")
	}
	if args.Selector != "" {
		kargs = append(kargs, "-l", args.Selector)
	}
	kargs = append(kargs, "-o", "wide")
	res, err := r.runner.Kubectl(ctx, call.RequestID, call.RemoteAddr, kargs...)
	return "$ kubectl " + strings.Join(kargs, " ") + "\n" + formatResult(res, err), map[string]any{"argv": kargs, "exit_code": res.ExitCode}, nil
}

func (r *Registry) nodeList() (string, map[string]any, error) {
	nodes := make([]map[string]any, 0, len(r.cfg.Nodes))
	var b strings.Builder
	for _, node := range r.cfg.Nodes {
		nodes = append(nodes, map[string]any{"name": node.Name, "host": node.Host, "local": node.Local})
		mode := "ssh"
		if node.Local {
			mode = "local"
		}
		b.WriteString(fmt.Sprintf("- %s (%s) %s\n", node.Name, mode, node.Host))
	}
	return b.String(), map[string]any{"nodes": nodes, "commands": r.commandNames()}, nil
}

func (r *Registry) nodeSnapshot(ctx context.Context, call Call) (string, map[string]any, error) {
	var args struct {
		Node    string `json:"node"`
		Command string `json:"command"`
	}
	if err := decode(call.Arguments, &args); err != nil {
		return "", nil, err
	}
	node, ok := r.cfg.NodeByName(args.Node)
	if !ok {
		return "", nil, fmt.Errorf("node %q is not configured", args.Node)
	}
	if _, ok := r.cfg.Commands[args.Command]; !ok {
		return "", nil, fmt.Errorf("command %q is not configured", args.Command)
	}
	res, err := r.runner.NodeCommand(ctx, call.RequestID, call.RemoteAddr, node, args.Command)
	return formatResult(res, err), map[string]any{"node": args.Node, "command": args.Command, "exit_code": res.ExitCode}, nil
}

func (r *Registry) nodeJournal(ctx context.Context, call Call) (string, map[string]any, error) {
	var args struct {
		Node  string `json:"node"`
		Unit  string `json:"unit"`
		Lines int    `json:"lines"`
	}
	if err := decode(call.Arguments, &args); err != nil {
		return "", nil, err
	}
	if !in(args.Unit, "kubelet", "containerd", "docker", "sshd") {
		return "", nil, fmt.Errorf("unit %q is not allowed", args.Unit)
	}
	node, ok := r.cfg.NodeByName(args.Node)
	if !ok {
		return "", nil, fmt.Errorf("node %q is not configured", args.Node)
	}
	res, err := r.runner.NodeJournal(ctx, call.RequestID, call.RemoteAddr, node, args.Unit, args.Lines)
	return formatResult(res, err), map[string]any{"node": args.Node, "unit": args.Unit, "lines": args.Lines, "exit_code": res.ExitCode}, nil
}

func (r *Registry) nodeNames() []string {
	names := make([]string, 0, len(r.cfg.Nodes))
	for _, node := range r.cfg.Nodes {
		names = append(names, node.Name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) commandNames() []string {
	names := make([]string, 0, len(r.cfg.Commands))
	for name := range r.cfg.Commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func formatResult(res runner.Result, err error) string {
	var b strings.Builder
	if res.Stdout != "" {
		b.WriteString(res.Stdout)
		if !strings.HasSuffix(res.Stdout, "\n") {
			b.WriteString("\n")
		}
	}
	if res.Stderr != "" {
		b.WriteString("\n[stderr]\n")
		b.WriteString(res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			b.WriteString("\n")
		}
	}
	if err != nil {
		b.WriteString(fmt.Sprintf("\n[error] %v\n", err))
	}
	return b.String()
}

func objectSchema(properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	return schema
}

func enumSchema(values ...string) map[string]any {
	enum := make([]any, 0, len(values))
	for _, value := range values {
		enum = append(enum, value)
	}
	return map[string]any{"type": "string", "enum": enum}
}

func in(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func namespaced(resource string) bool {
	return !in(resource, "nodes", "namespaces", "persistentvolumes")
}

func validateKubeArg(label, value string) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s cannot start with '-'", label)
	}
	if strings.ContainsAny(value, "\x00\n\r\t ") {
		return fmt.Errorf("%s cannot contain whitespace or control characters", label)
	}
	return nil
}
