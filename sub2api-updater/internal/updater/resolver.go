package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var (
	ErrTargetChanged     = errors.New("requested version is not the latest release")
	ErrCandidateNotReady = errors.New("qualified update candidate is not ready")
)

const (
	qualifiedImageRepository = "xingqiao-sub2api"
	defaultLatestReleaseURL  = "https://api.github.com/repos/Wei-Shaw/sub2api/releases/latest"
)

var versionPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){1,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

// CommandRunner is intentionally small so Docker and the host executor can be
// tested without a host daemon. env entries are appended to the inherited
// environment; nil leaves it untouched.
type CommandRunner interface {
	Run(ctx context.Context, env []string, name string, args ...string) (stdout string, stderr string, err error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, env []string, name string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		command.Env = append(os.Environ(), env...)
	}
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

// UpdateResolver verifies the current GitHub release and pins it to a qualified Xingqiao image ID.
type UpdateResolver struct {
	client           *http.Client
	latestReleaseURL string
	docker           CommandRunner
}

func NewResolver(client *http.Client, latestReleaseURL string, docker CommandRunner) *UpdateResolver {
	if client == nil {
		client = http.DefaultClient
	}
	if latestReleaseURL == "" {
		latestReleaseURL = defaultLatestReleaseURL
	}
	if docker == nil {
		docker = execCommandRunner{}
	}
	return &UpdateResolver{client: client, latestReleaseURL: latestReleaseURL, docker: docker}
}

func (r *UpdateResolver) Resolve(ctx context.Context, targetVersion string) (string, error) {
	target, err := normalizeVersion(targetVersion)
	if err != nil {
		return "", fmt.Errorf("invalid target version: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.latestReleaseURL, nil)
	if err != nil {
		return "", fmt.Errorf("build latest release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest release: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch latest release: status %d", res.StatusCode)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("decode latest release: %w", err)
	}
	latest, err := normalizeVersion(release.TagName)
	if err != nil || latest != target {
		return "", ErrTargetChanged
	}
	officialCommit, err := r.resolveOfficialCommit(ctx, release.TagName)
	if err != nil {
		return "", err
	}

	imageTag := qualifiedImageRepository + ":upstream-" + target
	stdout, stderr, err := r.docker.Run(ctx, nil, "docker", "image", "inspect", "--format", "{{json .}}", imageTag)
	if err != nil {
		failure := commandFailure(stderr, err)
		if strings.Contains(stderr, "No such image: "+imageTag) {
			return "", fmt.Errorf("%w for %s: %v", ErrCandidateNotReady, target, failure)
		}
		return "", fmt.Errorf("qualified Xingqiao image is not available for %s: %w", target, failure)
	}
	imageID, ok := matchingQualifiedImage(stdout, target, officialCommit)
	if !ok {
		return "", errors.New("Xingqiao image qualification labels are missing or do not match the official release")
	}
	return imageID, nil
}

func (r *UpdateResolver) resolveOfficialCommit(ctx context.Context, tag string) (string, error) {
	ref, err := r.gitObject(ctx, "/git/ref/tags/"+url.PathEscape(tag))
	if err != nil {
		return "", fmt.Errorf("resolve official release tag: %w", err)
	}
	switch ref.Type {
	case "commit":
		if commitPattern.MatchString(ref.SHA) {
			return ref.SHA, nil
		}
	case "tag":
		object, err := r.gitObject(ctx, "/git/tags/"+ref.SHA)
		if err != nil {
			return "", fmt.Errorf("dereference official release tag: %w", err)
		}
		if object.Type == "commit" && commitPattern.MatchString(object.SHA) {
			return object.SHA, nil
		}
	}
	return "", errors.New("official release tag does not resolve to a commit")
}

type gitObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

func (r *UpdateResolver) gitObject(ctx context.Context, suffix string) (gitObject, error) {
	endpoint, err := r.gitAPIURL(suffix)
	if err != nil {
		return gitObject{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return gitObject{}, fmt.Errorf("build GitHub request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := r.client.Do(req)
	if err != nil {
		return gitObject{}, fmt.Errorf("fetch GitHub object: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return gitObject{}, fmt.Errorf("fetch GitHub object: status %d", res.StatusCode)
	}
	var response struct {
		Object gitObject `json:"object"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&response); err != nil {
		return gitObject{}, fmt.Errorf("decode GitHub object: %w", err)
	}
	return response.Object, nil
}

func (r *UpdateResolver) gitAPIURL(suffix string) (string, error) {
	endpoint, err := url.Parse(r.latestReleaseURL)
	if err != nil {
		return "", fmt.Errorf("parse latest release URL: %w", err)
	}
	const latestSuffix = "/releases/latest"
	if !strings.HasSuffix(endpoint.Path, latestSuffix) {
		return "", errors.New("latest release URL must end with /releases/latest")
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, latestSuffix) + suffix
	endpoint.RawQuery = ""
	return endpoint.String(), nil
}

func normalizeVersion(version string) (string, error) {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if !versionPattern.MatchString(version) {
		return "", errors.New("invalid version")
	}
	return version, nil
}

func matchingQualifiedImage(output string, targetVersion string, officialCommit string) (string, bool) {
	var image struct {
		ID           string `json:"Id"`
		OS           string `json:"Os"`
		Architecture string `json:"Architecture"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &image); err != nil {
		return "", false
	}
	labels := image.Config.Labels
	if !digestPattern.MatchString(image.ID) ||
		image.OS != "linux" ||
		image.Architecture != "amd64" ||
		labels["com.xingqiao.sub2api.qualified"] != "true" ||
		labels["com.xingqiao.sub2api.upstream.version"] != targetVersion ||
		labels["com.xingqiao.sub2api.upstream.commit"] != officialCommit ||
		!commitPattern.MatchString(labels["com.xingqiao.sub2api.source.commit"]) {
		return "", false
	}
	return image.ID, true
}

func commandFailure(stderr string, err error) error {
	if strings.TrimSpace(stderr) == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr))
}
