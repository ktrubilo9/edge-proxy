package metrics

type SecurityMetrics struct {
	RateLimitStats struct {
		AllowedRequests uint64
		BlockedRequests uint64
	}
}
