package config

import (
	"time"
)

type FullConfig struct {
	ProxyPort    int               `json:"proxy_port"`
	LBStrategy   string            `json:"lb_strategy"`
	Backends     []*BackendConfig  `json:"backends"`
	HealthCheck  HealthCheckConfig `json:"health_check"`
	Timeouts     TimeoutsConfig    `json:"timeouts"`
	VirtualHosts []VirtualHost     `json:"virtual_hosts"`
	Logging      LoggingConfig     `json:"logging"`
}

type BackendConfig struct {
	URL     string `json:"url"`
	Weight  int32  `json:"weight"`
	Enabled bool   `json:"enabled,omitempty"`
}

type HealthCheckConfig struct {
	Path             string  `json:"path"`
	IntervalSeconds  int32   `json:"interval_seconds"`
	TimeoutSeconds   int32   `json:"timeout_seconds"`
	HealthyThreshold int32   `json:"healthy_threshold"`
	SuccessCodes     []int32 `json:"success_codes"`
}

type HealthCheckResponse = HealthCheckConfig

type TimeoutsConfig struct {
	ConnectTimeoutMs   int32 `json:"connect_timeout_ms"`
	ResponseTimeoutMs  int32 `json:"response_timeout_ms"`
	KeepAliveTimeoutMs int32 `json:"keep_alive_timeout_ms"`
	IdleConnTimeoutMs  int32 `json:"idle_conn_timeout_ms"`
}

type TimeoutsResponse = TimeoutsConfig

type RateLimitingConfig struct {
	Enabled   bool  `json:"enabled"`
	RatePerIP int32 `json:"rate_per_ip"`
	Burst     int32 `json:"burst"`
	WindowSec int32 `json:"window_sec"`
}

type PathRoute struct {
	Path        string   `json:"path"`
	Backends    []string `json:"backends"`
	StripPrefix bool     `json:"strip_prefix"`
}

type SecurityConfig struct {
	RateLimiting RateLimitingConfig `json:"rate_limiting"`
}

type VirtualHost struct {
	Domain     string          `json:"domain"`
	Backends   []string        `json:"backends"`
	Security   *SecurityConfig `json:"security,omitempty"`
	PathRoutes []PathRoute     `json:"path_routes,omitempty"`
}

type LoggingConfig struct {
	Enabled    bool   `json:"enabled"`
	Level      string `json:"level"`
	Async      bool   `json:"async"`
	BufferSize int64  `json:"buffer_size"` // for async
}

type LoggingConfigResponse = LoggingConfig

type GlobalConfigResponse struct {
	ProxyPort  int    `json:"proxy_port"`
	LBStrategy string `json:"lb_strategy"`
}

type BackendResponse struct {
	URL        string `json:"url"`
	Weight     int32  `json:"weight"`
	Enabled    bool   `json:"enabled"`
	Active     bool   `json:"active"`
	ErrorCount uint32 `json:"error_count"`
	LastError  string `json:"last_error,omitempty"`
}

type SecurityConfigResponse = SecurityConfig

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

type RateLimitMetricsResponse struct {
	AllowedRequests uint64 `json:"allowed_requests"`
	BlockedRequests uint64 `json:"blocked_requests"`
}

type VirtualHostResponse struct {
	Domain     string          `json:"domain"`
	Backends   []string        `json:"backends"`
	PathRoutes []PathRoute     `json:"path_routes,omitempty"`
	Security   *SecurityConfig `json:"security,omitempty"`
}

type ServerStatusResponse struct {
	Running           bool   `json:"running"`
	Uptime            uint64 `json:"uptime_seconds"`
	ActiveConnections int32  `json:"active_connections"`
	TotalBackends     int32  `json:"total_backends"`
	HealthyBackends   int32  `json:"healthy_backends"`
}
