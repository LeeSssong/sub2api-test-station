package service

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
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
	AccountID int64
	Name      string
	Platform  string
	Type      string
	Status    string
	BaseURL   string
	Snapshot  *AccountMonitorBalance
}

type UpstreamBalanceEvaluation struct {
	NormalizedBaseURL string
	ValueUSD          *float64
	ObservedAt        time.Time
	State             string
	Accounts          []UpstreamBalanceAccount
}

// NormalizeNotificationBaseURL returns the stable aggregation key for an
// upstream endpoint. Query strings, fragments and userinfo are rejected so a
// credential-bearing or request-specific URL can never become an event key.
func NormalizeNotificationBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("base URL is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("base URL is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("base URL scheme is invalid")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.EscapedPath(), "/")
	parsed.RawPath = ""
	parsed.ForceQuery = false
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if parsed.Path == "." {
		parsed.Path = ""
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// EvaluateUpstreamBaseURLBalance filters active OpenAI API-key accounts,
// groups them by normalized BaseURL, and selects one latest strict-valid USD
// snapshot per key. A same-time value conflict fails closed for the whole
// evaluation because the current value is not unique.
func EvaluateUpstreamBaseURLBalance(accounts []UpstreamBalanceAccount) ([]UpstreamBalanceEvaluation, error) {
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
		for i := range members {
			snapshot := members[i].Snapshot
			if !validNotificationSnapshot(snapshot) {
				continue
			}
			if latest == nil || snapshot.ObservedAt.After(*latest.ObservedAt) {
				latest = snapshot
				continue
			}
			if snapshot.ObservedAt.Equal(*latest.ObservedAt) && *snapshot.ValueUSD != *latest.ValueUSD {
				return nil, fmt.Errorf("base URL %q has conflicting balance snapshots", key)
			}
		}
		if latest == nil {
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

func validNotificationSnapshot(snapshot *AccountMonitorBalance) bool {
	if snapshot == nil || snapshot.Version != AccountMonitorBalanceVersion || snapshot.Status != AccountMonitorBalanceStatusOK ||
		(snapshot.Source != AccountMonitorBalanceSourceSub2API && snapshot.Source != AccountMonitorBalanceSourceNewAPI) ||
		snapshot.ValueUSD == nil || snapshot.ObservedAt == nil {
		return false
	}
	value := *snapshot.ValueUSD
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) && !snapshot.ObservedAt.IsZero()
}
