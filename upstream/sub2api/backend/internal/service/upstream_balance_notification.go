package service

import (
	"math"
	"sort"
	"time"

	upstreamnotify "github.com/Wei-Shaw/sub2api/internal/notify"
)

const (
	UpstreamBalanceStateHealthy = "healthy"
	UpstreamBalanceStateLow     = "low"
	UpstreamBalanceStateZero    = "zero"
)

// UpstreamBalanceAccount is the non-sensitive account projection consumed by
// the BaseURL evaluator. Credentials are intentionally absent; the sender
// obtains login values from the protected registry only after claiming an event.
type UpstreamBalanceAccount struct {
	AccountID             int64
	Name                  string
	Platform              string
	Type                  string
	Status                string
	BaseURL               string
	Snapshot              *AccountMonitorBalance
	CredentialFingerprint string
	Ranks                 []UpstreamBalanceAccountRank
}

type UpstreamBalanceAccountRank struct {
	GroupName   string
	Rank        *int
	RankTotal   int
	Eligible    bool
	T114Enabled bool
}

type UpstreamBalanceEvaluation struct {
	NormalizedBaseURL string
	ValueUSD          *float64
	ObservedAt        time.Time
	State             string
	Accounts          []UpstreamBalanceAccount
	RankingSnapshotAt time.Time
	RankingStale      bool
}

// NormalizeNotificationBaseURL returns the stable aggregation key for an
// upstream endpoint. Query strings, fragments and userinfo are rejected so a
// credential-bearing or request-specific URL can never become an event key.
func NormalizeNotificationBaseURL(raw string) (string, error) {
	return upstreamnotify.NormalizeBaseURL(raw)
}

// EvaluateUpstreamBaseURLBalance filters active OpenAI API-key accounts,
// groups them by normalized BaseURL, and selects one latest strict-valid USD
// snapshot per key. A same-time value conflict fails closed for that scope
// because the current value is not unique, while unrelated scopes continue.
func EvaluateUpstreamBaseURLBalance(accounts []UpstreamBalanceAccount, now time.Time) ([]UpstreamBalanceEvaluation, error) {
	groups := make(map[string][]UpstreamBalanceAccount)
	for _, account := range accounts {
		if account.Status != StatusActive || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
			continue
		}
		key, err := NormalizeNotificationBaseURL(account.BaseURL)
		if err != nil {
			continue
		}
		groups[key] = append(groups[key], account)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]UpstreamBalanceEvaluation, 0, len(keys))
	for _, key := range keys {
		members := groups[key]
		sort.SliceStable(members, func(i, j int) bool { return members[i].AccountID < members[j].AccountID })
		var latest *AccountMonitorBalance
		conflict := false
		probeFailure := false
		for i := range members {
			snapshot := members[i].Snapshot
			if snapshot != nil && snapshot.Status != AccountMonitorBalanceStatusOK {
				probeFailure = true
			}
			if !validNotificationSnapshotForAccount(snapshot, members[i], now) {
				continue
			}
			if latest == nil || snapshot.ObservedAt.After(*latest.ObservedAt) {
				latest = snapshot
				continue
			}
			if snapshot.ObservedAt.Equal(*latest.ObservedAt) && *snapshot.ValueUSD != *latest.ValueUSD {
				conflict = true
				break
			}
		}
		if conflict || probeFailure || latest == nil {
			continue
		}
		value := *latest.ValueUSD
		state := UpstreamBalanceStateHealthy
		switch {
		case value == 0:
			state = UpstreamBalanceStateZero
		case value < 5:
			state = UpstreamBalanceStateLow
		}
		result = append(result, UpstreamBalanceEvaluation{
			NormalizedBaseURL: key,
			ValueUSD:          &value,
			ObservedAt:        latest.ObservedAt.UTC(),
			State:             state,
			Accounts:          members,
		})
	}
	return result, nil
}

func validNotificationSnapshotForAccount(snapshot *AccountMonitorBalance, account UpstreamBalanceAccount, now time.Time) bool {
	if !validNotificationSnapshot(snapshot) || snapshot.ObservedAt == nil {
		return false
	}
	age := now.UTC().Sub(snapshot.ObservedAt.UTC())
	if age < 0 || age > AccountMonitorBalanceMaxAge {
		return false
	}
	return snapshot.CredentialFingerprint != "" && snapshot.CredentialFingerprint == account.CredentialFingerprint
}

func validNotificationSnapshot(snapshot *AccountMonitorBalance) bool {
	if snapshot == nil || snapshot.Version != AccountMonitorBalanceVersion || snapshot.Status != AccountMonitorBalanceStatusOK ||
		(snapshot.Source != AccountMonitorBalanceSourceSub2API && snapshot.Source != AccountMonitorBalanceSourceNewAPI) ||
		snapshot.ValueUSD == nil || snapshot.ObservedAt == nil {
		return false
	}
	value := *snapshot.ValueUSD
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) && !snapshot.ObservedAt.IsZero()
}
