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

const testOfficialCommit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const testSourceCommit = "cccccccccccccccccccccccccccccccccccccccc"

func releaseServer(t *testing.T, annotated bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Wei-Shaw/sub2api/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","target_commitish":"main"}`))
		case "/repos/Wei-Shaw/sub2api/git/ref/tags/v1.2.3":
			if annotated {
				_, _ = w.Write([]byte(`{"object":{"type":"tag","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"object":{"type":"commit","sha":"` + testOfficialCommit + `"}}`))
		case "/repos/Wei-Shaw/sub2api/git/tags/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa":
			if !annotated {
				t.Fatalf("unexpected annotated-tag request")
			}
			_, _ = w.Write([]byte(`{"object":{"type":"commit","sha":"` + testOfficialCommit + `"}}`))
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
}

func releaseURL(server *httptest.Server) string {
	return server.URL + "/repos/Wei-Shaw/sub2api/releases/latest"
}

func (r *recordedCommandRunner) Run(_ context.Context, env []string, name string, args ...string) (string, string, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	r.envs = append(r.envs, env)
	result := r.results[0]
	r.results = r.results[1:]
	return result.stdout, result.stderr, result.err
}

func TestResolverPinsLatestReleaseToQualifiedLocalImageID(t *testing.T) {
	github := releaseServer(t, false)
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{
		{stdout: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"linux","Architecture":"amd64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"` + testOfficialCommit + `","com.xingqiao.sub2api.source.commit":"` + testSourceCommit + `"}}}`},
	}}
	resolver := NewResolver(github.Client(), releaseURL(github), docker)
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

func TestResolverDereferencesAnnotatedReleaseTags(t *testing.T) {
	github := releaseServer(t, true)
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{{
		stdout: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"linux","Architecture":"amd64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"` + testOfficialCommit + `","com.xingqiao.sub2api.source.commit":"` + testSourceCommit + `"}}}`,
	}}}

	if _, err := NewResolver(github.Client(), releaseURL(github), docker).Resolve(context.Background(), "1.2.3"); err != nil {
		t.Fatal(err)
	}
}

func TestResolverRejectsIncompleteOrMismatchedCandidateQualification(t *testing.T) {
	for _, test := range []struct {
		name      string
		imageJSON string
	}{
		{
			name:      "official commit differs from latest release",
			imageJSON: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"linux","Architecture":"amd64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"dddddddddddddddddddddddddddddddddddddddd","com.xingqiao.sub2api.source.commit":"` + testSourceCommit + `"}}}`,
		},
		{
			name:      "source commit is missing",
			imageJSON: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"linux","Architecture":"amd64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"` + testOfficialCommit + `"}}}`,
		},
		{
			name:      "source commit is malformed",
			imageJSON: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"linux","Architecture":"amd64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"` + testOfficialCommit + `","com.xingqiao.sub2api.source.commit":"not-a-commit"}}}`,
		},
		{
			name:      "image is not linux",
			imageJSON: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"windows","Architecture":"amd64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"` + testOfficialCommit + `","com.xingqiao.sub2api.source.commit":"` + testSourceCommit + `"}}}`,
		},
		{
			name:      "image is not amd64",
			imageJSON: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Os":"linux","Architecture":"arm64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"` + testOfficialCommit + `","com.xingqiao.sub2api.source.commit":"` + testSourceCommit + `"}}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			github := releaseServer(t, false)
			defer github.Close()
			docker := &recordedCommandRunner{results: []commandResult{{stdout: test.imageJSON}}}

			_, err := NewResolver(github.Client(), releaseURL(github), docker).Resolve(context.Background(), "1.2.3")
			if err == nil {
				t.Fatal("expected qualification failure")
			}
		})
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

func TestResolverClassifiesMissingQualifiedImageAsCandidateNotReady(t *testing.T) {
	github := releaseServer(t, false)
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{{
		stderr: "Error response from daemon: No such image: xingqiao-sub2api:upstream-1.2.3",
		err:    errors.New("exit status 1"),
	}}}

	_, err := NewResolver(github.Client(), releaseURL(github), docker).Resolve(context.Background(), "1.2.3")
	if !errors.Is(err, ErrCandidateNotReady) {
		t.Fatalf("error = %v", err)
	}
}

func TestResolverDoesNotClassifyDockerDaemonOrPermissionFailuresAsCandidateNotReady(t *testing.T) {
	for _, stderr := range []string{
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
		"permission denied while trying to connect to the Docker daemon socket",
	} {
		t.Run(stderr, func(t *testing.T) {
			github := releaseServer(t, false)
			defer github.Close()
			docker := &recordedCommandRunner{results: []commandResult{{
				stderr: stderr,
				err:    errors.New("exit status 1"),
			}}}

			_, err := NewResolver(github.Client(), releaseURL(github), docker).Resolve(context.Background(), "1.2.3")
			if err == nil {
				t.Fatal("expected docker inspect failure")
			}
			if errors.Is(err, ErrCandidateNotReady) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestResolverRejectsImageWithoutQualificationLabel(t *testing.T) {
	github := releaseServer(t, false)
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{{stdout: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Config":{"Labels":{"com.xingqiao.sub2api.upstream.version":"1.2.3","com.xingqiao.sub2api.upstream.commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`}}}
	_, err := NewResolver(github.Client(), releaseURL(github), docker).Resolve(context.Background(), "1.2.3")
	if err == nil {
		t.Fatal("expected qualification failure")
	}
}

func TestResolverRejectsQualifiedImageForAnotherUpstreamVersion(t *testing.T) {
	github := releaseServer(t, false)
	defer github.Close()
	docker := &recordedCommandRunner{results: []commandResult{{stdout: `{"Id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"1.2.2","com.xingqiao.sub2api.upstream.commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`}}}
	_, err := NewResolver(github.Client(), releaseURL(github), docker).Resolve(context.Background(), "1.2.3")
	if err == nil {
		t.Fatal("expected upstream version mismatch")
	}
}
