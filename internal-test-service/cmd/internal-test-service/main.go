package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"example.invalid/internal-test-service/internal/app"
	"example.invalid/internal-test-service/internal/config"
	"example.invalid/internal-test-service/internal/store"
)

func main() {
	if handled, exitCode := runBackupCommand(os.Args[1:], os.Getenv, os.Stderr); handled {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := app.Healthcheck(context.Background(), envOr("D04_LISTEN_ADDRESS", ":8090")); err != nil {
			os.Exit(1)
		}
		return
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		if os.Getenv("D04_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "startup config: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "configuration unavailable")
		}
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	instance, err := app.New(ctx, cfg)
	if err != nil {
		if os.Getenv("D04_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "startup app: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "service unavailable")
		}
		os.Exit(1)
	}
	defer instance.Close()
	go func() { _ = instance.Scheduler.Run(ctx) }()
	server := &http.Server{Addr: cfg.ListenAddress, Handler: instance.Handler, ReadHeaderTimeout: 5_000_000_000, ReadTimeout: 15_000_000_000, WriteTimeout: 60_000_000_000, IdleTimeout: 60_000_000_000}
	go func() { <-ctx.Done(); _ = server.Shutdown(context.Background()) }()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "server stopped")
		os.Exit(1)
	}
}

func runBackupCommand(args []string, getenv func(string) string, stderr io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != "backup-sqlite" {
		return false, 0
	}
	if len(args) != 3 {
		fmt.Fprintln(stderr, "usage: internal-test-service backup-sqlite SOURCE DESTINATION")
		return true, 2
	}
	if err := store.BackupSQLite(context.Background(), args[1], args[2]); err != nil {
		if getenv("D04_DEBUG") == "1" {
			fmt.Fprintf(stderr, "backup sqlite: %v\n", err)
		} else {
			fmt.Fprintln(stderr, "backup unavailable")
		}
		return true, 1
	}
	return true, 0
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
