package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"example.invalid/sub2api-updater/internal/updater"
)

const (
	defaultSocket   = "/run/sub2api-updater/updater.sock"
	defaultState    = "/var/lib/sub2api-updater/state.json"
	defaultExecutor = "/opt/sub2api/production/ops/update-sub2api-host.sh"
	defaultOrigin   = "https://api.xingqialab.top"
	defaultAPI      = "http://127.0.0.1:8080"
)

func main() {
	var (
		socketPath = flag.String("socket", defaultSocket, "Unix socket path")
		statePath  = flag.String("state", defaultState, "persistent operation state path")
		executor   = flag.String("executor", defaultExecutor, "root-owned host update executor")
		official   = flag.String("official-api", defaultAPI, "official Sub2API API base URL")
		origin     = flag.String("origin", defaultOrigin, "required browser Origin")
		githubURL  = flag.String("github-latest-release", "", "GitHub latest release API URL")
	)
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*statePath), 0o700); err != nil {
		log.Fatalf("create state directory: %v", err)
	}
	if err := os.Chmod(filepath.Dir(*statePath), 0o700); err != nil {
		log.Fatalf("secure state directory: %v", err)
	}
	authenticator, err := updater.NewOfficialAuthenticator(*official, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		log.Fatalf("configure official authentication: %v", err)
	}
	service := updater.NewService(
		updater.NewStore(*statePath),
		updater.NewResolver(&http.Client{Timeout: 20 * time.Second}, *githubURL, nil),
		updater.NewHostExecutor(*executor, nil),
	)
	defer service.Close()

	listener, err := listenUnix(*socketPath)
	if err != nil {
		log.Fatalf("listen on updater socket: %v", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(*socketPath)
	}()

	server := &http.Server{
		Handler:           updater.NewHTTP(service, authenticator, *origin),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      16 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("updater server stopped: %v", err)
		}
	}()
	<-shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("updater server shutdown: %v", err)
	}
}

func listenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("refusing to replace non-socket listener path")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return listener, nil
}
