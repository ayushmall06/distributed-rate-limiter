package rules

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func (s *RedisStore) ruleKey(tenant, resource string) string {
	return fmt.Sprintf("rule:%s:%s", tenant, resource)
}

func (s *RedisStore) Add(ctx context.Context, rule Rule) error {
	key := s.ruleKey(rule.TenantId, rule.Resource)

	return s.rdb.HSet(ctx, key, map[string]interface{}{
		"capacity":    rule.Capacity,
		"refill_rate": rule.RefillRate,
	}).Err()
}

func (s *RedisStore) Get(ctx context.Context, tenant, resource string) (Rule, bool, error) {
	key := s.ruleKey(tenant, resource)

	data, err := s.rdb.HGetAll(ctx, key).Result()
	if err != nil || len(data) == 0 {
		return Rule{}, false, err
	}

	capacity, _ := strconv.ParseInt(data["capacity"], 10, 64)
	refillRate, _ := strconv.ParseInt(data["refill_rate"], 10, 64)

	return Rule{
		TenantId:   tenant,
		Resource:   resource,
		Capacity:   capacity,
		RefillRate: refillRate,
	}, true, nil
}

func (s *RedisStore) List(ctx context.Context) ([]Rule, error) {
	keys, err := s.rdb.Keys(ctx, "rule:*").Result()
	if err != nil {
		return nil, err
	}

	var rules []Rule
	for _, key := range keys {
		parts := len(key)
		_ = parts
	}

	return rules, nil
}
