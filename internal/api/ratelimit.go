package api

type RateLimitRequest struct {
	TenantId        string `json:"tennant_id"`
	Resource        string `json:"resource"`
	Key             string `json:"key"`
	TokensRequested int64  `json:"tokens_requested"`
}

type RateLimitResponse struct {
	Allowed      bool  `json:"allowed"`
	Remaining    int64 `json:"remaining"`
	RetryAfterMs int64 `json:"retry_after_ms"`
}
