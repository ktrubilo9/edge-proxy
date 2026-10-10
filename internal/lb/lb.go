package lb

import (
	"errors"
	"github.com/ktrubilo9/edge-proxy/internal/config"
	"github.com/ktrubilo9/edge-proxy/internal/logger"
	"github.com/ktrubilo9/edge-proxy/internal/metrics"
)

type LoadBalancer interface {
	// Next selects the next backend to rooute traffic to.
	// Returns the selected backend or an error if no backend is available.
	Next([]*config.BackendConfig) (*config.BackendConfig, error)
}

func GetLoadBalancer(strategy string, m *metrics.Metrics) LoadBalancer {
	switch strategy {
	case "least-connections":
		return NewLeastConnections(m)
	case "adaptive":
		return NewAdaptiveLB(m)
	default:
		logger.Warn("Unknown load balancing strategy, defaulting to least-connections", map[string]interface{}{
			"strategy": strategy,
		})
		return NewLeastConnections(m)
	}
}

var ErrNoAvailableBackend = errors.New("no available backend")
