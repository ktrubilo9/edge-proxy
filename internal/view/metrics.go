package view

import "time"

type ServerStatusResponse struct {
	Running           bool   `json:"running"`
	Uptime            uint64 `json:"uptime_seconds"`
	ActiveConnections int32  `json:"active_connections"`
	TotalBackends     int32  `json:"total_backends"`
	HealthyBackends   int32  `json:"healthy_backends"`
}

type SystemMetricsResponse struct {
	Timestamp     time.Time `json:"timestamp"`
	Goroutines    uint64    `json:"goroutines"`
	MemoryPercent float32   `json:"memory_percent"`
	CpuPercent    float32   `json:"cpu_percent"`
}

type BackendMetricsResponse struct {
	URL                string  `json:"url"`
	Requests           uint64  `json:"requests"`
	Failures           uint64  `json:"failures"`
	ActiveConnections  uint64  `json:"active_connections"`
	Timeouts           uint64  `json:"timeouts"`
	HealthChecks       uint64  `json:"health_checks"`
	FailedHealthChecks uint64  `json:"failed_health_checks"`
	LatencyEWMA        float64 `json:"latency_ewma"`
	ErrorRateEWMA      float64 `json:"error_rate_ewma"`
	CpuPercent         float64 `json:"cpu_percent"`
	MemoryPercent      float64 `json:"memory_percent"`
	Goroutines         uint32  `json:"goroutines"`
}

type BackendsMetricsResponse struct {
	Timestamp time.Time                `json:"timestamp"`
	Backends  []BackendMetricsResponse `json:"backends"`
}

type SecurityMetricsResponse struct {
	AllowedRequests uint64 `json:"allowed_requests"`
	BlockedRequests uint64 `json:"blocked_requests"`
}

type HTTPMetricsResponse struct {
	TotalRequestSize  uint64            `json:"total_request_size_bytes"`
	TotalResponseSize uint64            `json:"total_response_size_bytes"`
	StatusCodes       map[int32]uint64  `json:"status_codes"`
	MethodsCount      map[string]uint64 `json:"methods_count"`
}
