package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	openAISharedHealthKeyPrefix       = "sub2api:openai:shared-health:v1:"
	openAISharedHealthSchemaVersion   = 1
	openAISharedHealthEventTTL        = 10 * time.Minute
	openAISharedHealthHealthyTTL      = 30 * time.Minute
	openAISharedHealthCooldownTailTTL = 10 * time.Minute
	openAISharedHealthFenceTTL        = 24 * time.Hour
	openAISharedHealthEWMAAlpha       = 0.2
	openAISharedHealthDefaultTimeout  = 75 * time.Millisecond
)

var openAISharedHealthRecordAttemptScript = redis.NewScript(`
local marker = redis.call("SET", KEYS[1], "1", "PX", ARGV[1], "NX")
if not marker then
  return 0
end

local success = ARGV[5] == "1"
local observed_ms = tonumber(ARGV[9])
local cooldown_ms = tonumber(ARGV[10])
local ttl_ms = tonumber(ARGV[11])
local alpha = tonumber(ARGV[12])

local function update_state(key)
  local revision = redis.call("HINCRBY", key, "revision", 1)
  local streak = tonumber(redis.call("HGET", key, "failure_streak") or "0")
  local existing_cooldown_ms = tonumber(redis.call("HGET", key, "cooldown_until_unix_ms") or "0")
  local sample_error = 1
  local state = "soft_failed"
  if success then
    sample_error = 0
    if existing_cooldown_ms > observed_ms then
      state = "cooldown"
      cooldown_ms = existing_cooldown_ms
    else
      streak = 0
      state = "healthy"
      cooldown_ms = 0
    end
  else
    streak = streak + 1
    if cooldown_ms > observed_ms then
      state = "cooldown"
    end
  end

  local old_error = redis.call("HGET", key, "ewma_error_rate")
  local ewma_error = sample_error
  if old_error then
    ewma_error = alpha * sample_error + (1 - alpha) * tonumber(old_error)
  end

  local ttft_ms = tonumber(ARGV[8])
  local old_ttft = redis.call("HGET", key, "ewma_ttft_ms")
  local ewma_ttft = old_ttft and tonumber(old_ttft) or 0
  if ttft_ms > 0 then
    if old_ttft then
      ewma_ttft = alpha * ttft_ms + (1 - alpha) * tonumber(old_ttft)
    else
      ewma_ttft = ttft_ms
    end
  end

  redis.call("HSET", key,
    "schema_version", ARGV[2],
    "account_id", ARGV[3],
    "canonical_model", ARGV[4],
    "state", state,
    "failure_streak", streak,
    "half_open_successes", 0,
    "cooldown_until_unix_ms", cooldown_ms,
    "last_status_code", ARGV[6],
    "last_error_type", ARGV[7],
    "ewma_error_rate", ewma_error,
    "ewma_ttft_ms", ewma_ttft,
    "observed_at_unix_ms", observed_ms)
  redis.call("PEXPIRE", key, ttl_ms)
  return revision
end

for index = 2, #KEYS do
  update_state(KEYS[index])
end
return 1
`)

var openAISharedHealthAcquireLeaseScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  return {0, tonumber(redis.call("HGET", KEYS[1], "fencing_token") or "0")}
end
local token = redis.call("INCR", KEYS[2])
redis.call("HSET", KEYS[1],
  "owner", ARGV[1],
  "fencing_token", token,
  "issued_at_unix_ms", ARGV[2],
  "expires_at_unix_ms", ARGV[3])
redis.call("PEXPIRE", KEYS[1], ARGV[4])
redis.call("PEXPIRE", KEYS[2], ARGV[5])
return {1, token}
`)

var openAISharedHealthCompleteLeaseScript = redis.NewScript(`
local owner = redis.call("HGET", KEYS[1], "owner")
local token = redis.call("HGET", KEYS[1], "fencing_token")
if not owner or owner ~= ARGV[1] or not token or token ~= ARGV[2] then
  return 0
end
redis.call("DEL", KEYS[1])

local success = ARGV[3] == "1"
local observed_ms = tonumber(ARGV[4])
local streak = tonumber(redis.call("HGET", KEYS[2], "failure_streak") or "0")
local half_open_successes = tonumber(redis.call("HGET", KEYS[2], "half_open_successes") or "0")
local old_error = redis.call("HGET", KEYS[2], "ewma_error_rate")
local sample_error = 1
local state = "cooldown"
local cooldown_ms = 0
local ttl_ms = tonumber(ARGV[8])
if success then
  streak = 0
  sample_error = 0
  half_open_successes = half_open_successes + 1
  if half_open_successes >= 2 then
    half_open_successes = 0
    state = "healthy"
  else
    state = "half_open"
    -- Keep the account blocked while requiring the second independent probe.
    cooldown_ms = observed_ms
  end
else
  half_open_successes = 0
  streak = streak + 1
  local cooldown_seconds = 10
  if streak >= 3 then
    cooldown_seconds = 45
  end
  cooldown_ms = observed_ms + cooldown_seconds * 1000
  ttl_ms = cooldown_seconds * 1000 + tonumber(ARGV[9])
end

local alpha = tonumber(ARGV[10])
local ewma_error = sample_error
if old_error then
  ewma_error = alpha * sample_error + (1 - alpha) * tonumber(old_error)
end

redis.call("HINCRBY", KEYS[2], "revision", 1)
redis.call("HSET", KEYS[2],
  "schema_version", ARGV[5],
  "account_id", ARGV[6],
  "canonical_model", ARGV[7],
  "state", state,
  "failure_streak", streak,
  "half_open_successes", half_open_successes,
  "cooldown_until_unix_ms", cooldown_ms,
  "last_status_code", 0,
  "last_error_type", success and "" or "half_open_failed",
  "ewma_error_rate", ewma_error,
  "observed_at_unix_ms", observed_ms)
redis.call("PEXPIRE", KEYS[2], ttl_ms)
return 1
`)

var openAISharedHealthAcquireAdmissionScript = redis.NewScript(`
local now_ms = tonumber(ARGV[1])
local ttl_ms = tonumber(ARGV[2])
local stalled_ms = tonumber(ARGV[3])
local max_normal = tonumber(ARGV[4])
local max_long = tonumber(ARGV[5])
local requested_shape = ARGV[6]
local member = ARGV[7]

if redis.call("EXISTS", KEYS[2]) == 1 then
  return {0, "slow_session_guard", 0, 0, 0, 0}
end

local normal = 0
local long = 0
local stalled = 0
local entries = redis.call("HGETALL", KEYS[1])
for index = 1, #entries, 2 do
  local entry_member = entries[index]
  local shape, issued_ms, expires_ms = string.match(entries[index + 1], "^([^|]+)|(%d+)|(%d+)$")
  issued_ms = tonumber(issued_ms)
  expires_ms = tonumber(expires_ms)
  if not shape or not issued_ms or not expires_ms or expires_ms <= now_ms then
    redis.call("HDEL", KEYS[1], entry_member)
  else
    if shape == "normal" then normal = normal + 1 end
    if shape == "long" then long = long + 1 end
    if now_ms - issued_ms >= stalled_ms then stalled = 1 end
  end
end

if stalled == 1 then
  return {0, "stalled_pre_first_output", normal, long, 1, 0}
end
if long > 0 then
  return {0, "long_pre_first_output", normal, long, 0, 0}
end
if requested_shape == "normal" and normal >= max_normal then
  return {0, "normal_pre_first_output_capacity", normal, long, 0, 0}
end
if requested_shape == "long" and long >= max_long then
  return {0, "long_pre_first_output_capacity", normal, long, 0, 0}
end

local expires_ms = now_ms + ttl_ms
redis.call("HSET", KEYS[1], member, requested_shape .. "|" .. now_ms .. "|" .. expires_ms)
redis.call("PEXPIRE", KEYS[1], ttl_ms)
if requested_shape == "normal" then normal = normal + 1 end
if requested_shape == "long" then long = long + 1 end
return {1, "acquired", normal, long, 0, expires_ms}
`)

var openAISharedHealthRenewAdmissionScript = redis.NewScript(`
local current = redis.call("HGET", KEYS[1], ARGV[1])
if not current then return 0 end
local shape, issued_ms = string.match(current, "^([^|]+)|(%d+)|%d+$")
if not shape or shape ~= ARGV[2] or not issued_ms then return 0 end
redis.call("HSET", KEYS[1], ARGV[1], shape .. "|" .. issued_ms .. "|" .. ARGV[3])
redis.call("PEXPIRE", KEYS[1], ARGV[4])
return 1
`)

var openAISharedHealthReleaseAdmissionScript = redis.NewScript(`
redis.call("HDEL", KEYS[1], ARGV[1])
return 1
`)

var openAISharedHealthRecordSlowSessionGuardScript = redis.NewScript(`
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return 1
`)

type openAISharedHealthStore struct {
	rdb             *redis.Client
	timeout         time.Duration
	now             func() time.Time
	admissionConfig config.GatewayOpenAISharedHealthConfig
}

func NewOpenAISharedHealthStore(rdb *redis.Client) service.OpenAISharedHealthStore {
	return newOpenAISharedHealthStore(rdb, openAISharedHealthDefaultTimeout)
}

func newOpenAISharedHealthStore(rdb *redis.Client, timeout time.Duration) *openAISharedHealthStore {
	return newOpenAISharedHealthStoreWithConfig(rdb, timeout, config.DefaultGatewayOpenAISharedHealthConfig())
}

func newOpenAISharedHealthStoreWithConfig(rdb *redis.Client, timeout time.Duration, admissionConfig config.GatewayOpenAISharedHealthConfig) *openAISharedHealthStore {
	if timeout <= 0 {
		timeout = openAISharedHealthDefaultTimeout
	}
	return &openAISharedHealthStore{rdb: rdb, timeout: timeout, now: time.Now, admissionConfig: admissionConfig}
}

func ProvideOpenAISharedHealthStore(rdb *redis.Client, cfg *config.Config) service.OpenAISharedHealthStore {
	timeout := openAISharedHealthDefaultTimeout
	if cfg != nil && cfg.Gateway.OpenAISharedHealth.RedisTimeoutMS > 0 {
		timeout = time.Duration(cfg.Gateway.OpenAISharedHealth.RedisTimeoutMS) * time.Millisecond
	}
	admissionConfig := config.DefaultGatewayOpenAISharedHealthConfig()
	if cfg != nil {
		admissionConfig = cfg.Gateway.OpenAISharedHealth
	}
	return newOpenAISharedHealthStoreWithConfig(rdb, timeout, admissionConfig)
}

func (s *openAISharedHealthStore) GetAccountModel(ctx context.Context, key service.OpenAISharedHealthKey) (service.OpenAISharedHealthSnapshot, error) {
	if s == nil || s.rdb == nil {
		return service.OpenAISharedHealthSnapshot{Key: key, State: service.OpenAISharedHealthStateUnknown}, errors.New("OpenAI shared health Redis client is unavailable")
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	values, err := s.rdb.HGetAll(ctx, openAISharedHealthAccountModelKey(key)).Result()
	if err != nil {
		return service.OpenAISharedHealthSnapshot{Key: key, State: service.OpenAISharedHealthStateUnknown}, err
	}
	if len(values) == 0 {
		return service.OpenAISharedHealthSnapshot{SchemaVersion: openAISharedHealthSchemaVersion, Key: key, State: service.OpenAISharedHealthStateUnknown}, nil
	}
	return decodeOpenAISharedHealthSnapshot(key, values)
}

func (s *openAISharedHealthStore) RecordAttempt(ctx context.Context, event service.OpenAISharedHealthEvent) (service.OpenAISharedHealthSnapshot, error) {
	if s == nil || s.rdb == nil {
		return service.OpenAISharedHealthSnapshot{Key: event.Key, State: service.OpenAISharedHealthStateUnknown}, errors.New("OpenAI shared health Redis client is unavailable")
	}
	if strings.TrimSpace(event.ID) == "" {
		return service.OpenAISharedHealthSnapshot{Key: event.Key, State: service.OpenAISharedHealthStateUnknown}, errors.New("OpenAI shared health event id is required")
	}
	if event.Key.AccountID <= 0 || strings.TrimSpace(event.Key.CanonicalModel) == "" {
		return service.OpenAISharedHealthSnapshot{Key: event.Key, State: service.OpenAISharedHealthStateUnknown}, errors.New("invalid OpenAI shared health key")
	}
	observedAt := event.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	}
	keys := []string{openAISharedHealthEventKey(event.ID), openAISharedHealthAccountModelKey(event.Key)}
	seenDomains := make(map[string]struct{}, len(event.Domains))
	for _, domain := range event.Domains {
		domainKey := openAISharedHealthFailureDomainKey(domain, event.Key.CanonicalModel)
		if _, exists := seenDomains[domainKey]; exists {
			continue
		}
		seenDomains[domainKey] = struct{}{}
		keys = append(keys, domainKey)
	}
	stateTTL := openAISharedHealthHealthyTTL
	if event.CooldownUntil.After(observedAt) {
		stateTTL = event.CooldownUntil.Sub(observedAt) + openAISharedHealthCooldownTailTTL
	}
	lastErrorType := truncateOpenAISharedHealthValue(event.ErrorType, 128)
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	_, err := openAISharedHealthRecordAttemptScript.Run(ctx, s.rdb, keys,
		openAISharedHealthEventTTL.Milliseconds(),
		openAISharedHealthSchemaVersion,
		event.Key.AccountID,
		truncateOpenAISharedHealthValue(event.Key.CanonicalModel, 128),
		boolInt(event.Success),
		event.StatusCode,
		lastErrorType,
		event.TTFT.Milliseconds(),
		observedAt.UnixMilli(),
		timeToUnixMilli(event.CooldownUntil),
		stateTTL.Milliseconds(),
		openAISharedHealthEWMAAlpha,
	).Result()
	if err != nil {
		return service.OpenAISharedHealthSnapshot{Key: event.Key, State: service.OpenAISharedHealthStateUnknown}, err
	}
	values, err := s.rdb.HGetAll(ctx, openAISharedHealthAccountModelKey(event.Key)).Result()
	if err != nil {
		return service.OpenAISharedHealthSnapshot{Key: event.Key, State: service.OpenAISharedHealthStateUnknown}, err
	}
	return decodeOpenAISharedHealthSnapshot(event.Key, values)
}

func (s *openAISharedHealthStore) AcquireHalfOpen(ctx context.Context, key service.OpenAISharedHealthKey, owner string, ttl time.Duration) (service.OpenAISharedHalfOpenLease, bool, error) {
	owner = truncateOpenAISharedHealthValue(strings.TrimSpace(owner), 128)
	if s == nil || s.rdb == nil {
		return service.OpenAISharedHalfOpenLease{}, false, errors.New("OpenAI shared health Redis client is unavailable")
	}
	if owner == "" {
		return service.OpenAISharedHalfOpenLease{}, false, errors.New("OpenAI shared health lease owner is required")
	}
	if ttl <= 0 {
		return service.OpenAISharedHalfOpenLease{}, false, errors.New("OpenAI shared health lease ttl must be positive")
	}
	now := s.now().UTC()
	expiresAt := now.Add(ttl)
	leaseKey := openAISharedHealthLeaseKey(key)
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	result, err := openAISharedHealthAcquireLeaseScript.Run(ctx, s.rdb, []string{leaseKey, leaseKey + ":fence"},
		owner, now.UnixMilli(), expiresAt.UnixMilli(), ttl.Milliseconds(), openAISharedHealthFenceTTL.Milliseconds(),
	).Slice()
	if err != nil {
		return service.OpenAISharedHalfOpenLease{}, false, err
	}
	if len(result) != 2 {
		return service.OpenAISharedHalfOpenLease{}, false, fmt.Errorf("unexpected OpenAI shared health lease response")
	}
	ok, err := redisInteger(result[0])
	if err != nil {
		return service.OpenAISharedHalfOpenLease{}, false, err
	}
	token, err := redisInteger(result[1])
	if err != nil {
		return service.OpenAISharedHalfOpenLease{}, false, err
	}
	if ok == 0 {
		return service.OpenAISharedHalfOpenLease{}, false, nil
	}
	return service.OpenAISharedHalfOpenLease{Key: key, Owner: owner, FencingToken: token, IssuedAt: now, ExpiresAt: expiresAt}, true, nil
}

func (s *openAISharedHealthStore) CompleteHalfOpen(ctx context.Context, lease service.OpenAISharedHalfOpenLease, success bool, observedAt time.Time) error {
	if s == nil || s.rdb == nil {
		return errors.New("OpenAI shared health Redis client is unavailable")
	}
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	leaseKey := openAISharedHealthLeaseKey(lease.Key)
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	result, err := openAISharedHealthCompleteLeaseScript.Run(ctx, s.rdb, []string{leaseKey, openAISharedHealthAccountModelKey(lease.Key)},
		truncateOpenAISharedHealthValue(lease.Owner, 128),
		lease.FencingToken,
		boolInt(success),
		observedAt.UnixMilli(),
		openAISharedHealthSchemaVersion,
		lease.Key.AccountID,
		truncateOpenAISharedHealthValue(lease.Key.CanonicalModel, 128),
		openAISharedHealthHealthyTTL.Milliseconds(),
		openAISharedHealthCooldownTailTTL.Milliseconds(),
		openAISharedHealthEWMAAlpha,
	).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return service.ErrOpenAISharedHealthLeaseLost
	}
	return nil
}

func (s *openAISharedHealthStore) AcquireAdmission(ctx context.Context, request service.OpenAISharedAdmissionRequest) (service.OpenAISharedAdmissionDecision, error) {
	if s == nil || s.rdb == nil {
		return service.OpenAISharedAdmissionDecision{}, errors.New("OpenAI shared health Redis client is unavailable")
	}
	if err := validateOpenAISharedAdmissionRequest(request); err != nil {
		return service.OpenAISharedAdmissionDecision{}, err
	}
	observedAt := request.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	}
	leaseTTL := time.Duration(s.admissionConfig.AdmissionLeaseTTLSeconds) * time.Second
	stalledAfter := time.Duration(s.admissionConfig.StalledBeforeFirstOutputSeconds) * time.Second
	member := openAISharedHealthHash(request.LeaseID)
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	values, err := openAISharedHealthAcquireAdmissionScript.Run(ctx, s.rdb, []string{
		openAISharedHealthAdmissionKey(request.AccountID),
		openAISharedHealthSlowSessionGuardKey(request.AccountID),
	},
		observedAt.UnixMilli(),
		leaseTTL.Milliseconds(),
		stalledAfter.Milliseconds(),
		s.admissionConfig.MaxPreFirstOutputNormal,
		s.admissionConfig.MaxPreFirstOutputLong,
		string(request.Shape),
		member,
	).Slice()
	if err != nil {
		return service.OpenAISharedAdmissionDecision{}, err
	}
	if len(values) != 6 {
		return service.OpenAISharedAdmissionDecision{}, fmt.Errorf("unexpected OpenAI shared health admission response")
	}
	allowed, err := redisInteger(values[0])
	if err != nil {
		return service.OpenAISharedAdmissionDecision{}, err
	}
	normal, err := redisInteger(values[2])
	if err != nil {
		return service.OpenAISharedAdmissionDecision{}, err
	}
	long, err := redisInteger(values[3])
	if err != nil {
		return service.OpenAISharedAdmissionDecision{}, err
	}
	stalled, err := redisInteger(values[4])
	if err != nil {
		return service.OpenAISharedAdmissionDecision{}, err
	}
	expiresMS, err := redisInteger(values[5])
	if err != nil {
		return service.OpenAISharedAdmissionDecision{}, err
	}
	return service.OpenAISharedAdmissionDecision{
		Allowed:        allowed == 1,
		Reason:         redisString(values[1]),
		ActiveNormal:   int(normal),
		ActiveLong:     int(long),
		Stalled:        stalled == 1,
		LeaseExpiresAt: unixMilliTime(expiresMS),
	}, nil
}

func (s *openAISharedHealthStore) RenewAdmission(ctx context.Context, request service.OpenAISharedAdmissionRequest) error {
	if s == nil || s.rdb == nil {
		return errors.New("OpenAI shared health Redis client is unavailable")
	}
	if err := validateOpenAISharedAdmissionRequest(request); err != nil {
		return err
	}
	observedAt := request.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	}
	leaseTTL := time.Duration(s.admissionConfig.AdmissionLeaseTTLSeconds) * time.Second
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	result, err := openAISharedHealthRenewAdmissionScript.Run(ctx, s.rdb, []string{openAISharedHealthAdmissionKey(request.AccountID)},
		openAISharedHealthHash(request.LeaseID), string(request.Shape), observedAt.Add(leaseTTL).UnixMilli(), leaseTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return err
	}
	if result == 0 {
		return service.ErrOpenAISharedHealthLeaseLost
	}
	return nil
}

func (s *openAISharedHealthStore) ReleaseAdmission(ctx context.Context, request service.OpenAISharedAdmissionRequest) error {
	if s == nil || s.rdb == nil {
		return errors.New("OpenAI shared health Redis client is unavailable")
	}
	if err := validateOpenAISharedAdmissionRequest(request); err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	_, err := openAISharedHealthReleaseAdmissionScript.Run(ctx, s.rdb, []string{openAISharedHealthAdmissionKey(request.AccountID)}, openAISharedHealthHash(request.LeaseID)).Result()
	return err
}

func (s *openAISharedHealthStore) RecordSlowSessionGuard(ctx context.Context, accountID int64, observedAt time.Time) error {
	if s == nil || s.rdb == nil {
		return errors.New("OpenAI shared health Redis client is unavailable")
	}
	if accountID <= 0 {
		return errors.New("OpenAI shared slow-session guard account id must be positive")
	}
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	ttl := time.Duration(s.admissionConfig.SlowSessionGuardSeconds) * time.Second
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	_, err := openAISharedHealthRecordSlowSessionGuardScript.Run(ctx, s.rdb, []string{openAISharedHealthSlowSessionGuardKey(accountID)}, observedAt.UnixMilli(), ttl.Milliseconds()).Result()
	return err
}

func (s *openAISharedHealthStore) HasSlowSessionGuard(ctx context.Context, accountID int64) (bool, error) {
	if s == nil || s.rdb == nil {
		return false, errors.New("OpenAI shared health Redis client is unavailable")
	}
	if accountID <= 0 {
		return false, errors.New("OpenAI shared slow-session guard account id must be positive")
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	exists, err := s.rdb.Exists(ctx, openAISharedHealthSlowSessionGuardKey(accountID)).Result()
	return exists == 1, err
}

func (s *openAISharedHealthStore) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, s.timeout)
}

func decodeOpenAISharedHealthSnapshot(key service.OpenAISharedHealthKey, values map[string]string) (service.OpenAISharedHealthSnapshot, error) {
	snapshot := service.OpenAISharedHealthSnapshot{Key: key, State: service.OpenAISharedHealthStateUnknown}
	schemaVersion, err := strconv.Atoi(values["schema_version"])
	if err != nil || schemaVersion != openAISharedHealthSchemaVersion {
		return snapshot, service.ErrOpenAISharedHealthUnknownSchema
	}
	snapshot.SchemaVersion = schemaVersion
	snapshot.Revision, _ = strconv.ParseInt(values["revision"], 10, 64)
	snapshot.FailureStreak, _ = strconv.Atoi(values["failure_streak"])
	snapshot.HalfOpenSuccesses, _ = strconv.Atoi(values["half_open_successes"])
	snapshot.LastStatusCode, _ = strconv.Atoi(values["last_status_code"])
	snapshot.LastErrorType = values["last_error_type"]
	snapshot.EWMAErrorRate, _ = strconv.ParseFloat(values["ewma_error_rate"], 64)
	if ttftMS, parseErr := strconv.ParseFloat(values["ewma_ttft_ms"], 64); parseErr == nil {
		snapshot.EWMATTFT = time.Duration(ttftMS * float64(time.Millisecond))
	}
	if observedMS, parseErr := strconv.ParseInt(values["observed_at_unix_ms"], 10, 64); parseErr == nil && observedMS > 0 {
		snapshot.ObservedAt = time.UnixMilli(observedMS).UTC()
	}
	if cooldownMS, parseErr := strconv.ParseInt(values["cooldown_until_unix_ms"], 10, 64); parseErr == nil && cooldownMS > 0 {
		snapshot.CooldownUntil = time.UnixMilli(cooldownMS).UTC()
	}
	switch state := service.OpenAISharedHealthState(values["state"]); state {
	case service.OpenAISharedHealthStateHealthy, service.OpenAISharedHealthStateSoftFailed, service.OpenAISharedHealthStateCooldown, service.OpenAISharedHealthStateHalfOpen:
		snapshot.State = state
	default:
		return snapshot, fmt.Errorf("invalid OpenAI shared health state %q", values["state"])
	}
	return snapshot, nil
}

func openAISharedHealthAccountModelKey(key service.OpenAISharedHealthKey) string {
	return fmt.Sprintf("%saccount-model:%d:%s", openAISharedHealthKeyPrefix, key.AccountID, key.HashedSuffix())
}

func openAISharedHealthFailureDomainKey(domain service.OpenAIFailureDomain, canonicalModel string) string {
	return fmt.Sprintf("%sdomain:%s:%s:%s", openAISharedHealthKeyPrefix, domain.Type, openAISharedHealthHash(domain.ID), openAISharedHealthHash(strings.ToLower(strings.TrimSpace(canonicalModel))))
}

func openAISharedHealthLeaseKey(key service.OpenAISharedHealthKey) string {
	return fmt.Sprintf("%slease:%d:%s", openAISharedHealthKeyPrefix, key.AccountID, key.HashedSuffix())
}

func openAISharedHealthAdmissionKey(accountID int64) string {
	return fmt.Sprintf("%sadmission:%d", openAISharedHealthKeyPrefix, accountID)
}

func openAISharedHealthSlowSessionGuardKey(accountID int64) string {
	return fmt.Sprintf("%sslow-session:%d", openAISharedHealthKeyPrefix, accountID)
}

func openAISharedHealthEventKey(eventID string) string {
	return openAISharedHealthKeyPrefix + "event:" + openAISharedHealthHash(eventID)
}

func openAISharedHealthHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:16])
}

func truncateOpenAISharedHealthValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func timeToUnixMilli(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func redisInteger(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer type %T", value)
	}
}

func redisString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

func unixMilliTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func validateOpenAISharedAdmissionRequest(request service.OpenAISharedAdmissionRequest) error {
	if request.AccountID <= 0 {
		return errors.New("OpenAI shared admission account id must be positive")
	}
	if request.Shape != service.OpenAIAdmissionShapeNormal && request.Shape != service.OpenAIAdmissionShapeLong {
		return errors.New("OpenAI shared request shape must be normal or long")
	}
	if strings.TrimSpace(request.LeaseID) == "" {
		return errors.New("OpenAI shared admission lease id is required")
	}
	return nil
}
