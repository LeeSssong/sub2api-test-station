package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
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
	OpenAISharedHealthStateHealthy    OpenAISharedHealthState = "healthy"
	OpenAISharedHealthStateSoftFailed OpenAISharedHealthState = "soft_failed"
	OpenAISharedHealthStateCooldown   OpenAISharedHealthState = "cooldown"
	OpenAISharedHealthStateHalfOpen   OpenAISharedHealthState = "half_open"
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

func (s *OpenAIGatewayService) SetOpenAISharedHealthStore(store OpenAISharedHealthStore) {
	if s == nil {
		return
	}
	s.sharedHealthSnapshotMu.Lock()
	s.sharedHealthStore = store
	if s.sharedHealthSnapshots == nil {
		s.sharedHealthSnapshots = make(map[string]OpenAISharedHealthSnapshot)
	}
	s.sharedHealthSnapshotMu.Unlock()
}
