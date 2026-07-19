package probes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/domain"
)

var ErrOutputTooLarge = errors.New("candidate probe output exceeds limit")
var ErrMalformedOutput = errors.New("candidate probe output is malformed")
var ErrProbeFailed = errors.New("candidate probe process failed")

const candidateKeyEnv = "RELAY_OPS_CANDIDATE_KEY"
const maxProbeKeyBytes = 8 << 10

type ProbeRun struct {
	SchemaVersion     int             `json:"schema_version"`
	RunID             string          `json:"run_id"`
	ChannelID         string          `json:"channel_id"`
	ProfileID         string          `json:"profile_id"`
	RecordedAt        time.Time       `json:"recorded_at"`
	Status            string          `json:"status"`
	EvidenceSource    string          `json:"evidence_source"`
	Metrics           ProbeMetrics    `json:"metrics"`
	Errors            json.RawMessage `json:"errors"`
	ChatRequests      int             `json:"-"`
	ExpenseUpperBound domain.MicroUSD `json:"-"`
}

type ProbeMetrics struct {
	Catalog      json.RawMessage `json:"catalog"`
	TextModels   []string        `json:"text_models"`
	ProbedModels []string        `json:"probed_models"`
	PerModel     json.RawMessage `json:"per_model"`
	Usage        ProbeUsage      `json:"usage"`
}

type ProbeUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type V2Executor struct {
	RubyPath                 string
	ScriptPath               string
	ProfilePath              string
	QualificationProfilePath string
	MaxOutputBytes           int
	MaxRequestCost           domain.MicroUSD
}

type QualificationRequest struct {
	Reason string
}

type QualificationResult struct {
	RunID  string
	Status string
	Record json.RawMessage
}

func (e *V2Executor) Watch(ctx context.Context, candidate candidates.Candidate) (ProbeRun, error) {
	if !candidate.Enabled || candidate.ID <= 0 || candidate.BaseURL == "" {
		return ProbeRun{}, fmt.Errorf("candidate is not probeable")
	}
	keyPath, err := secretPath(candidate.ProbeSecretRef)
	if err != nil {
		return ProbeRun{}, err
	}
	key, err := readSecret(keyPath)
	if err != nil {
		return ProbeRun{}, err
	}
	defer clear(key)

	channelsPath, cleanup, err := writeChannelRegistry(candidate)
	if err != nil {
		return ProbeRun{}, err
	}
	defer cleanup()

	limit := e.MaxOutputBytes
	if limit <= 0 {
		limit = 2 << 20
	}
	stdout := &cappedBuffer{limit: limit}
	stderr := &cappedBuffer{limit: 4 << 10}
	channelID := fmt.Sprintf("candidate-%d", candidate.ID)
	cmd := exec.CommandContext(ctx, e.RubyPath, e.ScriptPath,
		"watch", "--channels", channelsPath, "--profile", e.ProfilePath,
		"--channel", channelID, "--key-env", candidateKeyEnv,
	)
	cmd.Env = []string{"LANG=C.UTF-8", "TZ=UTC", candidateKeyEnv + "=" + string(key)}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return ProbeRun{}, ctx.Err()
	}
	if stdout.overflow {
		return ProbeRun{}, ErrOutputTooLarge
	}
	if runErr != nil {
		return ProbeRun{}, ErrProbeFailed
	}

	var run ProbeRun
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&run); err != nil || run.SchemaVersion != 1 || run.RunID == "" || run.ChannelID != channelID || run.Status == "" {
		return ProbeRun{}, ErrMalformedOutput
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return ProbeRun{}, ErrMalformedOutput
	}
	run.ChatRequests = len(run.Metrics.ProbedModels) * 2
	run.ExpenseUpperBound = domain.MicroUSD(run.ChatRequests) * e.MaxRequestCost
	return run, nil
}

func (e *V2Executor) Qualify(ctx context.Context, candidate candidates.Candidate, request QualificationRequest) (QualificationResult, error) {
	if strings.TrimSpace(request.Reason) == "" {
		return QualificationResult{}, fmt.Errorf("qualification reason is required")
	}
	if e.QualificationProfilePath == "" {
		return QualificationResult{}, fmt.Errorf("qualification profile is unavailable")
	}
	keyPath, err := secretPath(candidate.ProbeSecretRef)
	if err != nil {
		return QualificationResult{}, err
	}
	key, err := readSecret(keyPath)
	if err != nil {
		return QualificationResult{}, err
	}
	defer clear(key)
	channelsPath, cleanup, err := writeChannelRegistry(candidate)
	if err != nil {
		return QualificationResult{}, err
	}
	defer cleanup()
	workdir := filepath.Dir(channelsPath)
	channelID := fmt.Sprintf("candidate-%d", candidate.ID)
	stdout, err := e.execute(ctx, key, []string{
		e.ScriptPath, "run", "--channels", channelsPath, "--profile", e.QualificationProfilePath,
		"--runs", filepath.Join(workdir, "runs.jsonl"), "--decisions", filepath.Join(workdir, "decisions.jsonl"),
		"--channel", channelID, "--key-env", candidateKeyEnv,
	})
	if err != nil {
		return QualificationResult{}, err
	}
	var envelope struct {
		SchemaVersion int    `json:"schema_version"`
		RunID         string `json:"run_id"`
		ChannelID     string `json:"channel_id"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(stdout, &envelope); err != nil || envelope.SchemaVersion != 1 || envelope.RunID == "" || envelope.ChannelID != channelID || envelope.Status == "" {
		return QualificationResult{}, ErrMalformedOutput
	}
	return QualificationResult{RunID: envelope.RunID, Status: envelope.Status, Record: append(json.RawMessage(nil), stdout...)}, nil
}

func (e *V2Executor) execute(ctx context.Context, key []byte, args []string) ([]byte, error) {
	limit := e.MaxOutputBytes
	if limit <= 0 {
		limit = 2 << 20
	}
	stdout := &cappedBuffer{limit: limit}
	stderr := &cappedBuffer{limit: 4 << 10}
	cmd := exec.CommandContext(ctx, e.RubyPath, args...)
	cmd.Env = []string{"LANG=C.UTF-8", "TZ=UTC", candidateKeyEnv + "=" + string(key)}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if stdout.overflow {
		return nil, ErrOutputTooLarge
	}
	if runErr != nil {
		return nil, ErrProbeFailed
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func secretPath(reference string) (string, error) {
	if !strings.HasPrefix(reference, "file:") {
		return "", fmt.Errorf("candidate probe secret reference must use file scheme")
	}
	path := filepath.Clean(strings.TrimPrefix(reference, "file:"))
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("candidate probe secret path must be absolute")
	}
	return path, nil
}

func readSecret(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("candidate probe key unavailable")
	}
	permissions := info.Mode().Perm()
	if !info.Mode().IsRegular() || (permissions != 0o600 && permissions != 0o640) || info.Size() <= 0 || info.Size() > maxProbeKeyBytes {
		return nil, fmt.Errorf("candidate probe key permissions or size are invalid")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("candidate probe key unavailable")
	}
	value = bytes.TrimSpace(value)
	if len(value) < 4 {
		clear(value)
		return nil, fmt.Errorf("candidate probe key is invalid")
	}
	return value, nil
}

func writeChannelRegistry(candidate candidates.Candidate) (string, func(), error) {
	directory, err := os.MkdirTemp("", "relay-ops-watch-")
	if err != nil {
		return "", nil, fmt.Errorf("create candidate probe workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	path := filepath.Join(directory, "channels.json")
	document := map[string]any{
		"schema_version": 1,
		"channels": []map[string]any{{
			"id": fmt.Sprintf("candidate-%d", candidate.ID), "display_name": candidate.Name,
			"base_url": candidate.BaseURL, "protocol": "openai_compatible",
			"resale_permission": "unknown", "lifecycle": "candidate",
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("encode candidate registry: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write candidate registry: %w", err)
	}
	return path, cleanup, nil
}

func clear(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

type cappedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (w *cappedBuffer) Write(value []byte) (int, error) {
	originalLength := len(value)
	if w.buffer.Len() < w.limit {
		remaining := w.limit - w.buffer.Len()
		if originalLength < remaining {
			remaining = originalLength
		}
		_, _ = w.buffer.Write(value[:remaining])
		value = value[remaining:]
	}
	if len(value) > 0 {
		w.overflow = true
	}
	return originalLength, nil
}

func (w *cappedBuffer) Bytes() []byte { return w.buffer.Bytes() }
