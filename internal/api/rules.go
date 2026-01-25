package api

type CreateRuleRequest struct {
	TenantId   string `json:"tenant_id"`
	Resource   string `json:"resource"`
	Capacity   int64  `json:"capacity"`
	RefillRate int64  `json:"refill_rate"`
}

type RuleResponse struct {
	TenantId   string `json:"tenant_id"`
	Resource   string `json:"resource"`
	Capacity   int64  `json:"capacity"`
	RefillRate int64  `json:"refill_rate"`
}
