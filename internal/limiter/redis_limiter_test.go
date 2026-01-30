package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupRedis(t *testing.T) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Redis must be running for integration tests: %v", err)
	}

	return rdb
}

func TestTokenBucket_NoRefill(t *testing.T) {
	ctx := context.Background()
	rdb := setupRedis(t)

	limiter, err := NewRedisLimiter(rdb)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	key := "r1:test:/test:user1"

	// clear any old state
	rdb.Del(ctx, key)

	// capacity = 3, refill = 0
	for i := 0; i < 3; i++ {
		allowed, remaining, _, err := limiter.Allow(ctx, key, time.Now().Unix(), 0, 3, 1)

		if err != nil {
			t.Fatal(err)
		}

		if !allowed {
			t.Fatal("Expected request to be allowed")
		}

		if remaining != int64(2-i) {
			t.Fatalf("Expected %d remaining, got %d", 2-i, remaining)
		}
	}

	// 4th request should be blocked
	allowed, _, retryAfter, _ := limiter.Allow(
		ctx, key, time.Now().Unix(), 0, 3, 1,
	)

	if allowed {
		t.Fatal("expected request to be blocked")
	}
	if retryAfter != -1 {
		t.Fatal("expected retry_after_ms = -1 for zero refill")
	}
}

func TestTokenBucket_WithRefill(t *testing.T) {
	ctx := context.Background()
	rdb := setupRedis(t)

	limiter, err := NewRedisLimiter(rdb)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	key := "rl:test:/refill:user1"
	rdb.Del(ctx, key)

	// First consume all tokens
	for i := 0; i < 2; i++ {
		limiter.Allow(ctx, key, time.Now().Unix(), 1, 2, 1)
	}

	// Wait for refill
	time.Sleep(2 * time.Second)

	allowed, remaining, _, _ := limiter.Allow(
		ctx, key, time.Now().Unix(), 1, 2, 1,
	)

	if !allowed {
		t.Fatal("expected request to be allowed after refill")
	}
	if remaining < 0 {
		t.Fatal("remaining tokens should not be negative")
	}
}
