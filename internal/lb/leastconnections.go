package lb

import (
	"github.com/ktrubilo9/edge-proxy/internal/config"
	"github.com/ktrubilo9/edge-proxy/internal/metrics"
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
	var minConnections uint64
	for _, b := range backends {
		if !b.Enabled {
			continue
		}
		bm := lc.metrics.Backends.Get(b.URL)
		if bm == nil {
			continue
		}

		currentConns := atomic.LoadUint64(&bm.ActiveConnections)
		if best == nil || currentConns < minConnections {
			minConnections = currentConns
			best = b
		}
	}

	if best == nil {
		return nil, ErrNoAvailableBackend
	}
	return best, nil
}
