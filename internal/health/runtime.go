package health

import (
	"github.com/ktrubilo9/edge-proxy/internal/config"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/runtime"
)

type HealthRuntime interface {
	GetBackend(id string) *config.BackendConfig
	GetBackends() []*config.BackendConfig
	GetHealthConfig() config.HealthCheckConfig
	GetBackendStatus(id string) (*runtime.BackendStatus, bool)
}
