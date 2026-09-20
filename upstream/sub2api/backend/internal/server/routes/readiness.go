package routes

import (
	"context"
	"database/sql"
	"errors"

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

type dependencyReadinessChecker struct {
	database databasePinger
	redis    redisPinger
}

// NewDependencyReadinessChecker builds the production readiness checker from existing clients.
func NewDependencyReadinessChecker(database *sql.DB, redisClient *redis.Client) ReadinessChecker {
	return newDependencyReadinessChecker(database, redisClient)
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
