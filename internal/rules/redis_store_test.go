package rules

import "testing"

func TestRuleKeyFormat(t *testing.T) {
	store := &RedisStore{}
	key := store.ruleKey("payments", "/charge")

	expected := "rule:payments:/charge"
	if key != expected {
		t.Fatalf("expected %s, got %s", expected, key)
	}
}
