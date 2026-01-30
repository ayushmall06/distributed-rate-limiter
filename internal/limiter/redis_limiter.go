package limiter

import (
	"context"
	"fmt"

	_ "embed"

	"github.com/redis/go-redis/v9"
)

//go:embed token_bucket.lua
var tokenBucketLua string

type RedisLimiter struct {
	rdb    *redis.Client
	script *redis.Script
}

func NewRedisLimiter(rdb *redis.Client) (*RedisLimiter, error) {
	script := redis.NewScript(tokenBucketLua)

	return &RedisLimiter{
		rdb:    rdb,
		script: script,
	}, nil
}

func (l *RedisLimiter) Allow(
	ctx context.Context,
	key string,
	now int64,
	refillRate int64,
	capacity int64,
	tokensRequested int64,
) (bool, int64, int64, error) {

	res, err := l.script.Run(ctx, l.rdb, []string{key},
		now, refillRate, capacity, tokensRequested,
	).Result()

	if err != nil {
		fmt.Println("Lua error:", err)
		return false, 0, 0, err
	}

	arr := res.([]interface{})
	allowed := arr[0].(int64) == 1
	remaining := arr[1].(int64)
	retryAfterMs := arr[2].(int64)

	return allowed, remaining, retryAfterMs, nil
}
