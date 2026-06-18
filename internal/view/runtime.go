package view

import "edge-proxy/internal/config"

type BackendResponse struct {
	URL        string `json:"url"`
	Weight     int32  `json:"weight"`
	Enabled    bool   `json:"enabled"`
	Active     bool   `json:"active"`
	ErrorCount uint32 `json:"error_count"`
	LastError  string `json:"last_error,omitempty"`
}

type GlobalConfigResponse struct {
	ProxyPort  int    `json:"proxy_port"`
	LBStrategy string `json:"lb_strategy"`
}

type LoggingConfigResponse = config.LoggingConfig

type VirtualHostResponse struct {
	Domain     string                  `json:"domain"`
	Backends   []string                `json:"backends"`
	PathRoutes []config.PathRoute      `json:"path_routes,omitempty"`
	Security   *SecurityConfigResponse `json:"security,omitempty"`
}

type HealthCheckResponse = config.HealthCheckConfig

type TimeoutsResponse = config.TimeoutsConfig
