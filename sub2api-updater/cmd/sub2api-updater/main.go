package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"example.invalid/sub2api-updater/internal/updater"
)

const (
	defaultSocket       = "/run/sub2api-updater/updater.sock"
	defaultState        = "/var/lib/sub2api-updater/state.json"
	defaultExecutor     = "/opt/sub2api/production/ops/update-sub2api-host.sh"
	defaultOrigin       = "https://api.xingqiaolab.top"
	defaultAPI          = "https://api.xingqiaolab.top"
	defaultOfficialDial = "127.0.0.1:443"
)

func main() {
	var (
		socketPath   = flag.String("socket", defaultSocket, "Unix socket path")
		statePath    = flag.String("state", defaultState, "persistent operation state path")
		executor     = flag.String("executor", defaultExecutor, "root-owned host update executor")
		official     = flag.String("official-api", defaultAPI, "official Sub2API API base URL")
		officialDial = flag.String("official-dial-address", defaultOfficialDial, "loopback Caddy address for official authentication")
		origin       = flag.String("origin", defaultOrigin, "required browser Origin")
		githubURL    = flag.String("github-latest-release", "", "GitHub latest release API URL")
	)
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*statePath), 0o700); err != nil {
		log.Fatalf("create state directory: %v", err)
	}
	if err := os.Chmod(filepath.Dir(*statePath), 0o700); err != nil {
		log.Fatalf("secure state directory: %v", err)
	}
	officialClient, err := newOfficialHTTPClient(*officialDial)
	if err != nil {
		log.Fatalf("configure official authentication transport: %v", err)
	}
	authenticator, err := updater.NewOfficialAuthenticator(*official, officialClient)
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

func newOfficialHTTPClient(dialAddress string) (*http.Client, error) {
	host, port, err := net.SplitHostPort(dialAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid official dial address: %w", err)
	}
	ip := net.ParseIP(host)
	portNumber, err := strconv.Atoi(port)
	if ip == nil || !ip.IsLoopback() || err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("official dial address must be a loopback IP and valid port")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, dialAddress)
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, nil
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
