package routingcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

type Role string

const (
	RolePrimary Role = "primary"
	RoleBackup  Role = "backup"
	RoleMixed   Role = "mixed"
	RoleNone    Role = "none"

	StatusSucceeded = "succeeded"
	StatusNoOp      = "no_op"
	StatusPartial   = "partial"
	StatusFailed    = "failed"
	StatusRejected  = "rejected"

	ErrorConfigMismatch    = "config_mismatch"
	ErrorStateAmbiguous    = "state_ambiguous"
	ErrorTargetUnavailable = "target_unavailable"
	ErrorModelUnavailable  = "model_unavailable"
	ErrorTargetWrite       = "target_write_failed"
	ErrorTargetVerify      = "target_verify_failed"
	ErrorSourceWrite       = "source_write_failed"
	ErrorFinalVerify       = "final_verify_failed"
	ErrorReadFailed        = "read_failed"
)

type Config struct {
	Groups []GroupRoute `json:"groups"`
}

type GroupRoute struct {
	Name             string   `json:"name"`
	PublicGroupID    int64    `json:"public_group_id"`
	PrimaryAccountID int64    `json:"primary_account_id"`
	BackupAccountID  int64    `json:"backup_account_id"`
	RequiredModels   []string `json:"required_models"`
}

type GroupState struct {
	GroupName       string  `json:"group_name"`
	GroupID         int64   `json:"group_id"`
	CurrentRole     Role    `json:"current_role"`
	PrimaryBound    bool    `json:"primary_bound"`
	BackupBound     bool    `json:"backup_bound"`
	PrimaryEligible bool    `json:"primary_eligible"`
	BackupEligible  bool    `json:"backup_eligible"`
	RateMultiplier  float64 `json:"rate_multiplier"`
}

type Result struct {
	Status     string     `json:"status"`
	ErrorCode  string     `json:"error_code,omitempty"`
	GroupName  string     `json:"group_name"`
	TargetRole Role       `json:"target_role,omitempty"`
	DryRun     bool       `json:"dry_run"`
	Before     GroupState `json:"before"`
	After      GroupState `json:"after"`
}

type Controller struct {
	Client sub2api.Controller
	Config Config
	Now    func() time.Time
}

var errConfigMismatch = errors.New("routing objects do not match configuration")

func LoadConfig(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, fmt.Errorf("read routing config metadata: %w", err)
	}
	if !info.Mode().IsRegular() || (info.Mode().Perm() != 0o600 && info.Mode().Perm() != 0o640) {
		return Config{}, errors.New("routing config must be a regular 0600 or 0640 file")
	}
	if info.Size() <= 0 || info.Size() > 64<<10 {
		return Config{}, errors.New("routing config has invalid size")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read routing config: %w", err)
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, errors.New("routing config JSON is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("routing config contains trailing JSON")
	}
	if err := validateConfig(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg == nil || len(cfg.Groups) != 2 {
		return errors.New("routing config must contain exactly GPT-Pro and GPT-Plus")
	}
	byName := make(map[string]GroupRoute, 2)
	groupIDs := map[int64]struct{}{}
	primaryAccountIDs := map[int64]struct{}{}
	backupAccountIDs := map[int64]struct{}{}
	for _, route := range cfg.Groups {
		if route.Name != "GPT-Pro" && route.Name != "GPT-Plus" {
			return fmt.Errorf("unsupported routing group %q", route.Name)
		}
		if _, exists := byName[route.Name]; exists {
			return fmt.Errorf("duplicate routing group %q", route.Name)
		}
		if route.PublicGroupID <= 0 || route.PrimaryAccountID <= 0 || route.BackupAccountID <= 0 || route.PrimaryAccountID == route.BackupAccountID {
			return fmt.Errorf("routing IDs are invalid for %s", route.Name)
		}
		if _, exists := groupIDs[route.PublicGroupID]; exists {
			return errors.New("routing group IDs must be unique")
		}
		groupIDs[route.PublicGroupID] = struct{}{}
		if _, exists := primaryAccountIDs[route.PrimaryAccountID]; exists {
			return errors.New("routing primary account IDs must be unique")
		}
		primaryAccountIDs[route.PrimaryAccountID] = struct{}{}
		backupAccountIDs[route.BackupAccountID] = struct{}{}
		if len(route.RequiredModels) == 0 {
			return fmt.Errorf("required models are missing for %s", route.Name)
		}
		models := append([]string(nil), route.RequiredModels...)
		sort.Strings(models)
		for i, model := range models {
			if strings.TrimSpace(model) == "" || (i > 0 && model == models[i-1]) {
				return fmt.Errorf("required models are invalid for %s", route.Name)
			}
		}
		route.RequiredModels = models
		byName[route.Name] = route
	}
	for id := range primaryAccountIDs {
		if _, exists := backupAccountIDs[id]; exists {
			return errors.New("routing primary accounts must not be reused as backups")
		}
	}
	pro, hasPro := byName["GPT-Pro"]
	plus, hasPlus := byName["GPT-Plus"]
	if !hasPro || !hasPlus {
		return errors.New("routing config must contain exactly GPT-Pro and GPT-Plus")
	}
	cfg.Groups = []GroupRoute{pro, plus}
	return nil
}

func (c *Controller) ReadAll(ctx context.Context) ([]GroupState, error) {
	states := make([]GroupState, 0, len(c.Config.Groups))
	for _, route := range c.Config.Groups {
		state, _, _, err := c.readRoute(ctx, route)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (c *Controller) Switch(ctx context.Context, groupName string, targetRole Role, dryRun bool) Result {
	result := Result{Status: StatusRejected, ErrorCode: ErrorConfigMismatch, GroupName: groupName, TargetRole: targetRole, DryRun: dryRun}
	route, ok := c.route(groupName)
	if !ok || (targetRole != RolePrimary && targetRole != RoleBackup) || c.Client == nil {
		return result
	}
	before, primary, backup, err := c.readRoute(ctx, route)
	result.Before = before
	result.After = before
	if err != nil {
		if errors.Is(err, errConfigMismatch) {
			result.Status = StatusRejected
			result.ErrorCode = ErrorConfigMismatch
			return result
		}
		result.Status = StatusFailed
		result.ErrorCode = ErrorReadFailed
		return result
	}
	if before.CurrentRole == targetRole {
		result.Status = StatusNoOp
		result.ErrorCode = ""
		return result
	}
	if before.CurrentRole != RolePrimary && before.CurrentRole != RoleBackup {
		result.ErrorCode = ErrorStateAmbiguous
		return result
	}
	target, source := backup, primary
	if targetRole == RolePrimary {
		target, source = primary, backup
	}
	if target.Status != "active" || target.Platform != primary.Platform || target.Platform != backup.Platform || !hasCredential(target.CredentialsStatus) || runtimeBlocked(target, c.now()) {
		result.ErrorCode = ErrorTargetUnavailable
		if target.Platform != primary.Platform || target.Platform != backup.Platform {
			result.ErrorCode = ErrorConfigMismatch
		}
		return result
	}
	models, err := c.Client.GetAccountModels(ctx, target.ID)
	if err != nil || !modelsContain(models, route.RequiredModels) {
		result.ErrorCode = ErrorModelUnavailable
		return result
	}
	if dryRun {
		result.Status = StatusSucceeded
		result.ErrorCode = ""
		result.After = predictedState(before, targetRole)
		return result
	}

	if !target.Schedulable {
		if _, err := c.Client.SetAccountSchedulable(ctx, target.ID, true); err != nil {
			verified, readErr := c.Client.GetAccount(ctx, target.ID)
			if readErr != nil {
				result.Status = StatusPartial
				result.ErrorCode = ErrorTargetWrite
				return result
			}
			if !verified.Schedulable {
				result.Status = StatusFailed
				result.ErrorCode = ErrorTargetWrite
				return result
			}
			target = verified
		} else {
			target.Schedulable = true
		}
	}
	targetWriteErr := error(nil)
	if _, err := c.Client.SetAccountGroups(ctx, target.ID, addGroup(target.GroupIDs, route.PublicGroupID)); err != nil {
		targetWriteErr = err
	}
	afterTarget, _, _, err := c.readRoute(ctx, route)
	result.After = afterTarget
	if err != nil {
		result.Status = StatusPartial
		result.ErrorCode = ErrorTargetVerify
		if targetWriteErr != nil {
			result.ErrorCode = ErrorTargetWrite
		}
		return result
	}
	if afterTarget.CurrentRole != RoleMixed && afterTarget.CurrentRole != targetRole {
		result.Status = StatusFailed
		result.ErrorCode = ErrorTargetWrite
		if targetWriteErr == nil {
			result.ErrorCode = ErrorTargetVerify
		}
		return result
	}
	if afterTarget.RateMultiplier != before.RateMultiplier {
		result.Status = StatusPartial
		result.ErrorCode = ErrorTargetVerify
		return result
	}
	sourceWriteErr := error(nil)
	if _, err := c.Client.SetAccountGroups(ctx, source.ID, removeGroup(source.GroupIDs, route.PublicGroupID)); err != nil {
		sourceWriteErr = err
	}
	final, _, _, err := c.readRoute(ctx, route)
	result.After = final
	if err != nil || final.CurrentRole != targetRole || final.RateMultiplier != before.RateMultiplier {
		result.Status = StatusPartial
		result.ErrorCode = ErrorFinalVerify
		if sourceWriteErr != nil {
			result.ErrorCode = ErrorSourceWrite
		}
		return result
	}
	result.Status = StatusSucceeded
	result.ErrorCode = ""
	return result
}

func (c *Controller) route(name string) (GroupRoute, bool) {
	for _, route := range c.Config.Groups {
		if route.Name == name {
			return route, true
		}
	}
	return GroupRoute{}, false
}

func (c *Controller) readRoute(ctx context.Context, route GroupRoute) (GroupState, sub2api.Account, sub2api.Account, error) {
	group, err := c.Client.GetGroup(ctx, route.PublicGroupID)
	if err != nil {
		return GroupState{}, sub2api.Account{}, sub2api.Account{}, err
	}
	primary, err := c.Client.GetAccount(ctx, route.PrimaryAccountID)
	if err != nil {
		return GroupState{}, sub2api.Account{}, sub2api.Account{}, err
	}
	backup, err := c.Client.GetAccount(ctx, route.BackupAccountID)
	if err != nil {
		return GroupState{}, sub2api.Account{}, sub2api.Account{}, err
	}
	if group.ID != route.PublicGroupID || group.Name != route.Name || group.Status != "active" || primary.ID != route.PrimaryAccountID || backup.ID != route.BackupAccountID || primary.Platform != group.Platform || backup.Platform != group.Platform {
		return GroupState{}, sub2api.Account{}, sub2api.Account{}, errConfigMismatch
	}
	state := GroupState{
		GroupName:       route.Name,
		GroupID:         route.PublicGroupID,
		PrimaryBound:    hasID(primary.GroupIDs, route.PublicGroupID),
		BackupBound:     hasID(backup.GroupIDs, route.PublicGroupID),
		PrimaryEligible: serves(primary, route.PublicGroupID, c.now()),
		BackupEligible:  serves(backup, route.PublicGroupID, c.now()),
		RateMultiplier:  group.RateMultiplier,
	}
	switch {
	case state.PrimaryEligible && state.BackupEligible:
		state.CurrentRole = RoleMixed
	case state.PrimaryEligible:
		state.CurrentRole = RolePrimary
	case state.BackupEligible:
		state.CurrentRole = RoleBackup
	default:
		state.CurrentRole = RoleNone
	}
	return state, primary, backup, nil
}

func serves(account sub2api.Account, groupID int64, now time.Time) bool {
	return account.Status == "active" && account.Schedulable && !runtimeBlocked(account, now) && hasID(account.GroupIDs, groupID)
}

func runtimeBlocked(account sub2api.Account, now time.Time) bool {
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
		return true
	}
	if account.OverloadUntil != nil && account.OverloadUntil.After(now) {
		return true
	}
	if account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now) {
		return true
	}
	return account.AutoPauseOnExpired && account.ExpiresAt != nil && *account.ExpiresAt <= now.Unix()
}

func (c *Controller) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func hasCredential(status map[string]bool) bool {
	for _, present := range status {
		if present {
			return true
		}
	}
	return false
}

func modelsContain(models []sub2api.Model, required []string) bool {
	available := make(map[string]struct{}, len(models))
	for _, model := range models {
		available[model.ID] = struct{}{}
	}
	for _, model := range required {
		if _, ok := available[model]; !ok {
			return false
		}
	}
	return true
}

func addGroup(values []int64, groupID int64) []int64 {
	result := append([]int64(nil), values...)
	if !hasID(result, groupID) {
		result = append(result, groupID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func removeGroup(values []int64, groupID int64) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value != groupID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func hasID(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func predictedState(before GroupState, target Role) GroupState {
	after := before
	after.CurrentRole = target
	if target == RolePrimary {
		after.PrimaryBound = true
		after.PrimaryEligible = true
		after.BackupBound = false
		after.BackupEligible = false
	} else {
		after.PrimaryBound = false
		after.PrimaryEligible = false
		after.BackupBound = true
		after.BackupEligible = true
	}
	return after
}
