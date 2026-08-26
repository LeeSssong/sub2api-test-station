package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSelectConnectionProbeModelPrefersSolThenFirstText(t *testing.T) {
	models := []string{"claude-3-5-sonnet", "gpt-4o", "gpt-5.6-sol"}
	if got := selectConnectionProbeModel(models, ""); got != "gpt-5.6-sol" {
		t.Fatalf("selectConnectionProbeModel() = %q, want gpt-5.6-sol", got)
	}
	if got := selectConnectionProbeModel([]string{"claude-3-5-sonnet", "gpt-4o"}, ""); got != "claude-3-5-sonnet" {
		t.Fatalf("selectConnectionProbeModel() = %q, want first deterministic text model", got)
	}
	if got := selectConnectionProbeModel([]string{"gpt-4o", "gpt-5.6-sol"}, "gpt-4o"); got != "gpt-4o" {
		t.Fatalf("selectConnectionProbeModel() did not preserve saved model: %q", got)
	}
}

func TestDetectionModelOptionsUseNativeCatalogIntersection(t *testing.T) {
	options := buildDetectionModelOptions([]string{"relay-alias", "gpt-5.6-sol"}, []string{"gpt-5.6-sol"}, "relay-alias")
	if len(options) != 2 || options[0].Selected || options[0].Supported {
		t.Fatalf("options = %#v, want native unsupported option visible but not selected", options)
	}
	if options[1].ID != "gpt-5.6-sol" || !options[1].Supported || !options[1].Selected {
		t.Fatalf("options = %#v, want supported fallback selected", options)
	}
}

func TestModelsFallBackWhenSavedDetectionModelLosesCatalogSupport(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"relay-alias": "relay-alias", "gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	repo := &detectionRepoStub{settings: AccountModelDetectionSettings{AccountID: 7, ModelDetectionModel: "relay-alias"}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})

	models, err := svc.Models(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if models.ModelDetectionModel != "gpt-5.6-sol" {
		t.Fatalf("effective detection model = %q, want supported fallback", models.ModelDetectionModel)
	}
}

func TestModelDetectorAvailabilityStates(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	tests := []struct {
		name          string
		sidecar       *detectionSidecarStub
		wantState     AccountModelDetectorState
		wantStatus    string
		wantReason    string
		wantModel     string
		wantSupported bool
	}{
		{name: "unconfigured", sidecar: &detectionSidecarStub{catalogErr: ErrAccountModelDetectorNotConfigured}, wantState: AccountModelDetectorStateUnconfigured, wantStatus: AccountModelDetectionStatusServiceUnconfigured, wantReason: "detector_unconfigured"},
		{name: "unavailable", sidecar: &detectionSidecarStub{catalogErr: errors.New("offline")}, wantState: AccountModelDetectorStateUnavailable, wantStatus: AccountModelDetectionStatusServiceUnavailable, wantReason: "detector_unavailable"},
		{name: "ready", sidecar: &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}}, wantState: AccountModelDetectorStateReady, wantStatus: AccountModelDetectionStatusUntested, wantModel: "gpt-5.6-sol", wantSupported: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAccountModelDetectionService(&detectionRepoStub{}, &detectionAccountReaderStub{account: account}, tt.sidecar)
			models, err := svc.Models(context.Background(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if models.DetectorState != tt.wantState || models.ModelDetectionModel != tt.wantModel {
				t.Fatalf("models = %#v", models)
			}
			if len(models.DetectionModels) != 1 || models.DetectionModels[0].Supported != tt.wantSupported || models.DetectionModels[0].Reason != tt.wantReason {
				t.Fatalf("options = %#v", models.DetectionModels)
			}
			projection, err := svc.ProjectionForAccount(context.Background(), account)
			if err != nil {
				t.Fatal(err)
			}
			if projection.DetectorState != tt.wantState || projection.Status != tt.wantStatus {
				t.Fatalf("projection = %#v", projection)
			}
		})
	}
}

func TestEnqueueRejectsOfflineDetector(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	svc := NewAccountModelDetectionService(&detectionRepoStub{}, &detectionAccountReaderStub{account: account}, &detectionSidecarStub{catalogErr: ErrAccountModelDetectorNotConfigured})
	if _, _, err := svc.EnqueueImmediate(context.Background(), account.ID, 1); !errors.Is(err, ErrAccountModelDetectionUnavailable) {
		t.Fatalf("EnqueueImmediate error = %v, want detector unavailable", err)
	}
}

func TestEnqueueImmediateReusesQueuedRunAndRejectsOAuth(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Credentials: map[string]any{"api_key": "secret", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	repo := &detectionRepoStub{}
	reader := &detectionAccountReaderStub{account: account}
	svc := NewAccountModelDetectionService(repo, reader, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	first, reused, err := svc.EnqueueImmediate(context.Background(), 7, 9)
	if err != nil || reused || first.Status != AccountModelDetectionStatusQueued {
		t.Fatalf("first enqueue = %#v reused=%v err=%v", first, reused, err)
	}
	second, reused, err := svc.EnqueueImmediate(context.Background(), 7, 9)
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("second enqueue = %#v reused=%v err=%v", second, reused, err)
	}
	account.Type = AccountTypeOAuth
	if _, _, err := svc.EnqueueImmediate(context.Background(), 7, 9); !errors.Is(err, ErrAccountModelDetectionUnsupported) {
		t.Fatalf("OAuth enqueue err = %v, want unsupported", err)
	}
}

func TestEnqueueImmediatePersistsUntilWorkerRunsQueuedDetection(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "secret", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	runRepo := &workerQueueDetectionRepoStub{}
	sidecar := &countingDetectionSidecar{catalog: []string{"gpt-5.6-sol"}}
	svc := NewAccountModelDetectionService(runRepo, &detectionAccountReaderStub{account: account}, sidecar)

	run, reused, err := svc.EnqueueImmediate(context.Background(), account.ID, 9)
	if err != nil || reused {
		t.Fatalf("enqueue run=%#v reused=%v err=%v", run, reused, err)
	}
	time.Sleep(20 * time.Millisecond)
	if calls := sidecar.detectCallCount(); calls != 0 {
		t.Fatalf("API enqueue executed sidecar %d times", calls)
	}
	started, err := svc.RunQueued(context.Background())
	if err != nil || started != 1 {
		t.Fatalf("RunQueued started=%d err=%v", started, err)
	}
	deadline := time.After(250 * time.Millisecond)
	for sidecar.detectCallCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("worker queue did not execute persisted run")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestModelsKeepOAuthConnectionChoiceButDisableDetection(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	svc := NewAccountModelDetectionService(&detectionRepoStub{}, &detectionAccountReaderStub{account: account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})

	models, err := svc.Models(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if models.ConnectionProbeModel != "gpt-5.6-sol" {
		t.Fatalf("connection probe model = %q", models.ConnectionProbeModel)
	}
	if models.ModelDetectionModel != "" {
		t.Fatalf("OAuth detection model = %q, want empty", models.ModelDetectionModel)
	}
	if len(models.DetectionModels) != 1 || models.DetectionModels[0].Supported {
		t.Fatalf("OAuth detection options = %#v, want visible but unsupported", models.DetectionModels)
	}
}

func TestProjectionKeepsOAuthUnsupportedEvenWithHistoricalResult(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	repo := &detectionRepoStub{recent: []AccountModelDetectionRun{{ID: "old-run", AccountID: 7, Status: AccountModelDetectionStatusNormal, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", QueuedAt: time.Now().UTC()}}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})

	projection, err := svc.ProjectionForAccount(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Status != AccountModelDetectionStatusUnsupported {
		t.Fatalf("OAuth projection status = %q, want unsupported", projection.Status)
	}
}

func TestProjectionUsesLatestCompletedFinalDetectionWhenCurrentEvidenceIsInsufficient(t *testing.T) {
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	currentFinished := now.Add(-time.Minute)
	previousFinished := now.Add(-time.Hour)
	repo := &detectionRepoStub{recent: []AccountModelDetectionRun{
		{ID: "current", AccountID: 7, Status: AccountModelDetectionStatusInsufficient, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", FinishedAt: &currentFinished, QueuedAt: currentFinished.Add(-time.Second)},
		{ID: "previous", AccountID: 7, Status: AccountModelDetectionStatusNormal, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", JuiceStatus: "ok", DetectorVersion: "detector-1", FinishedAt: &previousFinished, QueuedAt: previousFinished.Add(-time.Second)},
	}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})

	projection, err := svc.ProjectionForAccount(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Status != AccountModelDetectionStatusInsufficient {
		t.Fatalf("current status = %q, want insufficient", projection.Status)
	}
	if projection.Recent == nil || projection.Recent.RunID != "previous" || projection.Recent.Source != "historical_final" {
		t.Fatalf("effective recent = %#v, want previous historical final", projection.Recent)
	}
	if projection.Current == nil || projection.Current.RunID != "current" || projection.Current.Status != AccountModelDetectionStatusInsufficient {
		t.Fatalf("current projection = %#v", projection.Current)
	}
}

func TestCatalogFailureIsCachedForFiveMinutes(t *testing.T) {
	sidecar := &detectionSidecarStub{catalogErr: errors.New("offline")}
	svc := NewAccountModelDetectionService(&detectionRepoStub{}, &detectionAccountReaderStub{}, sidecar)
	svc.now = func() time.Time { return time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC) }

	if got := svc.catalogModels(context.Background()); len(got) != 0 {
		t.Fatalf("first catalog = %#v", got)
	}
	if got := svc.catalogModels(context.Background()); len(got) != 0 {
		t.Fatalf("second catalog = %#v", got)
	}
	if calls := sidecar.catalogCallCount(); calls != 1 {
		t.Fatalf("catalog calls = %d, want 1 cached failure", calls)
	}
}

func TestRunDueSlotsIncludesDisabledAndUnschedulableAPIKeyAccounts(t *testing.T) {
	accounts := []Account{
		{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusDisabled, Schedulable: false, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}},
		{ID: 8, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}},
		{ID: 9, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}},
	}
	repo := &detectionRepoStub{}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{accounts: accounts}, &detectionSidecarStub{catalog: []string{"gpt-5.6-sol"}})
	svc.now = func() time.Time { return time.Date(2026, 8, 17, 10, 5, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)) }

	queued, err := svc.RunDueSlots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if queued != 2 {
		t.Fatalf("queued = %d, want both API Key accounts regardless of status/schedulable", queued)
	}
}

func TestExecuteRechecksAccountTypeBeforeSendingCredentials(t *testing.T) {
	run := AccountModelDetectionRun{ID: "run-1", AccountID: 7, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Status: AccountModelDetectionStatusQueued}
	repo := &executionDetectionRepoStub{runs: map[string]AccountModelDetectionRun{run.ID: run}}
	sidecar := &blockingDetectionSidecar{catalog: []string{"gpt-5.6-sol"}, started: make(chan string, 1), release: make(chan struct{})}
	account := &Account{ID: 7, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "must-not-leak", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, sidecar)

	svc.execute(context.Background(), run.ID)
	select {
	case <-sidecar.started:
		t.Fatal("OAuth account credentials reached sidecar")
	default:
	}
	completed := repo.completion(run.ID)
	if completed.errorCode != "account_unavailable" || completed.response.Status != AccountModelDetectionStatusFailed {
		t.Fatalf("completion = %#v", completed)
	}
}

func TestExecuteRefreshesCatalogBeforeSendingCredentials(t *testing.T) {
	run := AccountModelDetectionRun{ID: "run-1", AccountID: 7, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Status: AccountModelDetectionStatusQueued}
	repo := &executionDetectionRepoStub{runs: map[string]AccountModelDetectionRun{run.ID: run}}
	sidecar := &countingDetectionSidecar{}
	account := &Account{ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "must-not-leak", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{account: account}, sidecar)
	svc.catalogAt = time.Now()
	svc.catalog = []string{"gpt-5.6-sol"}

	svc.execute(context.Background(), run.ID)
	if sidecar.detectCallCount() != 0 {
		t.Fatal("stale cached model reached sidecar detection")
	}
	if sidecar.catalogCallCount() != 1 {
		t.Fatalf("fresh catalog calls = %d, want 1", sidecar.catalogCallCount())
	}
	completed := repo.completion(run.ID)
	if completed.errorCode != "model_unavailable" {
		t.Fatalf("completion = %#v", completed)
	}
}

func TestExecuteAllowsFourAccountsToRunWithoutGlobalSerialization(t *testing.T) {
	runs := map[string]AccountModelDetectionRun{
		"run-1": {ID: "run-1", AccountID: 7, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Status: AccountModelDetectionStatusQueued},
		"run-2": {ID: "run-2", AccountID: 8, ModelID: "gpt-5.6-sol", ClaimedModel: "gpt-5.6-sol", Status: AccountModelDetectionStatusQueued},
	}
	repo := &executionDetectionRepoStub{runs: runs}
	sidecar := &blockingDetectionSidecar{catalog: []string{"gpt-5.6-sol"}, started: make(chan string, 2), release: make(chan struct{})}
	accounts := map[int64]*Account{
		7: {ID: 7, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "key-7", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}},
		8: {ID: 8, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"api_key": "key-8", "model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}}, Extra: map[string]any{}},
	}
	svc := NewAccountModelDetectionService(repo, &detectionAccountReaderStub{byID: accounts}, sidecar)

	done := make(chan struct{}, 2)
	for _, runID := range []string{"run-1", "run-2"} {
		go func(id string) {
			svc.execute(context.Background(), id)
			done <- struct{}{}
		}(runID)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-sidecar.started:
		case <-time.After(250 * time.Millisecond):
			close(sidecar.release)
			t.Fatal("detections were serialized across accounts")
		}
	}
	close(sidecar.release)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(250 * time.Millisecond):
			t.Fatal("detection did not finish after release")
		}
	}
}

func TestDueSlotOnlyFiresWithinThirtyMinutes(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	for _, tc := range []struct {
		name string
		at   string
		want string
	}{
		{name: "exact", at: "2026-08-17T10:00:00+08:00", want: "2026-08-17T10:00"},
		{name: "late", at: "2026-08-17T10:29:59+08:00", want: "2026-08-17T10:00"},
		{name: "too late", at: "2026-08-17T10:30:01+08:00", want: ""},
		{name: "between slots", at: "2026-08-17T10:45:00+08:00", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, _ := time.Parse(time.RFC3339, tc.at)
			if got := dueDetectionSlot(at.In(loc)); got != tc.want {
				t.Fatalf("dueDetectionSlot(%s) = %q, want %q", tc.at, got, tc.want)
			}
		})
	}
}

type detectionAccountReaderStub struct {
	account  *Account
	accounts []Account
	byID     map[int64]*Account
}

func (s *detectionAccountReaderStub) GetByID(_ context.Context, id int64) (*Account, error) {
	if s.byID != nil {
		return s.byID[id], nil
	}
	return s.account, nil
}
func (s *detectionAccountReaderStub) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	if s.accounts != nil {
		return append([]Account(nil), s.accounts...), nil
	}
	if s.account == nil {
		return nil, nil
	}
	return []Account{*s.account}, nil
}

type detectionRepoStub struct {
	mu       sync.Mutex
	runs     []*AccountModelDetectionRun
	recent   []AccountModelDetectionRun
	settings AccountModelDetectionSettings
}

func (s *detectionRepoStub) LoadSettings(context.Context, int64) (AccountModelDetectionSettings, error) {
	return s.settings, nil
}
func (s *detectionRepoStub) SaveSettings(context.Context, AccountModelDetectionSettings) error {
	return nil
}
func (s *detectionRepoStub) Enqueue(_ context.Context, run AccountModelDetectionRun) (AccountModelDetectionRun, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.AccountID == run.AccountID && ((run.SlotKey != nil && existing.SlotKey != nil && *existing.SlotKey == *run.SlotKey) || (run.SlotKey == nil && (existing.Status == AccountModelDetectionStatusQueued || existing.Status == AccountModelDetectionStatusRunning))) {
			return *existing, true, nil
		}
	}
	s.runs = append(s.runs, &run)
	return run, false, nil
}
func (s *detectionRepoStub) Claim(context.Context, string) (*AccountModelDetectionRun, error) {
	return nil, errors.New("not used")
}

func (s *detectionRepoStub) ListQueued(context.Context, int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.runs))
	for _, run := range s.runs {
		if run.Status == AccountModelDetectionStatusQueued {
			ids = append(ids, run.ID)
		}
	}
	return ids, nil
}
func (s *detectionRepoStub) Complete(context.Context, string, AccountModelDetectionResponse, string, string) error {
	return nil
}
func (s *detectionRepoStub) ListRecent(context.Context, int64, int, string, string, string, string, string, string) (AccountModelDetectionHistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]AccountModelDetectionRun(nil), s.recent...)
	return AccountModelDetectionHistoryPage{Items: items}, nil
}

type detectionSidecarStub struct {
	mu         sync.Mutex
	catalog    []string
	catalogErr error
	calls      int
}

func (s *detectionSidecarStub) Catalog(context.Context) (AccountModelDetectionSidecarCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.catalogErr != nil {
		return AccountModelDetectionSidecarCatalog{}, s.catalogErr
	}
	return AccountModelDetectionSidecarCatalog{Version: "test", Models: s.catalog}, nil
}
func (s *detectionSidecarStub) Detect(context.Context, AccountModelDetectionRequest) (AccountModelDetectionResponse, error) {
	return AccountModelDetectionResponse{Status: AccountModelDetectionStatusNormal}, nil
}

func (s *detectionSidecarStub) catalogCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type executionDetectionCompletion struct {
	response     AccountModelDetectionResponse
	errorCode    string
	errorMessage string
}

type executionDetectionRepoStub struct {
	mu          sync.Mutex
	runs        map[string]AccountModelDetectionRun
	completions map[string]executionDetectionCompletion
}

func (s *executionDetectionRepoStub) LoadSettings(context.Context, int64) (AccountModelDetectionSettings, error) {
	return AccountModelDetectionSettings{}, nil
}
func (s *executionDetectionRepoStub) SaveSettings(context.Context, AccountModelDetectionSettings) error {
	return nil
}
func (s *executionDetectionRepoStub) Enqueue(context.Context, AccountModelDetectionRun) (AccountModelDetectionRun, bool, error) {
	return AccountModelDetectionRun{}, false, errors.New("not used")
}
func (s *executionDetectionRepoStub) Claim(_ context.Context, runID string) (*AccountModelDetectionRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return nil, nil
	}
	run.Status = AccountModelDetectionStatusRunning
	s.runs[runID] = run
	return &run, nil
}
func (s *executionDetectionRepoStub) ListQueued(context.Context, int) ([]string, error) {
	return nil, nil
}
func (s *executionDetectionRepoStub) Complete(_ context.Context, runID string, response AccountModelDetectionResponse, errorCode, errorMessage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completions == nil {
		s.completions = make(map[string]executionDetectionCompletion)
	}
	s.completions[runID] = executionDetectionCompletion{response: response, errorCode: errorCode, errorMessage: errorMessage}
	return nil
}
func (s *executionDetectionRepoStub) ListRecent(context.Context, int64, int, string, string, string, string, string, string) (AccountModelDetectionHistoryPage, error) {
	return AccountModelDetectionHistoryPage{}, nil
}
func (s *executionDetectionRepoStub) completion(runID string) executionDetectionCompletion {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completions[runID]
}

type blockingDetectionSidecar struct {
	catalog []string
	started chan string
	release chan struct{}
}

func (s *blockingDetectionSidecar) Catalog(context.Context) (AccountModelDetectionSidecarCatalog, error) {
	return AccountModelDetectionSidecarCatalog{Models: s.catalog}, nil
}
func (s *blockingDetectionSidecar) Detect(_ context.Context, request AccountModelDetectionRequest) (AccountModelDetectionResponse, error) {
	s.started <- request.RunID
	<-s.release
	return AccountModelDetectionResponse{Status: AccountModelDetectionStatusNormal}, nil
}

type countingDetectionSidecar struct {
	mu           sync.Mutex
	catalog      []string
	catalogCalls int
	detectCalls  int
}

func (s *countingDetectionSidecar) Catalog(context.Context) (AccountModelDetectionSidecarCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.catalogCalls++
	return AccountModelDetectionSidecarCatalog{Models: append([]string(nil), s.catalog...)}, nil
}

type workerQueueDetectionRepoStub struct {
	mu   sync.Mutex
	runs map[string]AccountModelDetectionRun
}

func (s *workerQueueDetectionRepoStub) LoadSettings(context.Context, int64) (AccountModelDetectionSettings, error) {
	return AccountModelDetectionSettings{}, nil
}
func (s *workerQueueDetectionRepoStub) SaveSettings(context.Context, AccountModelDetectionSettings) error {
	return nil
}
func (s *workerQueueDetectionRepoStub) Enqueue(_ context.Context, run AccountModelDetectionRun) (AccountModelDetectionRun, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = make(map[string]AccountModelDetectionRun)
	}
	for _, existing := range s.runs {
		if existing.AccountID == run.AccountID && (existing.Status == AccountModelDetectionStatusQueued || existing.Status == AccountModelDetectionStatusRunning) {
			return existing, true, nil
		}
	}
	s.runs[run.ID] = run
	return run, false, nil
}
func (s *workerQueueDetectionRepoStub) ListQueued(context.Context, int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.runs))
	for id, run := range s.runs {
		if run.Status == AccountModelDetectionStatusQueued {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
func (s *workerQueueDetectionRepoStub) Claim(_ context.Context, runID string) (*AccountModelDetectionRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run.Status != AccountModelDetectionStatusQueued {
		return nil, nil
	}
	run.Status = AccountModelDetectionStatusRunning
	s.runs[runID] = run
	return &run, nil
}
func (s *workerQueueDetectionRepoStub) Complete(_ context.Context, runID string, response AccountModelDetectionResponse, errorCode, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[runID]
	run.Status = response.Status
	if errorCode != "" {
		run.Status = AccountModelDetectionStatusFailed
	}
	s.runs[runID] = run
	return nil
}
func (s *workerQueueDetectionRepoStub) ListRecent(context.Context, int64, int, string, string, string, string, string, string) (AccountModelDetectionHistoryPage, error) {
	return AccountModelDetectionHistoryPage{}, nil
}
func (s *countingDetectionSidecar) Detect(context.Context, AccountModelDetectionRequest) (AccountModelDetectionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detectCalls++
	return AccountModelDetectionResponse{Status: AccountModelDetectionStatusNormal}, nil
}
func (s *countingDetectionSidecar) catalogCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.catalogCalls
}
func (s *countingDetectionSidecar) detectCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detectCalls
}
