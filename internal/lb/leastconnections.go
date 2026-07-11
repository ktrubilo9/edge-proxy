package lb

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/metrics"
	"sync/atomic"
)

type LeastConnections struct {
	metrics *metrics.Metrics
}

func NewLeastConnections(metrics *metrics.Metrics) *LeastConnections {
	return &LeastConnections{metrics: metrics}
}

func (lc *LeastConnections) Next(backends []*config.BackendConfig) (*config.BackendConfig, error) {
	if len(backends) == 0 {
		return nil, ErrNoAvailableBackend
	}

	var best *config.BackendConfig
	minConnections := uint64(1<<31 - 1)
	for _, b := range backends {
		if !b.Enabled {
			continue
		}
		bm := lc.metrics.Backends.Get(b.URL)
		if bm == nil {
			continue
		}

		currentConns := atomic.LoadUint64(&bm.ActiveConnections)
		if currentConns < minConnections {
			minConnections = currentConns
			best = b
		}
	}

	return best, nil
}
