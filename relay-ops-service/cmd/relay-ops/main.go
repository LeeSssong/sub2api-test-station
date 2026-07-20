package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.invalid/relay-ops-service/internal/app"
	"example.invalid/relay-ops-service/internal/config"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatal("relay-ops configuration unavailable")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	application, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal("relay-ops startup failed")
	}
	defer application.Close()
	commandRuntime, err := app.ConfigureFeishuCommandsForStore(cfg, application.Store, application.Handler)
	if err != nil {
		log.Fatal("relay-ops command control startup failed")
	}
	server := &http.Server{Addr: cfg.ListenAddress, Handler: commandRuntime.Handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() { _ = application.Scheduler.Run(ctx) }()
	if commandRuntime.Worker != nil {
		go func() { _ = commandRuntime.Worker.Run(ctx) }()
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("relay-ops HTTP server failed")
	}
}
