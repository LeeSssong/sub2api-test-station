package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/notify"
)

func TestRunReadsStrictEventAndSendsRenderedCard(t *testing.T) {
	dir := t.TempDir()
	event := filepath.Join(dir, "event.json")
	webhook := filepath.Join(dir, "webhook")
	if err := os.WriteFile(event, []byte(`{"status":"succeeded","version":"0.1.167"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webhook, []byte("https://open.feishu.cn/hook/test"), 0o600); err != nil {
		t.Fatal(err)
	}
	var delivered notify.FeishuMessage
	err := run(
		[]string{"--event", event, "--webhook-file", webhook},
		func(_ context.Context, path string, message notify.FeishuMessage) error {
			if path != webhook {
				t.Fatalf("webhook path=%q", path)
			}
			delivered = message
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Card == nil || delivered.Card.Header.Title.Content == "" {
		t.Fatalf("message=%#v", delivered)
	}
}

func TestRunRejectsUnknownFieldsAndInsecureFiles(t *testing.T) {
	dir := t.TempDir()
	event := filepath.Join(dir, "event.json")
	webhook := filepath.Join(dir, "webhook")
	if err := os.WriteFile(webhook, []byte("https://open.feishu.cn/hook/test"), 0o600); err != nil {
		t.Fatal(err)
	}
	sender := func(context.Context, string, notify.FeishuMessage) error {
		t.Fatal("sender called")
		return nil
	}
	if err := os.WriteFile(event, []byte(`{"status":"failed","version":"0.1.167","raw_stderr":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--event", event, "--webhook-file", webhook}, sender); err == nil {
		t.Fatal("accepted unknown event field")
	}
	if err := os.WriteFile(event, []byte(`{"status":"succeeded","version":"0.1.167"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(event, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--event", event, "--webhook-file", webhook}, sender); err == nil {
		t.Fatal("accepted insecure event file")
	}
}

func TestRunRejectsInvalidStatusOversizedInputAndInsecureWebhook(t *testing.T) {
	dir := t.TempDir()
	event := filepath.Join(dir, "event.json")
	webhook := filepath.Join(dir, "webhook")
	sender := func(context.Context, string, notify.FeishuMessage) error {
		t.Fatal("sender called")
		return nil
	}
	if err := os.WriteFile(webhook, []byte("https://open.feishu.cn/hook/test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(event, []byte(`{"status":"pending","version":"0.1.167"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--event", event, "--webhook-file", webhook}, sender); err == nil {
		t.Fatal("accepted invalid event status")
	}
	if err := os.WriteFile(event, []byte(strings.Repeat("x", (64<<10)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--event", event, "--webhook-file", webhook}, sender); err == nil {
		t.Fatal("accepted oversized event")
	}
	if err := os.WriteFile(event, []byte(`{"status":"succeeded","version":"0.1.167"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(webhook, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--event", event, "--webhook-file", webhook}, sender); err == nil {
		t.Fatal("accepted insecure webhook file")
	}
}

func TestRunRejectsSecretOrNonFileArguments(t *testing.T) {
	sender := func(context.Context, string, notify.FeishuMessage) error {
		t.Fatal("sender called")
		return nil
	}
	for _, args := range [][]string{
		{"--event", `{"status":"succeeded"}`, "--webhook-file", "/tmp/webhook"},
		{"--event", "/tmp/event", "--webhook-file", "https://open.feishu.cn/hook/secret"},
		{"--event", "/tmp/event", "--webhook-file", "/tmp/webhook", "Bearer secret-value"},
	} {
		if err := run(args, sender); err == nil {
			t.Fatalf("accepted args=%q", args)
		}
	}
}
