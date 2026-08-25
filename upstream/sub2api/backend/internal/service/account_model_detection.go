package service

import (
	"context"
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
	ListRecent(context.Context, int64, int) ([]AccountModelDetectionRun, error)
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
	location       *time.Location
	now            func() time.Time
	executionSlots chan struct{}
	catalogMu      sync.Mutex
	catalogAt      time.Time
	catalog        []string
	catalogState   AccountModelDetectorState
}

func NewAccountModelDetectionService(repo AccountModelDetectionRepository, accounts AccountModelDetectionAccountReader, sidecar AccountModelDetectionSidecar) *AccountModelDetectionService {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return &AccountModelDetectionService{repo: repo, accounts: accounts, sidecar: sidecar, location: location, now: time.Now, executionSlots: make(chan struct{}, 4)}
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
	queued := run.QueuedAt
	return &AccountModelDetectionSummary{
		Status: run.Status, ModelID: run.ModelID, ClaimedModel: run.ClaimedModel,
		JuiceStatus: run.JuiceStatus, JuiceSummary: run.JuiceSummary,
		FingerprintCandidate: run.FingerprintCandidate, FingerprintSimilarity: run.FingerprintSimilarity,
		DetectorVersion: run.DetectorVersion, ErrorCode: run.ErrorCode, ErrorMessage: run.ErrorMessage,
		QueuedAt: &queued, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, RunID: run.ID, Source: source,
	}
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
	run := AccountModelDetectionRun{ID: uuid.NewString(), AccountID: accountID, TriggerKind: "manual", ModelID: claimed, ClaimedModel: claimed, Status: AccountModelDetectionStatusQueued, QueuedAt: s.now().UTC(), CreatedAt: s.now().UTC()}
	queued, reused, err := s.repo.Enqueue(ctx, run)
	if err != nil {
		return AccountModelDetectionRun{}, false, err
	}
	_ = actorID
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
		if account.Type != AccountTypeAPIKey {
			continue
		}
		models, err := s.modelsForAccount(ctx, account)
		if err != nil || models.ModelDetectionModel == "" {
			continue
		}
		slotCopy := slot
		run := AccountModelDetectionRun{ID: uuid.NewString(), AccountID: account.ID, SlotKey: &slotCopy, TriggerKind: "scheduled", ModelID: models.ModelDetectionModel, ClaimedModel: models.ModelDetectionModel, Status: AccountModelDetectionStatusQueued, QueuedAt: now.UTC(), CreatedAt: now.UTC()}
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
	if s == nil || s.repo == nil {
		return nil, errors.New("account model detection is unavailable")
	}
	return s.repo.ListRecent(ctx, accountID, limit)
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
		_ = s.repo.Complete(ctx, run.ID, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "account_unavailable", "检测前账号已不可用")
		return
	}
	registered := nativeAccountTextModels(account)
	freshState, freshCatalog := s.catalogSnapshot(ctx, true)
	if freshState != AccountModelDetectorStateReady {
		errorCode := "detector_unavailable"
		if freshState == AccountModelDetectorStateUnconfigured {
			errorCode = "detector_unconfigured"
		}
		_ = s.repo.Complete(ctx, run.ID, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, errorCode, "检测服务不可用")
		return
	}
	if !containsString(registered, run.ModelID) || !containsString(freshCatalog, run.ModelID) {
		_ = s.repo.Complete(ctx, run.ID, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "model_unavailable", "检测模型已失效")
		return
	}
	apiKey, _ := account.Credentials["api_key"].(string)
	if strings.TrimSpace(apiKey) == "" {
		_ = s.repo.Complete(ctx, run.ID, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "missing_api_key", "检测前未发现 API Key")
		return
	}
	baseURL := account.GetBaseURL()
	response, err := s.sidecar.Detect(ctx, AccountModelDetectionRequest{RunID: run.ID, DeclaredModel: run.ClaimedModel, RequestModel: run.ModelID, APIKey: apiKey, BaseURL: baseURL})
	if err != nil {
		_ = s.repo.Complete(ctx, run.ID, AccountModelDetectionResponse{Status: AccountModelDetectionStatusFailed}, "detector_unavailable", "检测器不可用")
		return
	}
	if err := s.repo.Complete(ctx, run.ID, response, "", ""); err != nil {
		return
	}
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
	slots := []int{0, 10, 12, 15, 18, 21}
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
