package mcp

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"readonly-k8s-mcp/internal/audit"
	"readonly-k8s-mcp/internal/config"
	"readonly-k8s-mcp/internal/runner"
	readonlytools "readonly-k8s-mcp/internal/tools"
)

func TestMCPRequiresBearerToken(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestInitializeAndToolsList(t *testing.T) {
	server := newTestServer(t)

	initResp := callRPC(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	if initResp["error"] != nil {
		t.Fatalf("initialize returned error: %#v", initResp["error"])
	}

	toolsResp := callRPC(t, server, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	if toolsResp["error"] != nil {
		t.Fatalf("tools/list returned error: %#v", toolsResp["error"])
	}
	result := toolsResp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Config{
		Server: config.ServerConfig{Listen: "127.0.0.1:0"},
		Auth:   config.AuthConfig{BearerToken: "test-token"},
		Audit:  config.AuditConfig{Path: filepath.Join(t.TempDir(), "audit.jsonl")},
		Nodes:  []config.NodeConfig{{Name: "master-1", Local: true}},
		Commands: map[string]string{
			"uptime": "/usr/bin/uptime",
		},
	}
	auditLog, err := audit.NewFileLogger(cfg.Audit.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = auditLog.Close() })
	r := runner.New(cfg, auditLog)
	registry := readonlytools.NewRegistry(cfg, r)
	return NewServer(cfg, registry, auditLog, slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func callRPC(t *testing.T, server *Server, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}
