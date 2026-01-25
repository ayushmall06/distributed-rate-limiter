package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"distributed-rate-limiter/internal/api"
	"distributed-rate-limiter/internal/limiter"
	"distributed-rate-limiter/internal/rules"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	// 1. Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	log.Println("Connected to Redis successfully")

	// 2. Initialize RedisLimiter
	rl, err := limiter.NewRedisLimiter(rdb)
	if err != nil {
		log.Fatal("Failed to initialize RedisLimiter:", err)
	}

	ruleStore := rules.NewStore()

	ruleStore.Add(rules.Rule{
		TenantId:   "search",
		Resource:   "/search",
		Capacity:   10,
		RefillRate: 1,
	})

	// 3. HTTP Health Check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// 4. Rate Limit Endpoint
	http.HandleFunc("/v1/ratelimit/check", func(w http.ResponseWriter, r *http.Request) {
		var req api.RateLimitRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		log.Println("Requested tokens:", req.TokensRequested)

		redisKey := limiter.BuildKey(req.TenantId, req.Resource, req.Key)

		now := time.Now().Unix()

		rule, ok := ruleStore.Get(req.TenantId, req.Resource)

		if !ok {
			http.Error(w, "no rate limit rule found", http.StatusNotFound)
			return
		}

		allowed, remaining, err := rl.Allow(
			ctx,
			redisKey,
			now,
			rule.RefillRate,
			rule.Capacity,
			req.TokensRequested,
		)

		if err != nil {
			http.Error(w, "rate limiter error", http.StatusInternalServerError)
			return
		}

		resp := api.RateLimitResponse{
			Allowed:      allowed,
			Remaining:    remaining,
			RetryAfterMs: 0,
		}

		w.Header().Set(
			"Content-Type", "application/json",
		)

		json.NewEncoder(w).Encode(resp)
	})

	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
