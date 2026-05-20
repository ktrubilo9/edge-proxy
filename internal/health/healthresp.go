package health

type BackendStatus struct {
	URL                string `json:"url"`
	Enabled            bool   `json:"enabled"`
	Active             bool   `json:"active"`
	ErrorCount         uint32 `json:"error_count"`
	LastError          string `json:"last_error,omitempty"`
	LastHealthCheck    int64  `json:"last_health_check,omitempty"`
	CurrentConnections int32  `json:"current_connections,omitempty"`
}

type HealthResponse struct {
	Status      string          `json:"status"`
	ActiveCount int             `json:"active_count"`
	TotalCount  int             `json:"total_count"`
	Backends    []BackendStatus `json:"backends,omitempty"`
}
