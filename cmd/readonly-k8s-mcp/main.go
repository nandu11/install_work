package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"readonly-k8s-mcp/internal/audit"
	"readonly-k8s-mcp/internal/config"
	"readonly-k8s-mcp/internal/mcp"
	"readonly-k8s-mcp/internal/runner"
	readonlytools "readonly-k8s-mcp/internal/tools"
)

func main() {
	configPath := flag.String("config", "/etc/readonly-k8s-mcp/config.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail("load config", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	auditLog, err := audit.NewFileLogger(cfg.Audit.Path)
	if err != nil {
		fail("open audit log", err)
	}
	defer auditLog.Close()

	commandRunner := runner.New(cfg, auditLog)
	registry := readonlytools.NewRegistry(cfg, commandRunner)
	server := mcp.NewServer(cfg, registry, auditLog, logger)

	httpServer := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("readonly k8s mcp listening", "addr", cfg.Server.Listen)
		errs <- httpServer.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		logger.Info("shutdown requested", "signal", sig.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			fail("serve", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		fail("shutdown", err)
	}
}

func fail(action string, err error) {
	_, _ = fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
