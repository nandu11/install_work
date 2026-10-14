package mcp

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"readonly-k8s-mcp/internal/audit"
	"readonly-k8s-mcp/internal/config"
	readonlytools "readonly-k8s-mcp/internal/tools"
)

type Server struct {
	cfg      config.Config
	tools    *readonlytools.Registry
	auditLog *audit.FileLogger
	logger   *slog.Logger
}

func NewServer(cfg config.Config, tools *readonlytools.Registry, auditLog *audit.FileLogger, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, tools: tools, auditLog: auditLog, logger: logger}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/mcp", s.handleMCP)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "readonly-k8s-mcp\n")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	_, _ = io.WriteString(w, "{\"status\":\"ok\"}")
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.setCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodDelete {
		s.setCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "MCP endpoint requires POST", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		s.auditLog.Write(audit.Event{
			RemoteAddr: r.RemoteAddr,
			Method:     "auth",
			Allowed:    false,
			Success:    false,
			Error:      "unauthorized",
		})
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	s.setCORS(w)
	w.Header().Set("content-type", "application/json")

	var req rpcRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: nil, Error: &rpcError{Code: -32700, Message: "parse error", Data: err.Error()}})
		return
	}
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}

	resp, ok := s.dispatch(r, req)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "{}")
		return
	}
	writeRPC(w, resp)
}

func (s *Server) authorized(r *http.Request) bool {
	got := strings.TrimPrefix(r.Header.Get("authorization"), "Bearer ")
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Auth.BearerToken)) == 1
}

func (s *Server) setCORS(w http.ResponseWriter) {
	w.Header().Set("access-control-allow-origin", "*")
	w.Header().Set("access-control-allow-methods", "POST, GET, DELETE, OPTIONS")
	w.Header().Set("access-control-allow-headers", "authorization, content-type, mcp-session-id")
	w.Header().Set("access-control-expose-headers", "mcp-session-id")
}

func (s *Server) dispatch(r *http.Request, req rpcRequest) (rpcResponse, bool) {
	switch req.Method {
	case "initialize":
		return ok(req.ID, map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "readonly-k8s-mcp",
				"version": "0.1.0",
			},
			"instructions": "Read-only Kubernetes and Linux node inspection tools. All tools are read-only and audited.",
		}), true
	case "notifications/initialized":
		return rpcResponse{}, false
	case "ping":
		return ok(req.ID, map[string]any{}), true
	case "tools/list":
		return ok(req.ID, map[string]any{"tools": s.tools.ToolDefinitions()}), true
	case "tools/call":
		result, err := s.callTool(r, req)
		if err != nil {
			return fail(req.ID, -32602, err.Error()), true
		}
		return ok(req.ID, result), true
	default:
		return fail(req.ID, -32601, fmt.Sprintf("method %q not found", req.Method)), true
	}
}

func (s *Server) callTool(r *http.Request, req rpcRequest) (map[string]any, error) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(req.Params) == 0 {
		return nil, errors.New("tools/call params are required")
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, errors.New("tool name is required")
	}

	requestID := fmt.Sprint(req.ID)
	output, structured, err := s.tools.Call(r.Context(), readonlytools.Call{
		Name:       params.Name,
		Arguments:  params.Arguments,
		RequestID:  requestID,
		RemoteAddr: r.RemoteAddr,
	})
	if err != nil {
		s.auditLog.Write(audit.Event{
			RequestID:  requestID,
			RemoteAddr: r.RemoteAddr,
			Method:     "tools/call",
			Tool:       params.Name,
			Allowed:    false,
			Success:    false,
			Error:      err.Error(),
		})
		return map[string]any{
			"isError": true,
			"content": []map[string]string{{
				"type": "text",
				"text": err.Error(),
			}},
		}, nil
	}

	return map[string]any{
		"isError":           false,
		"content":           []map[string]string{{"type": "text", "text": output}},
		"structuredContent": structured,
	}, nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func ok(id any, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func fail(id any, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
