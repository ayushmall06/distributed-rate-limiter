package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"distributed-rate-limiter/internal/api"
	"distributed-rate-limiter/internal/limiter"
	"distributed-rate-limiter/internal/metrics"
	"distributed-rate-limiter/internal/rules"

	"github.com/prometheus/client_golang/prometheus/promhttp"

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

	// Register Prometheus metrics
	metrics.Register()

	// Expose Prometheus metrics endpoint
	http.Handle("/metrics", promhttp.Handler())

	// 2. Initialize RedisLimiter
	rl, err := limiter.NewRedisLimiter(rdb)
	if err != nil {
		log.Fatal("Failed to initialize RedisLimiter:", err)
	}

	ruleStore := rules.NewRedisStore(rdb)

	// ruleStore.Add(rules.Rule{
	// 	TenantId:   "search",
	// 	Resource:   "/search",
	// 	Capacity:   10,
	// 	RefillRate: 1,
	// })

	// 3. HTTP Health Check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	// 4. Rate Limit Endpoint
	http.HandleFunc("/v1/ratelimit/check", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var req api.RateLimitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			metrics.ErrorsTotal.Inc()
			return
		}

		if req.TenantId == "" || req.Resource == "" {
			http.Error(w, "tenant_id and resource are required", http.StatusBadRequest)
			metrics.ErrorsTotal.Inc()
			return
		}

		metrics.RequestsTotal.WithLabelValues(req.TenantId, req.Resource).Inc()

		redisKey := limiter.BuildKey(req.TenantId, req.Resource, req.Key)
		log.Printf(" Redis Key : %s\n", redisKey)

		now := time.Now().Unix()

		rule, ok, err := ruleStore.Get(ctx, req.TenantId, req.Resource)
		if err != nil {
			http.Error(w, "rule lookup failed", http.StatusInternalServerError)
			metrics.ErrorsTotal.Inc()
			return
		}
		if !ok {
			http.Error(w, "no rate limit rule found", http.StatusNotFound)
			metrics.ErrorsTotal.Inc()
			return
		}

		log.Printf(" Rule : %s\n", rule.Resource)

		allowed, remaining, retryAfterMs, err := rl.Allow(
			ctx,
			redisKey,
			now,
			rule.RefillRate,
			rule.Capacity,
			req.TokensRequested,
		)

		if err != nil {
			http.Error(w, "rate limiter error", http.StatusInternalServerError)
			metrics.ErrorsTotal.Inc()
			return
		}

		// Rate-limit headers
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
		w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(rule.Capacity, 10))

		if !allowed && retryAfterMs >= 0 {
			w.Header().Set("X-RateLimit-Retry-After-Ms", strconv.FormatInt(retryAfterMs, 10))
		}

		if allowed {
			metrics.AllowedTotal.WithLabelValues(req.TenantId, req.Resource).Inc()
		} else {
			metrics.BlockedTotal.WithLabelValues(req.TenantId, req.Resource).Inc()
		}

		resp := api.RateLimitResponse{
			Allowed:      allowed,
			Remaining:    remaining,
			RetryAfterMs: retryAfterMs,
		}

		w.Header().Set(
			"Content-Type", "application/json",
		)

		json.NewEncoder(w).Encode(resp)

		metrics.LatencyMs.WithLabelValues(req.TenantId, req.Resource).Observe(float64(time.Since(start).Milliseconds()))
	})

	http.HandleFunc("/v1/rules", func(w http.ResponseWriter, r *http.Request) {

		switch r.Method {
		case http.MethodPost:
			var req api.CreateRuleRequest

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}

			if err != nil {
				http.Error(w, "failed to store rule", http.StatusInternalServerError)
				return
			}

			ruleStore.Add(ctx, rules.Rule{
				TenantId:   req.TenantId,
				Resource:   req.Resource,
				Capacity:   req.Capacity,
				RefillRate: req.RefillRate,
			})

			w.WriteHeader(http.StatusCreated)

		case http.MethodGet:
			allRules, _ := ruleStore.List(ctx)
			json.NewEncoder(w).Encode(allRules)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
