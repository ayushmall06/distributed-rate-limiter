package limiter

import "fmt"

func BuildKey(tenantId, resource, key string) string {
	return fmt.Sprintf("rl:%s:%s:%s", tenantId, resource, key)
}
