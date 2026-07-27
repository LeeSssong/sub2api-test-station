package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"example.invalid/sub2api-updater/internal/candidate"
)

const (
	settingsPath = "/etc/sub2api/sub2api-candidate-loader.env"
	composePath  = "/opt/sub2api/production/compose.yaml"
	statePath    = "/var/lib/sub2api-candidate-loader/state.json"
	container    = "sub2api-sub2api-1"
)

type settings struct {
	RegistryUser string
	Registry     string
}

func main() {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "candidate_loader status=failed reason=host_boundary")
		os.Exit(1)
	}
	config, err := loadSettings(settingsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate_loader status=failed reason=config")
		os.Exit(1)
	}
	request, err := parseRequest(os.Args[1:], os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate_loader status=failed reason=request")
		os.Exit(1)
	}
	defer clear(request.RegistryToken)

	loader := candidate.Loader{
		Runner: candidate.ExecRunner{}, Disk: candidate.StatfsDisk{},
		ComposePath: composePath, StatePath: statePath,
		Registry: config.Registry, RegistryUser: config.RegistryUser,
		ContainerName: container,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	result, err := loader.Prepare(ctx, request)
	if err != nil {
		fmt.Fprintln(os.Stderr, "candidate_loader status=failed reason=qualification")
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, "candidate_loader status=failed reason=output")
		os.Exit(1)
	}
}

func parseRequest(arguments []string, input io.Reader) (candidate.Request, error) {
	if len(arguments) != 5 || arguments[0] != "prepare" {
		return candidate.Request{}, errors.New("invalid command")
	}
	reader := bufio.NewReader(io.LimitReader(input, 4097))
	tokenBytes, err := io.ReadAll(reader)
	if err != nil || len(tokenBytes) == 0 || len(tokenBytes) > 4096 {
		return candidate.Request{}, errors.New("invalid registry token")
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return candidate.Request{}, errors.New("invalid registry token")
	}
	return candidate.Request{
		Reference: arguments[1], Version: arguments[2], OfficialCommit: arguments[3],
		SourceCommit: arguments[4], RegistryToken: []byte(token),
	}, nil
}

func loadSettings(path string) (settings, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return settings{}, errors.New("invalid settings file")
	}
	file, err := os.Open(path)
	if err != nil {
		return settings{}, errors.New("open settings")
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(io.LimitReader(file, 16<<10))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "SUB2API_CANDIDATE_REGISTRY_USER" && key != "SUB2API_CANDIDATE_REGISTRY") {
			return settings{}, errors.New("invalid settings entry")
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return settings{}, errors.New("read settings")
	}
	config := settings{
		RegistryUser: values["SUB2API_CANDIDATE_REGISTRY_USER"],
		Registry:     values["SUB2API_CANDIDATE_REGISTRY"],
	}
	if config.RegistryUser == "" || config.Registry != "ghcr.io/leesssong/xingqiao-sub2api" {
		return settings{}, errors.New("invalid settings values")
	}
	return config, nil
}

func clear(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
