package d04readiness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type SnapshotBase struct {
	Status        string         `json:"status"`
	Approvals     map[string]any `json:"approvals"`
	Modes         map[string]any `json:"modes"`
	Services      map[string]any `json:"services"`
	D04           map[string]any `json:"d04"`
	AccountBackup map[string]any `json:"account_backup"`
	Operations    map[string]any `json:"operations"`
}

type evidenceDocument[T any] struct {
	SchemaVersion int `json:"schema_version"`
	Records       []T `json:"records"`
}

type snapshotDocument struct {
	SchemaVersion     int               `json:"schema_version"`
	SnapshotID        string            `json:"snapshot_id"`
	Status            string            `json:"status"`
	CapturedAt        any               `json:"captured_at"`
	Approvals         map[string]any    `json:"approvals"`
	Modes             map[string]any    `json:"modes"`
	Services          map[string]any    `json:"services"`
	D04               map[string]any    `json:"d04"`
	UpstreamDiscovery UpstreamDiscovery `json:"upstream_discovery"`
	ActiveUpstreams   []ActiveUpstream  `json:"active_upstreams"`
	AccountBackup     map[string]any    `json:"account_backup"`
	Operations        map[string]any    `json:"operations"`
}

var forbiddenEvidenceKey = regexp.MustCompile(`(?i)^(api[_-]?key|token|access[_-]?token|refresh[_-]?token|cookie|authorization|password|private[_-]?key|client[_-]?secret|credentials?)$`)
var secretEvidenceValue = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+\S{16,}|cookie:\s*\S+|sk-[a-z0-9]{16,}|begin [a-z ]*private key)`)

func DecodeBalanceEvidence(reader io.Reader) ([]BalanceEvidence, error) {
	return decodeEvidence[BalanceEvidence](reader)
}

func DecodeQualityEvidence(reader io.Reader) ([]QualityEvidence, error) {
	return decodeEvidence[QualityEvidence](reader)
}

func DecodeSnapshotBase(reader io.Reader) (SnapshotBase, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 2<<20))
	decoder.DisallowUnknownFields()
	var base SnapshotBase
	if err := decoder.Decode(&base); err != nil {
		return SnapshotBase{}, fmt.Errorf("decode snapshot base: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return SnapshotBase{}, err
	}
	if strings.TrimSpace(base.Status) == "" {
		return SnapshotBase{}, fmt.Errorf("snapshot base status is required")
	}
	if err := rejectCredentialShapes(base); err != nil {
		return SnapshotBase{}, err
	}
	return base, nil
}

func decodeEvidence[T any](reader io.Reader) ([]T, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 2<<20))
	decoder.DisallowUnknownFields()
	var document evidenceDocument[T]
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode evidence: %w", err)
	}
	if document.SchemaVersion != 3 {
		return nil, fmt.Errorf("evidence schema_version must equal 3")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return document.Records, nil
}

func WriteSnapshotDocument(path string, snapshot Snapshot, base SnapshotBase) (err error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(base.Status) == "" {
		return fmt.Errorf("snapshot path and status are required")
	}
	document := snapshotDocument{
		SchemaVersion: snapshot.SchemaVersion, SnapshotID: snapshot.SnapshotID, Status: base.Status,
		CapturedAt: snapshot.CapturedAt, Approvals: base.Approvals, Modes: base.Modes,
		Services: base.Services, D04: base.D04, UpstreamDiscovery: snapshot.UpstreamDiscovery,
		ActiveUpstreams: snapshot.ActiveUpstreams, AccountBackup: base.AccountBackup, Operations: base.Operations,
	}
	if err := rejectCredentialShapes(document); err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open temporary snapshot: %w", err)
	}
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(temporary)
		}
	}()
	if err = file.Chmod(0o600); err != nil {
		return fmt.Errorf("restrict temporary snapshot: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(true)
	if err = encoder.Encode(document); err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync temporary snapshot: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close temporary snapshot: %w", err)
	}
	if err = os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace snapshot: %w", err)
	}
	directory, openErr := os.Open(filepath.Dir(path))
	if openErr != nil {
		return fmt.Errorf("open snapshot directory: %w", openErr)
	}
	defer directory.Close()
	if err = directory.Sync(); err != nil {
		return fmt.Errorf("sync snapshot directory: %w", err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("evidence contains multiple JSON documents")
		}
		return fmt.Errorf("decode evidence trailer: %w", err)
	}
	return nil
}

func rejectCredentialShapes(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode snapshot validation view: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("decode snapshot validation view: %w", err)
	}
	return scanCredentialShapes(decoded, "")
}

func scanCredentialShapes(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if forbiddenEvidenceKey.MatchString(key) {
				return fmt.Errorf("%s: credential fields are forbidden", childPath)
			}
			if err := scanCredentialShapes(child, childPath); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := scanCredentialShapes(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case string:
		if secretEvidenceValue.MatchString(typed) {
			return fmt.Errorf("%s: value looks like a secret", path)
		}
	}
	return nil
}
