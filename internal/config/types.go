package config

type FullConfig struct {
	Server       ServerConfig        `json:"server"`
	LoadBalancer LoadBalancingConfig `json:"load_balancing"`
	Backends     []*BackendConfig    `json:"backends"`
	HealthCheck  HealthCheckConfig   `json:"health_check"`
	Timeouts     TimeoutsConfig      `json:"timeouts"`
	Security     SecurityConfig      `json:"security"`
	VirtualHosts []VirtualHost       `json:"virtual_hosts"`
	Logging      LoggingConfig       `json:"logging"`
}

type ServerConfig struct {
	ProxyPort     int `json:"proxy_port"`
	AdminGrpcPort int `json:"admin_grpc_port"`
}

type LoadBalancingConfig struct {
	Strategy string `json:"strategy"`
}

type BackendConfig struct {
	Id      string `json:"id"`
	URL     string `json:"url"`
	Weight  int32  `json:"weight"`
	Enabled bool   `json:"enabled,omitempty"`
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
	BackendIDs  []string `json:"backend_ids"`
	StripPrefix bool     `json:"strip_prefix"`
}

type SecurityConfig struct {
	Policies []SecurityPolicy `json:"policies"`
}

type SecurityPolicy struct {
	Id           string             `json:"id"`
	RateLimiting RateLimitingConfig `json:"rate_limiting"`
}

type VirtualHost struct {
	Domain           string      `json:"domain"`
	BackendIDs       []string    `json:"backend_ids"`
	SecurityPolicyID string      `json:"security_policy_id"`
	PathRoutes       []PathRoute `json:"path_routes,omitempty"`
}

type LoggingConfig struct {
	Enabled    bool   `json:"enabled"`
	Level      string `json:"level"`
	Async      bool   `json:"async"`
	BufferSize int64  `json:"buffer_size"` // for async
}

type HealthCheckConfig struct {
	Enabled     bool                    `json:"enabled"`
	Probe       HealthProbeConfig       `json:"probe"`
	Schedule    HealthScheduleConfig    `json:"schedule"`
	Concurrency HealthConcurrencyConfig `json:"concurrency"`
	Thresholds  HealthThresholdConfig   `json:"thresholds"`
	Recovery    HealthRecoveryConfig    `json:"recovery"`
	Transport   HealthTransportConfig   `json:"transport"`
	Passive     PassiveHealthConfig     `json:"passive"`
}

type HealthProbeConfig struct {
	Type         string  `json:"type"`
	Path         string  `json:"path"`
	Method       string  `json:"method"`
	TimeoutMs    int64   `json:"timeout_ms"`
	SuccessCodes []int32 `json:"success_codes"`
}

type HealthScheduleConfig struct {
	IntervalMs int64 `json:"interval_ms"`
	JitterMs   int64 `json:"jitter_ms"`
}

type HealthConcurrencyConfig struct {
	Workers   int `json:"workers"`
	QueueSize int `json:"queue_size"`
}

type HealthThresholdConfig struct {
	Healthy   int32 `json:"healthy"`
	Unhealthy int32 `json:"unhealthy"`
}

type HealthRecoveryConfig struct {
	Backoff HealthBackoffConfig `json:"backoff"`
}

type HealthBackoffConfig struct {
	Enabled    bool    `json:"enabled"`
	InitialMs  int64   `json:"initial_ms"`
	MaxMs      int64   `json:"max_ms"`
	Multiplier float64 `json:"multiplier"`
}

type HealthTransportConfig struct {
	MaxIdleConns        int   `json:"max_idle_conns"`
	MaxIdleConnsPerHost int   `json:"max_idle_conns_per_host"`
	MaxConnsPerHost     int   `json:"max_conns_per_host"`
	KeepAliveMs         int64 `json:"keep_alive_ms"`
}

type PassiveHealthConfig struct {
	Enabled bool `json:"enabled"`
}
