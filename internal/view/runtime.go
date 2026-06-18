package view

import "edge-proxy/internal/config"

type BackendResponse struct {
	Id         string `json:"id"`
	URL        string `json:"url"`
	Weight     int32  `json:"weight"`
	Enabled    bool   `json:"enabled"`
	Active     bool   `json:"active"`
	ErrorCount uint32 `json:"error_count"`
	LastError  string `json:"last_error,omitempty"`
}

type ServerConfigResponse struct {
	ProxyPort     int `json:"proxy_port"`
	AdminGrpcPort int `json:"admin_grpc_port"`
}

type LoadBalancerConfigResponse struct {
	Strategy string `json:"strategy"`
}

type LoggingConfigResponse = config.LoggingConfig

type VirtualHostResponse struct {
	Domain           string             `json:"domain"`
	BackendIDs       []string           `json:"backend_ids"`
	PathRoutes       []config.PathRoute `json:"path_routes,omitempty"`
	SecurityPolicyID string             `json:"security_policy_id"`
}

type SecurityPolicyResponse struct {
	Id           string                    `json:"id"`
	RateLimiting config.RateLimitingConfig `json:"rate_limiting"`
}

type VirtualHostSecurityResponse struct {
	Domain           string                 `json:"domain"`
	SecurityPolicyID string                 `json:"security_policy_id"`
	Policy           SecurityPolicyResponse `json:"policy"`
}

type HealthCheckResponse = config.HealthCheckConfig

type TimeoutsResponse = config.TimeoutsConfig
