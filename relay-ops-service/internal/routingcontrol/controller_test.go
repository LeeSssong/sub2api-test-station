package routingcontrol

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestLoadConfigAcceptsExactlyApprovedGroups(t *testing.T) {
	path := writeRoutingConfig(t, `{
  "groups": [
    {"name":"GPT-Pro","public_group_id":3,"primary_account_id":11,"backup_account_id":12,"required_models":["gpt-b","gpt-a"]},
    {"name":"GPT-Plus","public_group_id":4,"primary_account_id":21,"backup_account_id":22,"required_models":["gpt-c"]}
  ]
}`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Groups) != 2 || cfg.Groups[0].Name != "GPT-Pro" || !reflect.DeepEqual(cfg.Groups[0].RequiredModels, []string{"gpt-a", "gpt-b"}) {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestLoadConfigAcceptsAccountReusedOnlyAsBackup(t *testing.T) {
	path := writeRoutingConfig(t, `{
  "groups": [
    {"name":"GPT-Pro","public_group_id":3,"primary_account_id":11,"backup_account_id":12,"required_models":["gpt-a"]},
    {"name":"GPT-Plus","public_group_id":4,"primary_account_id":21,"backup_account_id":12,"required_models":["gpt-c"]}
  ]
}`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig shared backup: %v", err)
	}
	if cfg.Groups[0].BackupAccountID != 12 || cfg.Groups[1].BackupAccountID != 12 {
		t.Fatalf("shared backup was not preserved: %#v", cfg.Groups)
	}
}

func TestLoadConfigRejectsUnsafeOrAmbiguousConfiguration(t *testing.T) {
	validPlus := `{"name":"GPT-Plus","public_group_id":4,"primary_account_id":21,"backup_account_id":22,"required_models":["gpt-c"]}`
	tests := []struct {
		name string
		body string
	}{
		{"unknown field", `{"groups":[],"extra":true}`},
		{"trailing json", `{"groups":[]} {}`},
		{"missing group", `{"groups":[` + validPlus + `]}`},
		{"duplicate group id", `{"groups":[{"name":"GPT-Pro","public_group_id":4,"primary_account_id":11,"backup_account_id":12,"required_models":["gpt-a"]},` + validPlus + `]}`},
		{"same role account", `{"groups":[{"name":"GPT-Pro","public_group_id":3,"primary_account_id":11,"backup_account_id":11,"required_models":["gpt-a"]},` + validPlus + `]}`},
		{"duplicate primary", `{"groups":[{"name":"GPT-Pro","public_group_id":3,"primary_account_id":21,"backup_account_id":12,"required_models":["gpt-a"]},` + validPlus + `]}`},
		{"primary reused as backup", `{"groups":[{"name":"GPT-Pro","public_group_id":3,"primary_account_id":11,"backup_account_id":21,"required_models":["gpt-a"]},` + validPlus + `]}`},
		{"no models", `{"groups":[{"name":"GPT-Pro","public_group_id":3,"primary_account_id":11,"backup_account_id":12,"required_models":[]},` + validPlus + `]}`},
		{"duplicate model", `{"groups":[{"name":"GPT-Pro","public_group_id":3,"primary_account_id":11,"backup_account_id":12,"required_models":["gpt-a","gpt-a"]},` + validPlus + `]}`},
		{"nonpositive id", `{"groups":[{"name":"GPT-Pro","public_group_id":0,"primary_account_id":11,"backup_account_id":12,"required_models":["gpt-a"]},` + validPlus + `]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := LoadConfig(writeRoutingConfig(t, tt.body)); err == nil {
				t.Fatal("invalid config unexpectedly accepted")
			}
		})
	}

	t.Run("insecure mode", func(t *testing.T) {
		path := writeRoutingConfig(t, `{"groups":[]}`)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil {
			t.Fatal("world-readable config unexpectedly accepted")
		}
	})
}

func TestSwitchAddsAndVerifiesTargetBeforeRemovingSource(t *testing.T) {
	fake := baseFake()
	controller := newTestController(t, fake)

	result := controller.Switch(context.Background(), "GPT-Pro", RoleBackup, false)
	if result.Status != StatusSucceeded || result.ErrorCode != "" || result.Before.CurrentRole != RolePrimary || result.After.CurrentRole != RoleBackup {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(fake.accounts[12].GroupIDs, []int64{3, 9}) {
		t.Fatalf("backup groups = %v", fake.accounts[12].GroupIDs)
	}
	if !reflect.DeepEqual(fake.accounts[11].GroupIDs, []int64{8}) {
		t.Fatalf("primary groups = %v", fake.accounts[11].GroupIDs)
	}
	targetWrite := indexOf(fake.calls, "set-groups:12:[3 9]")
	targetVerify := indexAfter(fake.calls, "get-account:12", targetWrite)
	sourceWrite := indexOf(fake.calls, "set-groups:11:[8]")
	if targetWrite < 0 || targetVerify < targetWrite || sourceWrite < targetVerify {
		t.Fatalf("unsafe call order: %v", fake.calls)
	}
}

func TestSwitchSharedBackupPreservesOtherGroupBinding(t *testing.T) {
	fake := baseFake()
	backup := fake.accounts[12]
	backup.GroupIDs = []int64{3, 9}
	fake.accounts[12] = backup
	fake.models[12] = append(fake.models[12], sub2api.Model{ID: "gpt-c"})
	controller := &Controller{
		Client: fake,
		Config: Config{Groups: []GroupRoute{
			{Name: "GPT-Pro", PublicGroupID: 3, PrimaryAccountID: 11, BackupAccountID: 12, RequiredModels: []string{"gpt-a", "gpt-b"}},
			{Name: "GPT-Plus", PublicGroupID: 4, PrimaryAccountID: 21, BackupAccountID: 12, RequiredModels: []string{"gpt-c"}},
		}},
		Now: func() time.Time { return time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC) },
	}

	switched := controller.Switch(context.Background(), "GPT-Plus", RoleBackup, false)
	if switched.Status != StatusSucceeded || !reflect.DeepEqual(fake.accounts[12].GroupIDs, []int64{3, 4, 9}) {
		t.Fatalf("switch result=%#v shared groups=%v", switched, fake.accounts[12].GroupIDs)
	}
	restored := controller.Switch(context.Background(), "GPT-Plus", RolePrimary, false)
	if restored.Status != StatusSucceeded || !reflect.DeepEqual(fake.accounts[12].GroupIDs, []int64{3, 9}) {
		t.Fatalf("restore result=%#v shared groups=%v", restored, fake.accounts[12].GroupIDs)
	}
}

func TestSwitchEnablesUnschedulableTargetBeforeBinding(t *testing.T) {
	fake := baseFake()
	backup := fake.accounts[12]
	backup.Schedulable = false
	fake.accounts[12] = backup

	result := newTestController(t, fake).Switch(context.Background(), "GPT-Pro", RoleBackup, false)
	if result.Status != StatusSucceeded {
		t.Fatalf("result = %#v", result)
	}
	enable := indexOf(fake.calls, "set-schedulable:12:true")
	bind := indexOf(fake.calls, "set-groups:12:[3 9]")
	if enable < 0 || bind < enable {
		t.Fatalf("call order = %v", fake.calls)
	}
}

func TestSwitchNoOpAndDryRunNeverWrite(t *testing.T) {
	t.Run("no op", func(t *testing.T) {
		fake := baseFake()
		result := newTestController(t, fake).Switch(context.Background(), "GPT-Pro", RolePrimary, false)
		if result.Status != StatusNoOp || fake.writeCount() != 0 {
			t.Fatalf("result = %#v calls = %v", result, fake.calls)
		}
	})

	t.Run("dry run", func(t *testing.T) {
		fake := baseFake()
		result := newTestController(t, fake).Switch(context.Background(), "GPT-Pro", RoleBackup, true)
		if result.Status != StatusSucceeded || !result.DryRun || result.After.CurrentRole != RoleBackup || fake.writeCount() != 0 {
			t.Fatalf("result = %#v calls = %v", result, fake.calls)
		}
	})
}

func TestSwitchRejectsPreflightFailureWithoutWrites(t *testing.T) {
	future := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*fakeSub2API)
		code   string
	}{
		{"missing model", func(fake *fakeSub2API) { fake.models[12] = []sub2api.Model{{ID: "other"}} }, ErrorModelUnavailable},
		{"inactive target", func(fake *fakeSub2API) {
			account := fake.accounts[12]
			account.Status = "error"
			fake.accounts[12] = account
		}, ErrorTargetUnavailable},
		{"missing credentials", func(fake *fakeSub2API) {
			account := fake.accounts[12]
			account.CredentialsStatus = map[string]bool{}
			fake.accounts[12] = account
		}, ErrorTargetUnavailable},
		{"platform mismatch", func(fake *fakeSub2API) {
			account := fake.accounts[12]
			account.Platform = "anthropic"
			fake.accounts[12] = account
		}, ErrorConfigMismatch},
		{"temporarily unschedulable", func(fake *fakeSub2API) {
			account := fake.accounts[12]
			account.TempUnschedulableUntil = &future
			fake.accounts[12] = account
		}, ErrorTargetUnavailable},
		{"overloaded", func(fake *fakeSub2API) {
			account := fake.accounts[12]
			account.OverloadUntil = &future
			fake.accounts[12] = account
		}, ErrorTargetUnavailable},
		{"rate limited", func(fake *fakeSub2API) {
			account := fake.accounts[12]
			account.RateLimitResetAt = &future
			fake.accounts[12] = account
		}, ErrorTargetUnavailable},
		{"expired", func(fake *fakeSub2API) {
			account := fake.accounts[12]
			expires := future.Add(-2 * time.Hour).Unix()
			account.ExpiresAt = &expires
			account.AutoPauseOnExpired = true
			fake.accounts[12] = account
		}, ErrorTargetUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := baseFake()
			tt.mutate(fake)
			result := newTestController(t, fake).Switch(context.Background(), "GPT-Pro", RoleBackup, false)
			if result.Status != StatusRejected || result.ErrorCode != tt.code || fake.writeCount() != 0 {
				t.Fatalf("result = %#v calls = %v", result, fake.calls)
			}
		})
	}
}

func TestSwitchUsesRereadToResolveUnknownWriteResponses(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeSub2API)
	}{
		{
			name: "schedulable response lost",
			configure: func(fake *fakeSub2API) {
				account := fake.accounts[12]
				account.Schedulable = false
				fake.accounts[12] = account
				fake.failAfter["set-schedulable:12"] = errors.New("response lost")
			},
		},
		{
			name: "target binding response lost",
			configure: func(fake *fakeSub2API) {
				fake.failAfter["set-groups:12"] = errors.New("response lost")
			},
		},
		{
			name: "source binding response lost",
			configure: func(fake *fakeSub2API) {
				fake.failAfter["set-groups:11"] = errors.New("response lost")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := baseFake()
			tt.configure(fake)
			result := newTestController(t, fake).Switch(context.Background(), "GPT-Pro", RoleBackup, false)
			if result.Status != StatusSucceeded || result.After.CurrentRole != RoleBackup {
				t.Fatalf("result = %#v calls = %v", result, fake.calls)
			}
		})
	}
}

func TestSwitchTargetFailureLeavesSourceAndSourceFailureIsPartial(t *testing.T) {
	t.Run("target failure", func(t *testing.T) {
		fake := baseFake()
		fake.failBefore["set-groups:12"] = errors.New("target failed")
		result := newTestController(t, fake).Switch(context.Background(), "GPT-Pro", RoleBackup, false)
		if result.Status != StatusFailed || result.ErrorCode != ErrorTargetWrite || !contains(fake.accounts[11].GroupIDs, 3) {
			t.Fatalf("result = %#v accounts = %#v", result, fake.accounts)
		}
	})

	t.Run("source failure", func(t *testing.T) {
		fake := baseFake()
		fake.failBefore["set-groups:11"] = errors.New("source failed")
		result := newTestController(t, fake).Switch(context.Background(), "GPT-Pro", RoleBackup, false)
		if result.Status != StatusPartial || result.ErrorCode != ErrorSourceWrite || result.After.CurrentRole != RoleMixed {
			t.Fatalf("result = %#v calls = %v", result, fake.calls)
		}
		if !contains(fake.accounts[11].GroupIDs, 3) || !contains(fake.accounts[12].GroupIDs, 3) {
			t.Fatalf("partial state was rolled back: %#v", fake.accounts)
		}
	})
}

func TestReadAllReportsMixedAndNoneExplicitly(t *testing.T) {
	fake := baseFake()
	backup := fake.accounts[12]
	backup.GroupIDs = append(backup.GroupIDs, 3)
	fake.accounts[12] = backup
	primaryPlus := fake.accounts[21]
	primaryPlus.GroupIDs = []int64{99}
	fake.accounts[21] = primaryPlus

	states, err := newTestController(t, fake).ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if states[0].CurrentRole != RoleMixed || states[1].CurrentRole != RoleNone {
		t.Fatalf("states = %#v", states)
	}
}

type fakeSub2API struct {
	groups     map[int64]sub2api.Group
	accounts   map[int64]sub2api.Account
	models     map[int64][]sub2api.Model
	calls      []string
	failBefore map[string]error
	failAfter  map[string]error
}

func baseFake() *fakeSub2API {
	credential := map[string]bool{"has_api_key": true}
	return &fakeSub2API{
		groups: map[int64]sub2api.Group{
			3: {ID: 3, Name: "GPT-Pro", Platform: "openai", Status: "active", RateMultiplier: 1},
			4: {ID: 4, Name: "GPT-Plus", Platform: "openai", Status: "active", RateMultiplier: 1},
		},
		accounts: map[int64]sub2api.Account{
			11: {ID: 11, Name: "pro-primary", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{3, 8}, CredentialsStatus: credential},
			12: {ID: 12, Name: "pro-backup", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{9}, CredentialsStatus: credential},
			21: {ID: 21, Name: "plus-primary", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{4}, CredentialsStatus: credential},
			22: {ID: 22, Name: "plus-backup", Platform: "openai", Status: "active", Schedulable: true, GroupIDs: []int64{10}, CredentialsStatus: credential},
		},
		models: map[int64][]sub2api.Model{
			11: {{ID: "gpt-a"}, {ID: "gpt-b"}}, 12: {{ID: "gpt-a"}, {ID: "gpt-b"}},
			21: {{ID: "gpt-c"}}, 22: {{ID: "gpt-c"}},
		},
		failBefore: map[string]error{},
		failAfter:  map[string]error{},
	}
}

func (f *fakeSub2API) GetGroup(_ context.Context, id int64) (sub2api.Group, error) {
	f.calls = append(f.calls, fmt.Sprintf("get-group:%d", id))
	group, ok := f.groups[id]
	if !ok {
		return sub2api.Group{}, errors.New("not found")
	}
	return group, nil
}

func (f *fakeSub2API) GetAccount(_ context.Context, id int64) (sub2api.Account, error) {
	f.calls = append(f.calls, fmt.Sprintf("get-account:%d", id))
	account, ok := f.accounts[id]
	if !ok {
		return sub2api.Account{}, errors.New("not found")
	}
	account.GroupIDs = append([]int64(nil), account.GroupIDs...)
	return account, nil
}

func (f *fakeSub2API) GetAccountModels(_ context.Context, id int64) ([]sub2api.Model, error) {
	f.calls = append(f.calls, fmt.Sprintf("get-models:%d", id))
	return append([]sub2api.Model(nil), f.models[id]...), nil
}

func (f *fakeSub2API) SetAccountGroups(_ context.Context, id int64, groupIDs []int64) (sub2api.Account, error) {
	f.calls = append(f.calls, fmt.Sprintf("set-groups:%d:%v", id, groupIDs))
	if err := f.failBefore[fmt.Sprintf("set-groups:%d", id)]; err != nil {
		return sub2api.Account{}, err
	}
	account := f.accounts[id]
	account.GroupIDs = append([]int64(nil), groupIDs...)
	f.accounts[id] = account
	if err := f.failAfter[fmt.Sprintf("set-groups:%d", id)]; err != nil {
		return sub2api.Account{}, err
	}
	return account, nil
}

func (f *fakeSub2API) SetAccountSchedulable(_ context.Context, id int64, schedulable bool) (sub2api.Account, error) {
	f.calls = append(f.calls, fmt.Sprintf("set-schedulable:%d:%t", id, schedulable))
	if err := f.failBefore[fmt.Sprintf("set-schedulable:%d", id)]; err != nil {
		return sub2api.Account{}, err
	}
	account := f.accounts[id]
	account.Schedulable = schedulable
	f.accounts[id] = account
	if err := f.failAfter[fmt.Sprintf("set-schedulable:%d", id)]; err != nil {
		return sub2api.Account{}, err
	}
	return account, nil
}

func (f *fakeSub2API) writeCount() int {
	count := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, "set-") {
			count++
		}
	}
	return count
}

func newTestController(t *testing.T, fake *fakeSub2API) *Controller {
	t.Helper()
	cfg := Config{Groups: []GroupRoute{
		{Name: "GPT-Pro", PublicGroupID: 3, PrimaryAccountID: 11, BackupAccountID: 12, RequiredModels: []string{"gpt-a", "gpt-b"}},
		{Name: "GPT-Plus", PublicGroupID: 4, PrimaryAccountID: 21, BackupAccountID: 22, RequiredModels: []string{"gpt-c"}},
	}}
	return &Controller{Client: fake, Config: cfg, Now: func() time.Time { return time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC) }}
}

func writeRoutingConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routing.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func indexAfter(values []string, want string, after int) int {
	for i := after + 1; i < len(values); i++ {
		if values[i] == want {
			return i
		}
	}
	return -1
}

func contains(values []int64, want int64) bool {
	i := sort.Search(len(values), func(i int) bool { return values[i] >= want })
	return i < len(values) && values[i] == want
}
