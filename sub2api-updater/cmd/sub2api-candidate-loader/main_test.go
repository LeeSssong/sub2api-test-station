package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRequestAcceptsOnlyExactPrepareCommand(t *testing.T) {
	request, err := parseRequest(
		[]string{"prepare", testReferenceMain, "0.1.167", strings.Repeat("a", 40), strings.Repeat("b", 40)},
		strings.NewReader("short-lived-token\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Reference != testReferenceMain || string(request.RegistryToken) != "short-lived-token" {
		t.Fatalf("request=%#v", request)
	}
	for _, arguments := range [][]string{
		nil,
		{"shell"},
		{"prepare", testReferenceMain, "0.1.167", strings.Repeat("a", 40)},
		{"prepare", testReferenceMain, "0.1.167", strings.Repeat("a", 40), strings.Repeat("b", 40), "extra"},
	} {
		if _, err := parseRequest(arguments, strings.NewReader("token\n")); err == nil {
			t.Fatalf("accepted %#v", arguments)
		}
	}
	if _, err := parseRequest(
		[]string{"prepare", testReferenceMain, "0.1.167", strings.Repeat("a", 40), strings.Repeat("b", 40)},
		strings.NewReader(strings.Repeat("x", 5000)),
	); err == nil {
		t.Fatal("accepted oversized token")
	}
}

func TestLoadSettingsReadsOnlyKnownRootOwnedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.env")
	if err := os.WriteFile(path, []byte(
		"SUB2API_CANDIDATE_REGISTRY_USER=LeeSssong\n"+
			"SUB2API_CANDIDATE_REGISTRY=ghcr.io/leesssong/xingqiao-sub2api\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := loadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.RegistryUser != "LeeSssong" || settings.Registry != "ghcr.io/leesssong/xingqiao-sub2api" {
		t.Fatalf("settings=%#v", settings)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSettings(path); err == nil {
		t.Fatal("accepted insecure settings file")
	}
}

const testReferenceMain = "ghcr.io/leesssong/xingqiao-sub2api@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
