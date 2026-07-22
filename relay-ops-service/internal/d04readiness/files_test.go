package d04readiness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecodeBalanceEvidenceRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodeBalanceEvidence(strings.NewReader(`{"schema_version":3,"records":[],"active_upstream":"manual"}`))
	if err == nil {
		t.Fatal("unknown membership field was accepted")
	}
}

func TestDecodeSnapshotBaseRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodeSnapshotBase(strings.NewReader(`{"status":"fictional","approvals":{},"modes":{},"services":{},"d04":{},"account_backup":{},"operations":{},"active_upstreams":[]}`))
	if err == nil {
		t.Fatal("membership field was accepted in base snapshot")
	}
}

func TestWriteSnapshotDocumentAtomically(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	err := WriteSnapshotDocument(path, Snapshot{
		SchemaVersion: 3,
		SnapshotID:    "snapshot-1",
		CapturedAt:    now,
		UpstreamDiscovery: UpstreamDiscovery{
			Source: "sub2api_admin_accounts", RecordedAt: now, AccountSetSHA256: strings.Repeat("a", 64),
		},
		ActiveUpstreams: []ActiveUpstream{{AccountID: 7, Status: "active", Schedulable: true, RuntimeAvailable: true}},
	}, SnapshotBase{
		Status:        "fictional",
		Approvals:     map[string]any{"launch_approved": false},
		Modes:         map[string]any{"d04_mode": "read_only"},
		Services:      map[string]any{"sub2api": true},
		D04:           map[string]any{"registered_users": 0},
		AccountBackup: map[string]any{"sha256_verified": true},
		Operations:    map[string]any{"primary_owner": "site-owner"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"schema_version":3`, `"snapshot_id":"snapshot-1"`, `"active_upstreams":[`, `"account_id":7`} {
		if !strings.Contains(string(data), required) {
			t.Fatalf("snapshot missing %q: %s", required, data)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot mode = %o", info.Mode().Perm())
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains: %v", err)
	}
}
