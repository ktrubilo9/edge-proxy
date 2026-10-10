package view

import "github.com/ktrubilo9/edge-proxy/internal/config"

type SecurityConfigResponse struct {
	RateLimiting config.RateLimitingConfig `json:"rate_limiting"`
}

type RateLimitMetricsResponse struct {
	AllowedRequests uint64 `json:"allowed_requests"`
	BlockedRequests uint64 `json:"blocked_requests"`
}
