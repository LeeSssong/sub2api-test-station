package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type OpenAISharedHealthConfig = config.GatewayOpenAISharedHealthConfig

func DefaultOpenAISharedHealthConfig() OpenAISharedHealthConfig {
	return config.DefaultGatewayOpenAISharedHealthConfig()
}

type OpenAISharedHealthKey struct {
	AccountID      int64
	CanonicalModel string
}

func NewOpenAISharedHealthKey(accountID int64, model string) (OpenAISharedHealthKey, error) {
	canonicalModel := strings.ToLower(strings.TrimSpace(model))
	if accountID <= 0 {
		return OpenAISharedHealthKey{}, fmt.Errorf("account id must be positive")
	}
	if canonicalModel == "" {
		return OpenAISharedHealthKey{}, fmt.Errorf("canonical model must not be empty")
	}
	return OpenAISharedHealthKey{AccountID: accountID, CanonicalModel: canonicalModel}, nil
}

func (k OpenAISharedHealthKey) HashedSuffix() string {
	sum := sha256.Sum256([]byte(k.CanonicalModel))
	return hex.EncodeToString(sum[:16])
}

type OpenAIFailureDomainType string

const (
	OpenAIFailureDomainProviderChannel OpenAIFailureDomainType = "provider_channel"
	OpenAIFailureDomainQuotaPool       OpenAIFailureDomainType = "quota_pool"
	OpenAIFailureDomainUnknown         OpenAIFailureDomainType = "unknown"
)

type OpenAIFailureDomain struct {
	Type OpenAIFailureDomainType
	ID   string
}

func DeriveOpenAIFailureDomains(account *Account, channelID int64) []OpenAIFailureDomain {
	if account == nil {
		return []OpenAIFailureDomain{{Type: OpenAIFailureDomainUnknown, ID: "unknown"}}
	}
	platform := strings.ToLower(strings.TrimSpace(account.Platform))
	if platform == "" {
		platform = "unknown"
	}
	domains := make([]OpenAIFailureDomain, 0, 2)
	if channelID > 0 {
		domains = append(domains, OpenAIFailureDomain{
			Type: OpenAIFailureDomainProviderChannel,
			ID:   platform + ":channel:" + strconv.FormatInt(channelID, 10),
		})
	}
	if quotaPoolID := explicitOpenAIQuotaPoolID(account.Extra); quotaPoolID != "" {
		domains = append(domains, OpenAIFailureDomain{
			Type: OpenAIFailureDomainQuotaPool,
			ID:   platform + ":quota_pool:" + quotaPoolID,
		})
	}
	if len(domains) == 0 {
		return []OpenAIFailureDomain{{Type: OpenAIFailureDomainUnknown, ID: "unknown"}}
	}
	return domains
}

func explicitOpenAIQuotaPoolID(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	value, ok := extra["quota_pool_id"]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) {
			return ""
		}
		return strconv.FormatFloat(typed, 'f', 0, 64)
	default:
		return ""
	}
}

type OpenAISharedHealthState string

const (
	OpenAISharedHealthStateUnknown    OpenAISharedHealthState = "unknown"
	OpenAISharedHealthStateHealthy    OpenAISharedHealthState = "healthy"
	OpenAISharedHealthStateSoftFailed OpenAISharedHealthState = "soft_failed"
	OpenAISharedHealthStateCooldown   OpenAISharedHealthState = "cooldown"
	OpenAISharedHealthStateHalfOpen   OpenAISharedHealthState = "half_open"
)

var (
	ErrOpenAISharedHealthUnknownSchema = errors.New("unknown OpenAI shared health schema")
	ErrOpenAISharedHealthLeaseLost     = errors.New("OpenAI shared health half-open lease lost")
)

type OpenAISharedHealthFreshness string

const (
	OpenAISharedHealthFresh OpenAISharedHealthFreshness = "fresh"
	OpenAISharedHealthStale OpenAISharedHealthFreshness = "stale"
)

type OpenAISharedHealthSnapshot struct {
	SchemaVersion  int
	Revision       int64
	Key            OpenAISharedHealthKey
	State          OpenAISharedHealthState
	FailureStreak  int
	CooldownUntil  time.Time
	LastStatusCode int
	LastErrorType  string
	EWMAErrorRate  float64
	EWMATTFT       time.Duration
	ObservedAt     time.Time
}

func (s OpenAISharedHealthSnapshot) Freshness(now time.Time, staleAfter time.Duration) OpenAISharedHealthFreshness {
	if staleAfter <= 0 || s.ObservedAt.IsZero() || now.Sub(s.ObservedAt) > staleAfter {
		return OpenAISharedHealthStale
	}
	return OpenAISharedHealthFresh
}

type OpenAISharedHealthEvent struct {
	ID            string
	Key           OpenAISharedHealthKey
	Domains       []OpenAIFailureDomain
	Success       bool
	StatusCode    int
	ErrorType     string
	TTFT          time.Duration
	ObservedAt    time.Time
	CooldownUntil time.Time
}

type OpenAISharedHalfOpenLease struct {
	Key          OpenAISharedHealthKey
	Owner        string
	FencingToken int64
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

type OpenAISharedHealthStore interface {
	GetAccountModel(ctx context.Context, key OpenAISharedHealthKey) (OpenAISharedHealthSnapshot, error)
	RecordAttempt(ctx context.Context, event OpenAISharedHealthEvent) (OpenAISharedHealthSnapshot, error)
	AcquireHalfOpen(ctx context.Context, key OpenAISharedHealthKey, owner string, ttl time.Duration) (OpenAISharedHalfOpenLease, bool, error)
	CompleteHalfOpen(ctx context.Context, lease OpenAISharedHalfOpenLease, success bool, observedAt time.Time) error
}

type OpenAISharedHealthMode string

const (
	OpenAISharedHealthModeDisabled  OpenAISharedHealthMode = "disabled"
	OpenAISharedHealthModeLocalOnly OpenAISharedHealthMode = "local_only"
	OpenAISharedHealthModeShared    OpenAISharedHealthMode = "shared"
)

var openAISharedHealthOwnerSequence atomic.Uint64

func newOpenAISharedHealthOwner() string {
	return fmt.Sprintf("gateway-%d-%d", time.Now().UnixNano(), openAISharedHealthOwnerSequence.Add(1))
}

func (s *OpenAIGatewayService) SetOpenAISharedHealthStore(store OpenAISharedHealthStore) {
	if s == nil {
		return
	}
	s.sharedHealthSnapshotMu.Lock()
	s.sharedHealthStore = store
	if s.sharedHealthSnapshots == nil {
		s.sharedHealthSnapshots = make(map[string]OpenAISharedHealthSnapshot)
	}
	if s.sharedHealthLeases == nil {
		s.sharedHealthLeases = make(map[string]OpenAISharedHalfOpenLease)
	}
	if s.sharedHealthOwner == "" {
		s.sharedHealthOwner = newOpenAISharedHealthOwner()
	}
	s.sharedHealthSnapshotMu.Unlock()
}

func (s *OpenAIGatewayService) openAISharedHealthConfig() OpenAISharedHealthConfig {
	cfg := DefaultOpenAISharedHealthConfig()
	if s != nil && s.cfg != nil {
		cfg = s.cfg.Gateway.OpenAISharedHealth
	}
	return cfg
}

func (s *OpenAIGatewayService) openAISharedHealthSelectionContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	timeout := time.Duration(s.openAISharedHealthConfig().RedisTimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > 100*time.Millisecond {
		timeout = time.Duration(DefaultOpenAISharedHealthConfig().RedisTimeoutMS) * time.Millisecond
	}
	return context.WithTimeout(parent, timeout)
}

func openAISharedHealthCacheKey(key OpenAISharedHealthKey) string {
	return strconv.FormatInt(key.AccountID, 10) + ":" + key.HashedSuffix()
}

func (s *OpenAIGatewayService) hasOpenAISharedHealthStore() bool {
	if s == nil {
		return false
	}
	s.sharedHealthSnapshotMu.Lock()
	hasStore := s.sharedHealthStore != nil
	s.sharedHealthSnapshotMu.Unlock()
	return hasStore
}

func (s *OpenAIGatewayService) readOpenAISharedHealthSnapshot(ctx context.Context, key OpenAISharedHealthKey, now time.Time, allowRemote bool) (OpenAISharedHealthSnapshot, bool, error) {
	if s == nil {
		return OpenAISharedHealthSnapshot{}, false, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	cacheKey := openAISharedHealthCacheKey(key)
	s.sharedHealthSnapshotMu.Lock()
	store := s.sharedHealthStore
	cached, cachedOK := s.sharedHealthSnapshots[cacheKey]
	s.sharedHealthSnapshotMu.Unlock()
	staleAfter := time.Duration(s.openAISharedHealthConfig().StaleAfterSeconds) * time.Second
	if staleAfter <= 0 || staleAfter > 30*time.Second {
		staleAfter = 30 * time.Second
	}
	if store == nil {
		return OpenAISharedHealthSnapshot{}, false, nil
	}
	if !allowRemote {
		if cachedOK && cached.Freshness(now, staleAfter) == OpenAISharedHealthFresh {
			return cached, true, nil
		}
		return OpenAISharedHealthSnapshot{Key: key, State: OpenAISharedHealthStateUnknown}, false, nil
	}
	readCtx, cancel := s.openAISharedHealthSelectionContext(ctx)
	defer cancel()
	snapshot, err := store.GetAccountModel(readCtx, key)
	if err == nil {
		if snapshot.Freshness(now, staleAfter) == OpenAISharedHealthFresh {
			s.sharedHealthSnapshotMu.Lock()
			s.sharedHealthSnapshots[cacheKey] = snapshot
			s.sharedHealthSnapshotMu.Unlock()
			return snapshot, true, nil
		}
		return OpenAISharedHealthSnapshot{Key: key, State: OpenAISharedHealthStateUnknown}, false, nil
	}
	if cachedOK && cached.Freshness(now, staleAfter) == OpenAISharedHealthFresh {
		return cached, true, err
	}
	return OpenAISharedHealthSnapshot{Key: key, State: OpenAISharedHealthStateUnknown}, false, err
}

func openAISharedHealthSnapshotBlocks(snapshot OpenAISharedHealthSnapshot) bool {
	return snapshot.State == OpenAISharedHealthStateCooldown || snapshot.State == OpenAISharedHealthStateHalfOpen
}

func (s *OpenAIGatewayService) recordOpenAISharedHealthFailure(ctx context.Context, event OpenAIAccountModelFailureEvent, now time.Time, local openAIAccountModelTransientDecision) (OpenAISharedHealthSnapshot, bool) {
	if s == nil {
		return OpenAISharedHealthSnapshot{}, false
	}
	key, err := NewOpenAISharedHealthKey(event.AccountID, event.CanonicalModel)
	if err != nil {
		return OpenAISharedHealthSnapshot{}, false
	}
	s.sharedHealthSnapshotMu.Lock()
	store := s.sharedHealthStore
	s.sharedHealthSnapshotMu.Unlock()
	if store == nil {
		return OpenAISharedHealthSnapshot{}, false
	}
	domains := event.Domains
	if len(domains) == 0 {
		if event.GroupID != nil && *event.GroupID > 0 {
			platform := strings.ToLower(strings.TrimSpace(event.Platform))
			if platform == "" {
				platform = "unknown"
			}
			domains = []OpenAIFailureDomain{{Type: OpenAIFailureDomainProviderChannel, ID: platform + ":channel:" + strconv.FormatInt(*event.GroupID, 10)}}
		} else {
			domains = []OpenAIFailureDomain{{Type: OpenAIFailureDomainUnknown, ID: "unknown"}}
		}
	}
	eventID := strings.TrimSpace(event.EventID)
	if eventID == "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%s|%d", event.AccountID, key.CanonicalModel, event.StatusCode, strings.ToLower(strings.TrimSpace(event.ErrorType)), now.UnixNano())))
		eventID = hex.EncodeToString(sum[:16])
	}
	sharedEvent := OpenAISharedHealthEvent{
		ID: eventID, Key: key, Domains: domains, StatusCode: event.StatusCode,
		ErrorType: event.ErrorType, TTFT: event.TTFT, ObservedAt: now, CooldownUntil: local.BlockUntil,
	}
	writeCtx, cancel := s.openAISharedHealthSelectionContext(ctx)
	defer cancel()
	snapshot, err := store.RecordAttempt(writeCtx, sharedEvent)
	if err != nil {
		slog.Warn("openai.shared_health.record_failed", "account_id", event.AccountID, "model_hash", key.HashedSuffix(), "failure", "shared_health_store_unavailable")
		return OpenAISharedHealthSnapshot{}, false
	}
	s.sharedHealthSnapshotMu.Lock()
	s.sharedHealthSnapshots[openAISharedHealthCacheKey(key)] = snapshot
	s.sharedHealthSnapshotMu.Unlock()
	return snapshot, true
}

func (s *OpenAIGatewayService) acquireOpenAISharedHalfOpenLease(ctx context.Context, key OpenAISharedHealthKey) (OpenAISharedHalfOpenLease, bool, error) {
	s.sharedHealthSnapshotMu.Lock()
	store, owner := s.sharedHealthStore, s.sharedHealthOwner
	s.sharedHealthSnapshotMu.Unlock()
	if store == nil {
		return OpenAISharedHalfOpenLease{}, false, nil
	}
	ttl := time.Duration(s.openAISharedHealthConfig().HalfOpenLeaseSeconds) * time.Second
	if ttl <= 0 || ttl > 15*time.Second {
		ttl = 15 * time.Second
	}
	leaseCtx, cancel := s.openAISharedHealthSelectionContext(ctx)
	defer cancel()
	return store.AcquireHalfOpen(leaseCtx, key, owner, ttl)
}

func (s *OpenAIGatewayService) storeOpenAISharedHalfOpenLease(lease OpenAISharedHalfOpenLease) {
	s.sharedHealthSnapshotMu.Lock()
	s.sharedHealthLeases[openAISharedHealthCacheKey(lease.Key)] = lease
	s.sharedHealthSnapshotMu.Unlock()
}

func (s *OpenAIGatewayService) takeOpenAISharedHalfOpenLease(key OpenAISharedHealthKey) (OpenAISharedHalfOpenLease, bool) {
	cacheKey := openAISharedHealthCacheKey(key)
	s.sharedHealthSnapshotMu.Lock()
	lease, ok := s.sharedHealthLeases[cacheKey]
	if ok {
		delete(s.sharedHealthLeases, cacheKey)
	}
	delete(s.sharedHealthSnapshots, cacheKey)
	s.sharedHealthSnapshotMu.Unlock()
	return lease, ok
}

func (s *OpenAIGatewayService) completeOpenAISharedHalfOpenLease(ctx context.Context, lease OpenAISharedHalfOpenLease, success bool, now time.Time) {
	s.sharedHealthSnapshotMu.Lock()
	store := s.sharedHealthStore
	s.sharedHealthSnapshotMu.Unlock()
	if store == nil {
		return
	}
	completeCtx, cancel := s.openAISharedHealthSelectionContext(ctx)
	defer cancel()
	if err := store.CompleteHalfOpen(completeCtx, lease, success, now); err != nil {
		slog.Warn("openai.shared_health.half_open_complete_failed", "account_id", lease.Key.AccountID, "model_hash", lease.Key.HashedSuffix(), "failure", "shared_health_store_unavailable")
	}
}
