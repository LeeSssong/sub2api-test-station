package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"example.invalid/relay-ops-service/internal/notify"
)

const (
	maxEventBytes   = 64 << 10
	maxWebhookBytes = 8 << 10
)

var releaseVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type sendReleasePreparation func(context.Context, string, notify.FeishuMessage) error

func main() {
	if err := run(os.Args[1:], sendWebhook); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "release preparation notification failed")
		os.Exit(1)
	}
}

func run(args []string, send sendReleasePreparation) error {
	flags := flag.NewFlagSet("release-prep-notify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var eventPath string
	var webhookPath string
	flags.StringVar(&eventPath, "event", "", "absolute path to a release preparation event")
	flags.StringVar(&webhookPath, "webhook-file", "", "absolute path to the Feishu webhook file")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid arguments")
	}
	if send == nil {
		return errors.New("notification sender is required")
	}

	eventBytes, err := readSecureFile(eventPath, maxEventBytes)
	if err != nil {
		return errors.New("release preparation event is unavailable")
	}
	webhookFile, err := inspectSecureFile(webhookPath, maxWebhookBytes)
	if err != nil {
		return errors.New("Feishu webhook file is unavailable")
	}
	if err := webhookFile.Close(); err != nil {
		return errors.New("Feishu webhook file is unavailable")
	}

	var view notify.ReleasePreparationView
	decoder := json.NewDecoder(bytes.NewReader(eventBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		return errors.New("release preparation event is invalid")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return errors.New("release preparation event is invalid")
	}
	if err := validateReleasePreparationView(view); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return send(ctx, webhookPath, notify.RenderReleasePreparation(view))
}

func sendWebhook(ctx context.Context, webhookPath string, message notify.FeishuMessage) error {
	return (notify.Client{WebhookFile: webhookPath}).Send(ctx, message)
}

func validateReleasePreparationView(view notify.ReleasePreparationView) error {
	switch view.Status {
	case "succeeded":
		if !releaseVersion.MatchString(view.Version) {
			return errors.New("release preparation version is invalid")
		}
	case "failed":
		if view.Version != "" && !releaseVersion.MatchString(view.Version) {
			return errors.New("release preparation version is invalid")
		}
	default:
		return errors.New("release preparation status is invalid")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func readSecureFile(path string, maximum int64) ([]byte, error) {
	file, err := inspectSecureFile(path, maximum)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || len(value) == 0 || int64(len(value)) > maximum {
		return nil, errors.New("secure file is unreadable")
	}
	return value, nil
}

func inspectSecureFile(path string, maximum int64) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("secure file path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
		info.Size() <= 0 || info.Size() > maximum {
		return nil, errors.New("secure file is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("secure file is unavailable")
	}
	current, err := file.Stat()
	if err != nil || !current.Mode().IsRegular() || current.Mode().Perm() != 0o600 ||
		current.Size() != info.Size() {
		_ = file.Close()
		return nil, errors.New("secure file changed")
	}
	return file, nil
}
