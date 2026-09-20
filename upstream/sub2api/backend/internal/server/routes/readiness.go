package routes

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ReadinessChecker verifies whether the API's required runtime dependencies can accept work.
type ReadinessChecker interface {
	Check(context.Context) error
}

type databasePinger interface {
	PingContext(context.Context) error
}

type redisPinger interface {
	Ping(context.Context) *redis.StatusCmd
}

type redisReadinessPinger struct {
	options redis.Options
}

func newRedisReadinessPinger(client *redis.Client) redisPinger {
	if client == nil {
		return nil
	}
	options := *client.Options()
	options.ContextTimeoutEnabled = true
	options.MaxRetries = -1
	options.MinIdleConns = 0
	options.PoolSize = 1
	return &redisReadinessPinger{options: options}
}

func (p *redisReadinessPinger) Ping(ctx context.Context) *redis.StatusCmd {
	options := p.options
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return redis.NewStatusResult("", context.DeadlineExceeded)
		}
		options.DialTimeout = minPositiveDuration(options.DialTimeout, remaining)
		options.ReadTimeout = minPositiveDuration(options.ReadTimeout, remaining)
		options.WriteTimeout = minPositiveDuration(options.WriteTimeout, remaining)
	}
	client := redis.NewClient(&options)
	result := client.Ping(ctx)
	_ = client.Close()
	return result
}

func minPositiveDuration(configured, limit time.Duration) time.Duration {
	if configured <= 0 || configured > limit {
		return limit
	}
	return configured
}

type dependencyReadinessChecker struct {
	database databasePinger
	redis    redisPinger
}

// NewDependencyReadinessChecker builds the production readiness checker from existing clients.
func NewDependencyReadinessChecker(database *sql.DB, redisClient *redis.Client) ReadinessChecker {
	if database == nil || redisClient == nil {
		return newDependencyReadinessChecker(nil, nil)
	}
	return newDependencyReadinessChecker(database, newRedisReadinessPinger(redisClient))
}

func newDependencyReadinessChecker(database databasePinger, redisClient redisPinger) ReadinessChecker {
	return &dependencyReadinessChecker{database: database, redis: redisClient}
}

func (r *dependencyReadinessChecker) Check(ctx context.Context) error {
	if r.database == nil || r.redis == nil {
		return errors.New("readiness dependency unavailable")
	}
	if err := r.database.PingContext(ctx); err != nil {
		return err
	}
	return r.redis.Ping(ctx).Err()
}
