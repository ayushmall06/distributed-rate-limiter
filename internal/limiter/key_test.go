package limiter

import "testing"

func TestBuildKey(t *testing.T) {
	key := BuildKey("payments", "/charge", "user1")
	expected := "rl:payments:/charge:user1"

	if key != expected {
		t.Fatalf("expected %s, got %s", expected, key)
	}
}
