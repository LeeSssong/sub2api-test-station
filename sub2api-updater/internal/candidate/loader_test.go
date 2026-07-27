package candidate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testVersion        = "0.1.167"
	testOfficialCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testSourceCommit   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testDigest         = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	testReference      = "ghcr.io/leesssong/xingqiao-sub2api@" + testDigest
	testImageID        = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

func TestPreparePullsAndQualifiesCandidateWithoutChangingRuntime(t *testing.T) {
	temp := t.TempDir()
	compose := filepath.Join(temp, "compose.yaml")
	if err := os.WriteFile(compose, []byte("services:\n  sub2api:\n    image: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	loader := Loader{
		Runner: runner, Disk: fakeDisk{Free: 8 << 30, UsedPercent: 50},
		ComposePath: compose, StatePath: filepath.Join(temp, "state/state.json"),
		TempRoot: temp, Registry: "ghcr.io/leesssong/xingqiao-sub2api",
		RegistryUser: "LeeSssong", ContainerName: "sub2api-sub2api-1",
	}

	result, err := loader.Prepare(context.Background(), Request{
		Reference: testReference, Version: testVersion,
		OfficialCommit: testOfficialCommit, SourceCommit: testSourceCommit,
		RegistryToken: []byte("token-must-not-leak"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ImageID != testImageID || result.Version != testVersion {
		t.Fatalf("result=%#v", result)
	}
	if len(runner.calls) != 7 {
		t.Fatalf("calls=%#v", runner.calls)
	}
	all := runner.rendered()
	for _, required := range []string{
		"login ghcr.io -u LeeSssong --password-stdin",
		"pull --platform linux/amd64 " + testReference,
		"image inspect --format {{json .}} " + testReference,
		"run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges",
		"image tag " + testReference + " xingqiao-sub2api:upstream-" + testVersion,
	} {
		if !strings.Contains(all, required) {
			t.Fatalf("missing %q in %s", required, all)
		}
	}
	for _, forbidden := range []string{"token-must-not-leak", " compose ", "/system/update", "psql", "restart", "prune"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("forbidden %q in %s", forbidden, all)
		}
	}
	if runner.loginInput != "token-must-not-leak" {
		t.Fatalf("login input was not delivered through stdin")
	}
	if runner.dockerConfig == "" || strings.Contains(runner.dockerConfig, "token") {
		t.Fatalf("docker config=%q", runner.dockerConfig)
	}
	if _, err := os.Stat(runner.dockerConfig); !os.IsNotExist(err) {
		t.Fatalf("temporary Docker config remains: %v", err)
	}
	stateInfo, err := os.Stat(loader.StatePath)
	if err != nil || stateInfo.Mode().Perm() != 0o600 {
		t.Fatalf("state mode=%v err=%v", stateInfo, err)
	}
	stateBytes, _ := os.ReadFile(loader.StatePath)
	if strings.Contains(string(stateBytes), "token") {
		t.Fatal("state leaked registry token")
	}
	var state Result
	if err := json.Unmarshal(stateBytes, &state); err != nil || state.Runtime.Health != "healthy" {
		t.Fatalf("state=%s err=%v", stateBytes, err)
	}
}

func TestPrepareFailsClosedBeforeDockerForInvalidInputOrLowDisk(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		disk fakeDisk
	}{
		{name: "mutable tag", req: validRequest("ghcr.io/leesssong/xingqiao-sub2api:latest"), disk: fakeDisk{Free: 8 << 30, UsedPercent: 50}},
		{name: "wrong registry", req: validRequest("ghcr.io/other/image@" + testDigest), disk: fakeDisk{Free: 8 << 30, UsedPercent: 50}},
		{name: "invalid version", req: Request{Reference: testReference, Version: "0.1.167;id", OfficialCommit: testOfficialCommit, SourceCommit: testSourceCommit, RegistryToken: []byte("x")}, disk: fakeDisk{Free: 8 << 30, UsedPercent: 50}},
		{name: "low free", req: validRequest(testReference), disk: fakeDisk{Free: 4 << 30, UsedPercent: 50}},
		{name: "high use", req: validRequest(testReference), disk: fakeDisk{Free: 8 << 30, UsedPercent: 85}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{}
			loader := Loader{Runner: runner, Disk: test.disk, ComposePath: filepath.Join(t.TempDir(), "compose"), Registry: "ghcr.io/leesssong/xingqiao-sub2api"}
			if _, err := loader.Prepare(context.Background(), test.req); err == nil {
				t.Fatal("Prepare accepted invalid request")
			}
			if len(runner.calls) != 0 {
				t.Fatalf("Docker was called: %#v", runner.calls)
			}
		})
	}
}

func TestPrepareRejectsRuntimeDriftAndDoesNotWriteState(t *testing.T) {
	temp := t.TempDir()
	compose := filepath.Join(temp, "compose.yaml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{driftAfter: true}
	state := filepath.Join(temp, "state.json")
	loader := Loader{
		Runner: runner, Disk: fakeDisk{Free: 8 << 30, UsedPercent: 50},
		ComposePath: compose, StatePath: state, TempRoot: temp,
		Registry: "ghcr.io/leesssong/xingqiao-sub2api", RegistryUser: "LeeSssong",
		ContainerName: "sub2api-sub2api-1",
	}
	if _, err := loader.Prepare(context.Background(), validRequest(testReference)); err == nil {
		t.Fatal("runtime drift was accepted")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state written after drift: %v", err)
	}
}

func validRequest(reference string) Request {
	return Request{
		Reference: reference, Version: testVersion, OfficialCommit: testOfficialCommit,
		SourceCommit: testSourceCommit, RegistryToken: []byte("token-must-not-leak"),
	}
}

type fakeDisk struct {
	Free        uint64
	UsedPercent int
}

func (d fakeDisk) Usage(string) (uint64, int, error) {
	return d.Free, d.UsedPercent, nil
}

type runnerCall struct {
	stdin string
	env   []string
	args  []string
}

type fakeRunner struct {
	calls        []runnerCall
	loginInput   string
	dockerConfig string
	inspectCount int
	driftAfter   bool
}

func (r *fakeRunner) Run(_ context.Context, stdin []byte, env []string, name string, args ...string) (string, string, error) {
	callArgs := append([]string{name}, args...)
	r.calls = append(r.calls, runnerCall{stdin: string(stdin), env: append([]string(nil), env...), args: callArgs})
	if len(args) >= 3 && args[2] == "login" {
		r.loginInput = string(stdin)
		r.dockerConfig = args[1]
	}
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "inspect --format {{json .}} sub2api-sub2api-1"):
		r.inspectCount++
		image := "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
		if r.driftAfter && r.inspectCount > 1 {
			image = testImageID
		}
		return `{"Id":"container-1","Image":"` + image + `","RestartCount":0,"State":{"StartedAt":"2026-07-28T00:00:00Z","Status":"running","Health":{"Status":"healthy"}}}`, "", nil
	case strings.Contains(joined, "image inspect --format {{json .}}"):
		return `{"Id":"` + testImageID + `","Os":"linux","Architecture":"amd64","Config":{"Labels":{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"` + testVersion + `","com.xingqiao.sub2api.upstream.commit":"` + testOfficialCommit + `","com.xingqiao.sub2api.source.commit":"` + testSourceCommit + `"}}}`, "", nil
	case strings.Contains(joined, "--entrypoint /app/sub2api"):
		return "Sub2API " + testVersion + "\nCommit: " + testOfficialCommit + "\n", "", nil
	default:
		return "", "", nil
	}
}

func (r *fakeRunner) rendered() string {
	var lines []string
	for _, call := range r.calls {
		lines = append(lines, strings.Join(call.args, " ")+" "+strings.Join(call.env, " "))
	}
	return strings.Join(lines, "\n")
}
