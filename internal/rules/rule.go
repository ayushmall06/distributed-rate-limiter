package rules

type Rule struct {
	TenantId   string
	Resource   string
	Capacity   int64
	RefillRate int64
}
