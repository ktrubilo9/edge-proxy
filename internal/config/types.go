package config

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

type TimeoutsConfig struct {
	ConnectTimeoutMs   int32 `json:"connect_timeout_ms"`
	ResponseTimeoutMs  int32 `json:"response_timeout_ms"`
	KeepAliveTimeoutMs int32 `json:"keep_alive_timeout_ms"`
	IdleConnTimeoutMs  int32 `json:"idle_conn_timeout_ms"`
}

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
