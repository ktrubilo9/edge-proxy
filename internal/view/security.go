package view

import "edge-proxy/internal/config"

type SecurityConfigResponse = config.SecurityConfig

type RateLimitMetricsResponse struct {
	AllowedRequests uint64 `json:"allowed_requests"`
	BlockedRequests uint64 `json:"blocked_requests"`
}
