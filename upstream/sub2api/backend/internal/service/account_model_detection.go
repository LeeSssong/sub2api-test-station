package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrAccountModelDetectionUnsupported = errors.New("account model detection unsupported")
var ErrAccountModelDetectionUnavailable = errors.New("account model detection unavailable")

type AccountModelDetectionAccountReader interface {
	GetByID(context.Context, int64) (*Account, error)
	ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error)
}

type AccountModelDetectionRepository interface {
	LoadSettings(context.Context, int64) (AccountModelDetectionSettings, error)
	SaveSettings(context.Context, AccountModelDetectionSettings) error
	Enqueue(context.Context, AccountModelDetectionRun) (AccountModelDetectionRun, bool, error)
	ListQueued(context.Context, int) ([]string, error)
	Claim(context.Context, string) (*AccountModelDetectionRun, error)
	Complete(context.Context, string, AccountModelDetectionResponse, string, string) error
	ListRecent(context.Context, int64, int, string, string, string, string, string, string) (AccountModelDetectionHistoryPage, error)
}

type AccountModelDetectionSidecar interface {
	Catalog(context.Context) (AccountModelDetectionSidecarCatalog, error)
	Detect(context.Context, AccountModelDetectionRequest) (AccountModelDetectionResponse, error)
}

type AccountModelDetectionModelsResponse struct {
	AccountID            int64                              `json:"account_id"`
	DetectorState        AccountModelDetectorState          `json:"detector_state"`
	ConnectionProbeModel string                             `json:"connection_probe_model"`
	ModelDetectionModel  string                             `json:"model_detection_model"`
	ConnectionModels     []AccountModelDetectionModelOption `json:"connection_models"`
	DetectionModels      []AccountModelDetectionModelOption `json:"detection_models"`
}

type AccountModelDetectionService struct {
	repo           AccountModelDetectionRepository
	accounts       AccountModelDetectionAccountReader
	sidecar        AccountModelDetectionSidecar
	usage          ActiveProbeUsageWindowReader
	location       *time.Location
	now            func() time.Time
	executionSlots chan struct{}
	catalogMu      sync.Mutex
	catalogAt      time.Time
	catalog        []string
	catalogState   AccountModelDetectorState
	escalationMu   sync.Mutex
	escalationAt   map[int64]time.Time
}

func NewAccountModelDetectionService(repo AccountModelDetectionRepository, accounts AccountModelDetectionAccountReader, sidecar AccountModelDetectionSidecar) *AccountModelDetectionService {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return &AccountModelDetectionService{repo: repo, accounts: accounts, sidecar: sidecar, location: location, now: time.Now, executionSlots: make(chan struct{}, 4), escalationAt: make(map[int64]time.Time)}
}

// SetActiveProbeUsageReader injects the authoritative usage reader used to
// fail closed before scheduled detection runs contact an upstream account.
func (s *AccountModelDetectionService) SetActiveProbeUsageReader(reader ActiveProbeUsageWindowReader) {
	if s != nil {
		s.usage = reader
	}
}

func newAccountModelDetectionRun(accountID int64, modelID, profile, mode, reason string) AccountModelDetectionRun {
	planned := map[string]int{
		AccountModelDetectionProfileLow:    19,
		AccountModelDetectionProfileMedium: 49,
		AccountModelDetectionProfileHigh:   158,
	}[profile]
	return AccountModelDetectionRun{
		ID: uuid.NewString(), AccountID: accountID, ModelID: modelID, ClaimedModel: modelID,
		Profile: profile, Mode: mode, TriggerReason: reason, PlannedRequests: planned,
		EvidenceState: AccountModelDetectionEvidenceUnavailable,
		Status:        AccountModelDetectionStatusQueued, QueuedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}
}

func (s *AccountModelDetectionService) Models(ctx context.Context, accountID int64) (AccountModelDetectionModelsResponse, error) {
	if s == nil || s.accounts == nil {
		return AccountModelDetectionModelsResponse{}, errors.New("account model detection is unavailable")
	}
	account, err := s.accounts.GetByID(ctx, accountID)
	if err != nil || account == nil {
		if err == nil {
			err = errors.New("account not found")
		}
		return AccountModelDetectionModelsResponse{}, err
	}
	return s.modelsForAccount(ctx, account)
}

func (s *AccountModelDetectionService) LoadSettings(ctx context.Context, accountID int64) (AccountModelDetectionSettings, error) {
	if s == nil || s.repo == nil {
		return AccountModelDetectionSettings{AccountID: accountID}, errors.New("account model detection is unavailable")
	}
	return s.repo.LoadSettings(ctx, accountID)
}

func (s *AccountModelDetectionService) ProjectionForAccount(ctx context.Context, account *Account) (AccountModelDetectionProjection, error) {
	models, err := s.modelsForAccount(ctx, account)
	if err != nil {
		return AccountModelDetectionProjection{}, err
	}
	recent, err := s.Recent(ctx, account.ID, 20)
	if err != nil {
		return AccountModelDetectionProjection{}, err
	}
	status := AccountModelDetectionStatusUntested
	if account.Type != AccountTypeAPIKey {
		status = AccountModelDetectionStatusUnsupported
	} else if models.DetectorState == AccountModelDetectorStateUnconfigured {
		status = AccountModelDetectionStatusServiceUnconfigured
	} else if models.DetectorState == AccountModelDetectorStateUnavailable {
		status = AccountModelDetectionStatusServiceUnavailable
	} else if len(models.DetectionModels) == 0 || models.ModelDetectionModel == "" {
		status = AccountModelDetectionStatusUnsupported
	}
	var summary *AccountModelDetectionSummary
	var currentSummary *AccountModelDetectionSummary
	if len(recent) > 0 {
		current := recent[0]
		currentSummary = accountModelDetectionSummary(current, "current")
		effective := current
		effectiveSource := "current"
		if !accountModelDetectionHasFinalEvidence(current) {
			for _, candidate := range recent[1:] {
				if accountModelDetectionHasFinalEvidence(candidate) {
					effective = candidate
					effectiveSource = "historical_final"
					break
				}
			}
		}
		run := effective
		status = current.Status
		if current.Status == "" {
			status = AccountModelDetectionStatusFailed
		}
		summary = accountModelDetectionSummary(run, effectiveSource)
	}
	if account.Type != AccountTypeAPIKey {
		status = AccountModelDetectionStatusUnsupported
	}
	settings := AccountModelDetectionSettings{AccountID: account.ID, ConnectionProbeModel: models.ConnectionProbeModel, ModelDetectionModel: models.ModelDetectionModel}
	if loaded, loadErr := s.LoadSettings(ctx, account.ID); loadErr == nil {
		settings = loaded
	}
	if account.Type == AccountTypeAPIKey {
		if models.DetectorState == AccountModelDetectorStateUnconfigured {
			status = AccountModelDetectionStatusServiceUnconfigured
		} else if models.DetectorState == AccountModelDetectorStateUnavailable {
			status = AccountModelDetectionStatusServiceUnavailable
		}
	}
	return AccountModelDetectionProjection{Status: status, DetectorState: models.DetectorState, Settings: settings, ModelOptions: models.DetectionModels, Recent: summary, Current: currentSummary}, nil
}

func accountModelDetectionHasFinalEvidence(run AccountModelDetectionRun) bool {
	if run.FinishedAt == nil {
		return false
	}
	return run.Status == AccountModelDetectionStatusNormal || run.Status == AccountModelDetectionStatusAbnormal
}

func accountModelDetectionSummary(run AccountModelDetectionRun, source string) *AccountModelDetectionSummary {
	if accountModelDetectionIsHistorical(run) && source == "current" {
		source = "historical"
	}
	queued := run.QueuedAt
	return &AccountModelDetectionSummary{
		Status: run.Status, ModelID: run.ModelID, ClaimedModel: run.ClaimedModel,
		JuiceStatus: run.JuiceStatus, JuiceSummary: run.JuiceSummary,
		FingerprintCandidate: run.FingerprintCandidate, FingerprintSimilarity: run.FingerprintSimilarity,
		DetectorVersion: run.DetectorVersion, ErrorCode: run.ErrorCode, ErrorMessage: run.ErrorMessage,
		QueuedAt: &queued, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, RunID: run.ID, Source: source,
		Profile: run.Profile, Mode: run.Mode, TriggerReason: run.TriggerReason, PlannedRequests: run.PlannedRequests,
		ValidSamples: run.ValidSamples, EvidenceState: run.EvidenceState, FingerprintStatus: run.FingerprintStatus,
	}
}

func accountModelDetectionIsHistorical(run AccountModelDetectionRun) bool {
	return run.Profile == "" || run.Profile == AccountModelDetectionProfileUnknown || run.Mode == "" || run.Mode == AccountModelDetectionModeHistorical || run.EvidenceState == "" || run.EvidenceState == AccountModelDetectionEvidenceHistorical
}

func (s *AccountModelDetectionService) modelsForAccount(ctx context.Context, account *Account) (AccountModelDetectionModelsResponse, error) {
	registered := nativeAccountTextModels(account)
	settings := AccountModelDetectionSettings{AccountID: account.ID}
	if s.repo != nil {
		loaded, err := s.repo.LoadSettings(ctx, account.ID)
		if err == nil {
			settings = loaded
		}
	}
	connection := selectConnectionProbeModel(registered, settings.ConnectionProbeModel)
	connectionOptions := make([]AccountModelDetectionModelOption, 0, len(registered))
	for _, model := range registered {
		connectionOptions = append(connectionOptions, AccountModelDetectionModelOption{ID: model, Supported: true, Selected: model == connection})
	}
	catalogModels := []string(nil)
	detectorState := AccountModelDetectorStateReady
	if account.Type == AccountTypeAPIKey {
		detectorState, catalogModels = s.catalogSnapshot(ctx, false)
	}
	detectionOptions := buildDetectionModelOptions(registered, catalogModels, settings.ModelDetectionModel)
	for i := range detectionOptions {
		if detectorState == AccountModelDetectorStateUnconfigured {
			detectionOptions[i].Reason = "detector_unconfigured"
			detectionOptions[i].Supported = false
			detectionOptions[i].Selected = false
		} else if detectorState == AccountModelDetectorStateUnavailable {
			detectionOptions[i].Reason = "detector_unavailable"
			detectionOptions[i].Supported = false
			detectionOptions[i].Selected = false
		}
	}
	selectedDetection := settings.ModelDetectionModel
	for _, option := range detectionOptions {
		if option.Selected && option.Supported {
			selectedDetection = option.ID
			break
		}
	}
	if selectedDetection == "" {
		for _, option := range detectionOptions {
			if option.Supported {
				selectedDetection = option.ID
				break
			}
		}
	}
	if detectorState != AccountModelDetectorStateReady {
		selectedDetection = ""
	}
	return AccountModelDetectionModelsResponse{
		AccountID: account.ID, DetectorState: detectorState, ConnectionProbeModel: connection, ModelDetectionModel: selectedDetection,
		ConnectionModels: connectionOptions, DetectionModels: detectionOptions,
	}, nil
}

func (s *AccountModelDetectionService) catalogModels(ctx context.Context) []string {
	_, models := s.catalogSnapshot(ctx, false)
	return models
}

func (s *AccountModelDetectionService) catalogModelsFresh(ctx context.Context) []string {
	_, models := s.catalogSnapshot(ctx, true)
	return models
}

func (s *AccountModelDetectionService) catalogSnapshot(ctx context.Context, force bool) (AccountModelDetectorState, []string) {
	if s == nil || s.sidecar == nil {
		return AccountModelDetectorStateUnconfigured, nil
	}
	now := s.now()
	s.catalogMu.Lock()
	if !force && !s.catalogAt.IsZero() && now.Sub(s.catalogAt) < 5*time.Minute {
		models := append([]string(nil), s.catalog...)
		state := s.catalogState
		s.catalogMu.Unlock()
		if state == "" {
			state = AccountModelDetectorStateReady
		}
		return state, models
	}
	s.catalogMu.Unlock()
	catalog, err := s.sidecar.Catalog(ctx)
	if err != nil {
		state := AccountModelDetectorStateUnavailable
		if errors.Is(err, ErrAccountModelDetectorNotConfigured) {
			state = AccountModelDetectorStateUnconfigured
		}
		s.catalogMu.Lock()
		s.catalogAt = now
		s.catalog = nil
		s.catalogState = state
		s.catalogMu.Unlock()
		return state, nil
	}
	state := catalog.State
	if state == "" {
		state = AccountModelDetectorStateReady
	}
	s.catalogMu.Lock()
	s.catalogAt = now
	s.catalog = append([]string(nil), catalog.Models...)
	s.catalogState = state
	models := append([]string(nil), s.catalog...)
	s.catalogMu.Unlock()
	return state, models
}

func (s *AccountModelDetectionService) loadCatalogModels(ctx context.Context, force bool) []string {
	_, models := s.catalogSnapshot(ctx, force)
	return models
}

func (s *AccountModelDetectionService) SaveModels(ctx context.Context, actorID, accountID int64, connectionModel, detectionModel string) (AccountModelDetectionModelsResponse, error) {
	if s == nil || s.repo == nil || s.accounts == nil {
		return AccountModelDetectionModelsResponse{}, errors.New("account model detection is unavailable")
	}
	account, err := s.accounts.GetByID(ctx, accountID)
	if err != nil || account == nil {
		if err == nil {
			err = errors.New("account not found")
		}
		return AccountModelDetectionModelsResponse{}, err
	}
	models, err := s.modelsForAccount(ctx, account)
	if err != nil {
		return AccountModelDetectionModelsResponse{}, err
	}
	registered := nativeAccountTextModels(account)
	if !containsString(registered, strings.TrimSpace(connectionModel)) {
		return AccountModelDetectionModelsResponse{}, fmt.Errorf("connection probe model is not registered")
	}
	if strings.TrimSpace(detectionModel) != "" {
		valid := false
		for _, option := range models.DetectionModels {
			if option.ID == strings.TrimSpace(detectionModel) && option.Supported {
				valid = true
				break
			}
		}
		if !valid {
			return AccountModelDetectionModelsResponse{}, ErrAccountModelDetectionUnsupported
		}
	}
	settings := AccountModelDetectionSettings{AccountID: accountID, ConnectionProbeModel: strings.TrimSpace(connectionModel), ModelDetectionModel: strings.TrimSpace(detectionModel), UpdatedBy: actorID, UpdatedAt: s.now().UTC()}
	if err := s.repo.SaveSettings(ctx, settings); err != nil {
		return AccountModelDetectionModelsResponse{}, err
	}
	return s.modelsForAccount(ctx, account)
}

func (s *AccountModelDetectionService) EnqueueImmediate(ctx context.Context, accountID, actorID int64) (AccountModelDetectionRun, bool, error) {
	if s == nil || s.repo == nil || s.accounts == nil {
		return AccountModelDetectionRun{}, false, errors.New("account model detection is unavailable")
	}
	account, err := s.accounts.GetByID(ctx, accountID)
	if err != nil || account == nil {
		if err == nil {
			err = errors.New("account not found")
		}
		return AccountModelDetectionRun{}, false, err
	}
	if account.Type != AccountTypeAPIKey {
		return AccountModelDetectionRun{}, false, ErrAccountModelDetectionUnsupported
	}
	models, err := s.modelsForAccount(ctx, account)
	if err != nil {
		return AccountModelDetectionRun{}, false, err
	}
	if models.DetectorState != AccountModelDetectorStateReady {
		return AccountModelDetectionRun{}, false, ErrAccountModelDetectionUnavailable
	}
	if models.ModelDetectionModel == "" {
		return AccountModelDetectionRun{}, false, ErrAccountModelDetectionUnsupported
	}
	claimed := models.ModelDetectionModel
	run := newAccountModelDetectionRun(accountID, claimed, AccountModelDetectionProfileLow, AccountModelDetectionModeManual, AccountModelDetectionTriggerManual)
	run.TriggerKind = "manual"
	run.QueuedAt = s.now().UTC()
	run.CreatedAt = run.QueuedAt
	queued, reused, err := s.repo.Enqueue(ctx, run)
	if err != nil {
		return AccountModelDetectionRun{}, false, err
	}
	_ = actorID
	return queued, reused, nil
}

func shouldEscalateDetection(recent []AccountModelDetectionRun) (bool, string) {
	final := make([]AccountModelDetectionRun, 0, len(recent))
	for _, run := range recent {
		if run.FinishedAt != nil {
			final = append(final, run)
		}
	}
	if len(final) == 0 {
		return true, AccountModelDetectionTriggerFirstRun
	}
	if final[0].JuiceStatus == "mismatch" || final[0].FingerprintStatus == "mismatch" || final[0].FingerprintStatus == "strong_conflict" {
		return true, AccountModelDetectionTriggerModelConflict
	}
	if len(final) >= 2 && final[0].Status == AccountModelDetectionStatusAbnormal && final[1].Status == AccountModelDetectionStatusAbnormal {
		return true, AccountModelDetectionTriggerConsecutiveAbnormal
	}
	if len(final) >= 2 && final[0].Status == AccountModelDetectionStatusInsufficient && final[1].Status == AccountModelDetectionStatusInsufficient {
		return true, AccountModelDetectionTriggerInsufficient
	}
	if len(final) == 1 && final[0].Status != AccountModelDetectionStatusInsufficient {
		return true, AccountModelDetectionTriggerFirstRun
	}
	return false, ""
}

// isSuspiciousDetection returns true only for completed runs carrying an
// explicit model-conflict signal. Transport failures, detector failures,
// insufficient evidence, and an unqualified abnormal label are not enough to
// spend a higher-tier probe budget.
func isSuspiciousDetection(run AccountModelDetectionRun) bool {
	if run.FinishedAt == nil || run.Status == AccountModelDetectionStatusFailed {
		return false
	}
	if run.JuiceStatus == "mismatch" || run.FingerprintStatus == "mismatch" || run.FingerprintStatus == "strong_conflict" {
		return true
	}
	return strings.TrimSpace(run.FingerprintCandidate) != "" &&
		strings.TrimSpace(run.ClaimedModel) != "" &&
		strings.TrimSpace(run.FingerprintCandidate) != strings.TrimSpace(run.ClaimedModel)
}

// nextScheduledDetectionProfile derives the profile for the next natural
// six-hour slot from the most recent completed scheduled monitor run.
func nextScheduledDetectionProfile(recent []AccountModelDetectionRun) string {
	for _, run := range recent {
		if run.FinishedAt == nil {
			continue
		}
		if run.Mode != "" && run.Mode != AccountModelDetectionModeMonitor {
			continue
		}
		if run.TriggerKind != "" && run.TriggerKind != "scheduled" {
			continue
		}
		profile := run.Profile
		if profile == "" || profile == AccountModelDetectionProfileUnknown {
			profile = AccountModelDetectionProfileLow
		}
		if profile == AccountModelDetectionProfileHigh {
			if run.Status != AccountModelDetectionStatusFailed {
				return AccountModelDetectionProfileLow
			}
			return AccountModelDetectionProfileHigh
		}
		if isSuspiciousDetection(run) {
			if profile == AccountModelDetectionProfileLow {
				return AccountModelDetectionProfileMedium
			}
			if profile == AccountModelDetectionProfileMedium {
				return AccountModelDetectionProfileHigh
			}
		}
		if run.Status == AccountModelDetectionStatusNormal {
			return AccountModelDetectionProfileLow
		}
		return profile
	}
	return AccountModelDetectionProfileLow
}

func (s *AccountModelDetectionService) EnqueueEscalationHigh(ctx context.Context, accountID int64, reason string) (AccountModelDetectionRun, bool, error) {
	if s == nil || s.repo == nil || s.accounts == nil {
		return AccountModelDetectionRun{}, false, errors.New("account model detection is unavailable")
	}
	account, err := s.accounts.GetByID(ctx, accountID)
	if err != nil || account == nil {
		if err == nil {
			err = errors.New("account not found")
		}
		return AccountModelDetectionRun{}, false, err
	}
	if account.Type != AccountTypeAPIKey {
		return AccountModelDetectionRun{}, false, ErrAccountModelDetectionUnsupported
	}
	models, err := s.modelsForAccount(ctx, account)
	if err != nil {
		return AccountModelDetectionRun{}, false, err
	}
	if models.DetectorState != AccountModelDetectorStateReady {
		return AccountModelDetectionRun{}, false, ErrAccountModelDetectionUnavailable
	}
	if models.ModelDetectionModel == "" {
		return AccountModelDetectionRun{}, false, ErrAccountModelDetectionUnsupported
	}
	if reason == "" {
		reason = AccountModelDetectionTriggerFirstRun
	}
	s.escalationMu.Lock()
	last := s.escalationAt[accountID]
	now := s.now().UTC()
	if !last.IsZero() && now.Sub(last) < 30*time.Minute {
		s.escalationMu.Unlock()
		return AccountModelDetectionRun{}, true, nil
	}
	s.escalationAt[accountID] = now
	s.escalationMu.Unlock()
	run := newAccountModelDetectionRun(accountID, models.ModelDetectionModel, AccountModelDetectionProfileHigh, AccountModelDetectionModeEscalation, reason)
	run.TriggerKind = "scheduled"
	run.QueuedAt = now
	run.CreatedAt = now
	queued, reused, err := s.repo.Enqueue(ctx, run)
	if err != nil {
		return AccountModelDetectionRun{}, false, err
	}
	return queued, reused, nil
}

func (s *AccountModelDetectionService) RunDueSlots(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil || s.accounts == nil {
		return 0, nil
	}
	now := s.now().In(s.location)
	slot := dueDetectionSlot(now)
	if slot == "" {
		return 0, nil
	}
	accounts, err := s.accounts.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return 0, err
	}
	completed := 0
	for i := range accounts {
		account := &accounts[i]
		if account.Type != AccountTypeAPIKey || account.Status != StatusActive || !account.Schedulable || !account.ActiveProbeEnabled() || !accountActiveProbeEnabledByGroups(account) || s.usage == nil {
			continue
		}
		bucketStart, bucketEnd := currentActiveProbeBucket(now)
		used, usageErr := s.usage.HasAccountUsageInWindow(ctx, account.ID, bucketStart, bucketEnd)
		if usageErr != nil || used {
			continue
		}
		models, err := s.modelsForAccount(ctx, account)
		if err != nil || models.ModelDetectionModel == "" {
			continue
		}
		recent, recentErr := s.repo.ListRecent(ctx, account.ID, 20, "", "", "", "", "", "")
		if recentErr != nil {
			continue
		}
		profile := nextScheduledDetectionProfile(recent.Items)
		slotCopy := slot
		run := newAccountModelDetectionRun(account.ID, models.ModelDetectionModel, profile, AccountModelDetectionModeMonitor, AccountModelDetectionTriggerScheduled)
		if profile != AccountModelDetectionProfileLow {
			run.TriggerReason = AccountModelDetectionTriggerSuspicious
		}
		run.SlotKey = &slotCopy
		run.TriggerKind = "scheduled"
		run.QueuedAt = now.UTC()
		run.CreatedAt = run.QueuedAt
		_, reused, err := s.repo.Enqueue(ctx, run)
		if err != nil {
			return completed, err
		}
		if !reused {
			completed++
		}
	}
	return completed, nil
}

func (s *AccountModelDetectionService) RunQueued(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	runIDs, err := s.repo.ListQueued(ctx, 100)
	if err != nil {
		return 0, err
	}
	for _, runID := range runIDs {
		runID := runID
		go s.execute(ctx, runID)
	}
	return len(runIDs), nil
}

func (s *AccountModelDetectionService) Recent(ctx context.Context, accountID int64, limit int) ([]AccountModelDetectionRun, error) {
	page, err := s.RecentPage(ctx, accountID, limit, "", "", "", "", "", "")
	return page.Items, err
}

func (s *AccountModelDetectionService) RecentPage(ctx context.Context, accountID int64, limit int, cursor, status, profile, mode, juiceStatus, fingerprintStatus string) (AccountModelDetectionHistoryPage, error) {
	if s == nil || s.repo == nil {
		return AccountModelDetectionHistoryPage{}, errors.New("account model detection is unavailable")
	}
	page, err := s.repo.ListRecent(ctx, accountID, limit, cursor, status, profile, mode, juiceStatus, fingerprintStatus)
	if err != nil {
		return AccountModelDetectionHistoryPage{}, err
	}
	// Keep repository implementations and lightweight test doubles compatible:
	// if a repository returns an unbounded slice, apply the same stable page here.
	if page.NextCursor == "" && (len(page.Items) > limit || strings.TrimSpace(cursor) != "") {
		page.Items, page.NextCursor = paginateDetectionRuns(page.Items, limit, cursor, status, profile, mode)
	}
	return page, nil
}

type detectionHistoryCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func paginateDetectionRuns(items []AccountModelDetectionRun, limit int, cursor, status, profile, mode string) ([]AccountModelDetectionRun, string) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	filtered := make([]AccountModelDetectionRun, 0, len(items))
	var after detectionHistoryCursor
	if decoded, err := base64.RawURLEncoding.DecodeString(cursor); err == nil {
		_ = json.Unmarshal(decoded, &after)
	}
	for _, item := range items {
		if status != "" && item.Status != status || profile != "" && item.Profile != profile || mode != "" && item.Mode != mode {
			continue
		}
		if !after.CreatedAt.IsZero() && !item.CreatedAt.Before(after.CreatedAt) {
			if item.CreatedAt.Equal(after.CreatedAt) && item.ID < after.ID {
				// same timestamp rows after the cursor remain eligible
			} else {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	if len(filtered) <= limit {
		return filtered, ""
	}
	page := filtered[:limit]
	last := page[len(page)-1]
	cursorValue, _ := json.Marshal(detectionHistoryCursor{CreatedAt: last.CreatedAt.UTC(), ID: last.ID})
	return page, base64.RawURLEncoding.EncodeToString(cursorValue)
}

func (s *AccountModelDetectionService) execute(ctx context.Context, runID string) {
	select {
	case s.executionSlots <- struct{}{}:
		defer func() { <-s.executionSlots }()
	case <-ctx.Done():
		return
	}
	if s.repo == nil || s.accounts == nil || s.sidecar == nil {
		return
	}
	run, err := s.repo.Claim(ctx, runID)
	if err != nil || run == nil {
		return
	}
	account, err := s.accounts.GetByID(ctx, run.AccountID)
	if err != nil || account == nil || account.Type != AccountTypeAPIKey {
		_ = s.completeRun(ctx, *run, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "account_unavailable", "检测前账号已不可用")
		return
	}
	registered := nativeAccountTextModels(account)
	freshState, freshCatalog := s.catalogSnapshot(ctx, true)
	if freshState != AccountModelDetectorStateReady {
		errorCode := "detector_unavailable"
		if freshState == AccountModelDetectorStateUnconfigured {
			errorCode = "detector_unconfigured"
		}
		_ = s.completeRun(ctx, *run, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, errorCode, "检测服务不可用")
		return
	}
	if !containsString(registered, run.ModelID) || !containsString(freshCatalog, run.ModelID) {
		_ = s.completeRun(ctx, *run, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "model_unavailable", "检测模型已失效")
		return
	}
	apiKey, _ := account.Credentials["api_key"].(string)
	if strings.TrimSpace(apiKey) == "" {
		_ = s.completeRun(ctx, *run, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "missing_api_key", "检测前未发现 API Key")
		return
	}
	if run.TriggerKind == "scheduled" && run.Mode == AccountModelDetectionModeMonitor && s.usage != nil {
		bucketStart, bucketEnd := currentActiveProbeBucket(s.now())
		used, usageErr := s.usage.HasAccountUsageInWindow(ctx, account.ID, bucketStart, bucketEnd)
		if usageErr != nil || used {
			_ = s.completeRun(ctx, *run, AccountModelDetectionResponse{Status: AccountModelDetectionStatusInsufficient, Profile: run.Profile, PlannedRequests: run.PlannedRequests, EvidenceState: AccountModelDetectionEvidenceInsufficient}, "", "当前 5 分钟桶有真实请求，已跳过主动检测")
			return
		}
	}
	baseURL := account.GetBaseURL()
	response, err := s.sidecar.Detect(ctx, AccountModelDetectionRequest{RunID: run.ID, DeclaredModel: run.ClaimedModel, RequestModel: run.ModelID, APIKey: apiKey, BaseURL: baseURL, Profile: run.Profile, Mode: run.Mode, TriggerReason: run.TriggerReason})
	if err != nil {
		_ = s.completeRun(ctx, *run, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "detector_unavailable", "检测器不可用")
		return
	}
	if response.Profile == "" {
		response.Profile = run.Profile
	}
	if response.PlannedRequests == 0 {
		response.PlannedRequests = run.PlannedRequests
	}
	if response.EvidenceState == "" {
		if response.Status == AccountModelDetectionStatusNormal || response.Status == AccountModelDetectionStatusAbnormal {
			response.EvidenceState = AccountModelDetectionEvidenceComplete
		} else {
			response.EvidenceState = AccountModelDetectionEvidenceInsufficient
		}
	}
	if err := s.completeRun(ctx, *run, response, "", ""); err != nil {
		return
	}
}

func (s *AccountModelDetectionService) completeRun(ctx context.Context, run AccountModelDetectionRun, response AccountModelDetectionResponse, errorCode, errorMessage string) error {
	if response.Profile == "" {
		response.Profile = run.Profile
	}
	if response.PlannedRequests == 0 {
		response.PlannedRequests = run.PlannedRequests
	}
	if response.EvidenceState == "" {
		if errorCode != "" {
			response.EvidenceState = AccountModelDetectionEvidenceUnavailable
		} else if response.Status == AccountModelDetectionStatusNormal || response.Status == AccountModelDetectionStatusAbnormal {
			response.EvidenceState = AccountModelDetectionEvidenceComplete
		} else {
			response.EvidenceState = AccountModelDetectionEvidenceInsufficient
		}
	}
	return s.repo.Complete(ctx, run.ID, response, errorCode, errorMessage)
}

func nativeAccountTextModels(account *Account) []string {
	if account == nil {
		return nil
	}
	seen := map[string]bool{}
	models := make([]string, 0)
	for model := range account.GetModelMapping() {
		model = strings.TrimSpace(model)
		if model != "" && isAccountMonitorTextModel(model) && !seen[model] {
			seen[model] = true
			models = append(models, model)
		}
	}
	if len(models) == 0 {
		if fallback := strings.TrimSpace(monitorModelForAccount(account)); fallback != "" {
			models = append(models, fallback)
		}
	}
	sort.Slice(models, func(i, j int) bool { return naturalModelCompare(models[i], models[j]) > 0 })
	return models
}

func selectConnectionProbeModel(models []string, saved string) string {
	if containsString(models, strings.TrimSpace(saved)) {
		return strings.TrimSpace(saved)
	}
	for _, model := range models {
		if strings.EqualFold(model, "gpt-5.6-sol") {
			return model
		}
	}
	if len(models) > 0 {
		return models[0]
	}
	return ""
}

func buildDetectionModelOptions(native, catalog []string, saved string) []AccountModelDetectionModelOption {
	supported := make(map[string]bool, len(catalog))
	for _, model := range catalog {
		supported[strings.TrimSpace(model)] = true
	}
	effective := ""
	saved = strings.TrimSpace(saved)
	if saved != "" && supported[saved] && containsString(native, saved) {
		effective = saved
	}
	if effective == "" {
		for _, model := range native {
			model = strings.TrimSpace(model)
			if supported[model] {
				effective = model
				break
			}
		}
	}
	options := make([]AccountModelDetectionModelOption, 0, len(native))
	for _, model := range native {
		model = strings.TrimSpace(model)
		ok := supported[model]
		options = append(options, AccountModelDetectionModelOption{ID: model, Supported: ok, Selected: model == effective, Reason: func() string {
			if !ok {
				return "detector_unsupported"
			}
			return ""
		}()})
	}
	return options
}

func dueDetectionSlot(now time.Time) string {
	minutes := now.Minute()
	if minutes >= 30 {
		return ""
	}
	slots := []int{0, 6, 12, 18}
	best := -1
	for _, slotHour := range slots {
		if slotHour < now.Hour() || (slotHour == now.Hour() && minutes >= 0) {
			best = slotHour
		}
	}
	if best < 0 {
		return ""
	}
	slotTime := time.Date(now.Year(), now.Month(), now.Day(), best, 0, 0, 0, now.Location())
	if now.Sub(slotTime) > 30*time.Minute {
		return ""
	}
	return slotTime.Format("2006-01-02T15:04")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
