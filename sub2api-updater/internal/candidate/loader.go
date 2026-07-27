package candidate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

const (
	minimumFreeBytes = uint64(5 << 30)
	maximumUsedPct   = 85
)

var (
	versionPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){1,2}$`)
	commitPattern  = regexp.MustCompile(`^[a-f0-9]{40}$`)
	digestPattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type Request struct {
	Reference      string
	Version        string
	OfficialCommit string
	SourceCommit   string
	RegistryToken  []byte
}

type RuntimeSnapshot struct {
	ContainerID   string `json:"container_id"`
	ImageID       string `json:"image_id"`
	StartedAt     string `json:"started_at"`
	Status        string `json:"status"`
	Health        string `json:"health"`
	RestartCount  int    `json:"restart_count"`
	ComposeSHA256 string `json:"compose_sha256"`
}

type Result struct {
	Version        string          `json:"version"`
	Reference      string          `json:"reference"`
	ImageID        string          `json:"image_id"`
	OfficialCommit string          `json:"official_commit"`
	SourceCommit   string          `json:"source_commit"`
	Runtime        RuntimeSnapshot `json:"runtime"`
}

type CommandRunner interface {
	Run(context.Context, []byte, []string, string, ...string) (string, string, error)
}

type DiskChecker interface {
	Usage(string) (freeBytes uint64, usedPercent int, err error)
}

type Loader struct {
	Runner        CommandRunner
	Disk          DiskChecker
	ComposePath   string
	StatePath     string
	TempRoot      string
	Registry      string
	RegistryUser  string
	ContainerName string
}

func (l *Loader) Prepare(ctx context.Context, request Request) (Result, error) {
	if err := l.validate(request); err != nil {
		return Result{}, err
	}
	free, used, err := l.Disk.Usage(l.ComposePath)
	if err != nil {
		return Result{}, errors.New("candidate disk check failed")
	}
	if free < minimumFreeBytes || used >= maximumUsedPct {
		return Result{}, errors.New("candidate disk threshold is not satisfied")
	}
	before, err := l.snapshot(ctx)
	if err != nil {
		return Result{}, err
	}

	tempRoot := l.TempRoot
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	dockerConfig, err := os.MkdirTemp(tempRoot, "sub2api-candidate-docker.")
	if err != nil {
		return Result{}, errors.New("create temporary Docker config")
	}
	if err := os.Chmod(dockerConfig, 0o700); err != nil {
		_ = os.RemoveAll(dockerConfig)
		return Result{}, errors.New("secure temporary Docker config")
	}
	defer func() { _ = os.RemoveAll(dockerConfig) }()

	dockerArgs := []string{"--config", dockerConfig}
	if _, _, err := l.Runner.Run(ctx, request.RegistryToken, nil, "docker",
		append(dockerArgs, "login", "ghcr.io", "-u", l.RegistryUser, "--password-stdin")...); err != nil {
		return Result{}, errors.New("candidate registry login failed")
	}
	if _, _, err := l.Runner.Run(ctx, nil, nil, "docker",
		append(dockerArgs, "pull", "--platform", "linux/amd64", request.Reference)...); err != nil {
		return Result{}, errors.New("candidate image pull failed")
	}

	imageJSON, _, err := l.Runner.Run(ctx, nil, nil, "docker",
		"image", "inspect", "--format", "{{json .}}", request.Reference)
	if err != nil {
		return Result{}, errors.New("candidate image inspect failed")
	}
	imageID, err := validateImage(imageJSON, request)
	if err != nil {
		return Result{}, err
	}
	versionOutput, _, err := l.Runner.Run(ctx, nil, nil, "docker",
		"run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--entrypoint", "/app/sub2api",
		request.Reference, "--version")
	if err != nil {
		return Result{}, errors.New("candidate binary version check failed")
	}
	if !strings.Contains(versionOutput, "Sub2API "+request.Version) ||
		!strings.Contains(versionOutput, request.OfficialCommit) {
		return Result{}, errors.New("candidate binary version does not match release")
	}
	localTag := "xingqiao-sub2api:upstream-" + request.Version
	if _, _, err := l.Runner.Run(ctx, nil, nil, "docker", "image", "tag", request.Reference, localTag); err != nil {
		return Result{}, errors.New("candidate local tag failed")
	}

	after, err := l.snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	if before != after {
		return Result{}, errors.New("production runtime changed while staging candidate")
	}
	result := Result{
		Version: request.Version, Reference: request.Reference, ImageID: imageID,
		OfficialCommit: request.OfficialCommit, SourceCommit: request.SourceCommit,
		Runtime: after,
	}
	if err := writeState(l.StatePath, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (l *Loader) validate(request Request) error {
	if l.Runner == nil || l.Disk == nil || l.ComposePath == "" || l.StatePath == "" ||
		l.Registry == "" || l.RegistryUser == "" || l.ContainerName == "" {
		return errors.New("candidate loader is not configured")
	}
	if !versionPattern.MatchString(request.Version) ||
		!commitPattern.MatchString(request.OfficialCommit) ||
		!commitPattern.MatchString(request.SourceCommit) ||
		len(request.RegistryToken) == 0 || len(request.RegistryToken) > 4096 {
		return errors.New("invalid candidate request")
	}
	prefix := l.Registry + "@"
	if !strings.HasPrefix(request.Reference, prefix) ||
		!digestPattern.MatchString(strings.TrimPrefix(request.Reference, prefix)) {
		return errors.New("invalid candidate image reference")
	}
	return nil
}

func (l *Loader) snapshot(ctx context.Context) (RuntimeSnapshot, error) {
	composeHash, err := fileSHA256(l.ComposePath)
	if err != nil {
		return RuntimeSnapshot{}, errors.New("read production Compose")
	}
	output, _, err := l.Runner.Run(ctx, nil, nil, "docker", "inspect", "--format", "{{json .}}", l.ContainerName)
	if err != nil {
		return RuntimeSnapshot{}, errors.New("inspect production container")
	}
	var data struct {
		ID           string `json:"Id"`
		Image        string `json:"Image"`
		RestartCount int    `json:"RestartCount"`
		State        struct {
			StartedAt string `json:"StartedAt"`
			Status    string `json:"Status"`
			Health    struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &data); err != nil {
		return RuntimeSnapshot{}, errors.New("decode production container")
	}
	if data.ID == "" || data.Image == "" || data.State.StartedAt == "" ||
		data.State.Status != "running" || data.State.Health.Status != "healthy" {
		return RuntimeSnapshot{}, errors.New("production container is not healthy")
	}
	return RuntimeSnapshot{
		ContainerID: data.ID, ImageID: data.Image, StartedAt: data.State.StartedAt,
		Status: data.State.Status, Health: data.State.Health.Status,
		RestartCount: data.RestartCount, ComposeSHA256: composeHash,
	}, nil
}

func validateImage(raw string, request Request) (string, error) {
	var image struct {
		ID           string `json:"Id"`
		OS           string `json:"Os"`
		Architecture string `json:"Architecture"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &image); err != nil {
		return "", errors.New("decode candidate image")
	}
	labels := image.Config.Labels
	if !digestPattern.MatchString(image.ID) || image.OS != "linux" || image.Architecture != "amd64" ||
		labels["com.xingqiao.sub2api.qualified"] != "true" ||
		labels["com.xingqiao.sub2api.upstream.version"] != request.Version ||
		labels["com.xingqiao.sub2api.upstream.commit"] != request.OfficialCommit ||
		labels["com.xingqiao.sub2api.source.commit"] != request.SourceCommit {
		return "", errors.New("candidate image qualification mismatch")
	}
	return image.ID, nil
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, stdin []byte, env []string, name string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = bytes.NewReader(stdin)
	if env != nil {
		command.Env = append(os.Environ(), env...)
	}
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

type StatfsDisk struct{}

func (StatfsDisk) Usage(path string) (uint64, int, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if total == 0 {
		return 0, 0, fmt.Errorf("empty filesystem")
	}
	return free, int(((total - free) * 100) / total), nil
}
