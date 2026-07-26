package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordedCommandRunner struct {
	calls   [][]string
	envs    [][]string
	results []commandResult
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

func (r *recordedCommandRunner) Run(_ context.Context, env []string, name string, args ...string) (string, string, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	r.envs = append(r.envs, env)
	result := r.results[0]
	r.results = r.results[1:]
	return result.stdout, result.stderr, result.err
}

func TestResolverPinsLatestReleaseToQualifiedLocalImageID(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{
		{stdout: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`},
	}}
	resolver := NewResolver(github.Client(), github.URL+"/latest", docker)
	image, err := resolver.Resolve(context.Background(), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	want := "sha256:" + strings.Repeat("a", 64)
	if image != want {
		t.Fatalf("image = %q, want %q", image, want)
	}
	if got := docker.calls; len(got) != 1 || strings.Join(got[0], " ") != "docker image inspect --format {{json .}} xingqiao-sub2api:upstream-1.2.3" {
		t.Fatalf("docker calls = %#v", got)
	}
}

func TestResolverRejectsTargetThatIsNotLatestBeforeDocker(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer github.Close()
	docker := &recordedCommandRunner{}
	_, err := NewResolver(github.Client(), github.URL, docker).Resolve(context.Background(), "1.2.4")
	if !errors.Is(err, ErrTargetChanged) {
		t.Fatalf("error = %v", err)
	}
	if len(docker.calls) != 0 {
		t.Fatalf("docker calls = %#v", docker.calls)
	}
}

func TestResolverRejectsImageWithoutQualificationLabel(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{{stdout: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Config":{"Labels":{"com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`}}}
	_, err := NewResolver(github.Client(), github.URL, docker).Resolve(context.Background(), "1.2.3")
	if err == nil {
		t.Fatal("expected qualification failure")
	}
}

func TestResolverRejectsQualifiedImageForAnotherUpstreamVersion(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{{stdout: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.2","com.xingqiao.sub2api.upstream.commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`}}}
	_, err := NewResolver(github.Client(), github.URL, docker).Resolve(context.Background(), "1.2.3")
	if err == nil {
		t.Fatal("expected upstream version mismatch")
	}
}
