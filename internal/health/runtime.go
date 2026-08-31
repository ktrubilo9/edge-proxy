package health

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
)

type HealthRuntime interface {
	GetBackend(id string) *config.BackendConfig
	GetBackends() []*config.BackendConfig
	GetHealthConfig() config.HealthCheckConfig
	GetBackendStatus(id string) (*runtime.BackendStatus, bool)
}
