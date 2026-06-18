package runtime

import (
	"edge-proxy/internal/config"
	"sync"
)

type BackendRegistry struct {
	mu       sync.RWMutex
	statuses map[string]*BackendStatus
}

func NewBackendRegistry() *BackendRegistry {
	return &BackendRegistry{
		statuses: make(map[string]*BackendStatus),
	}
}

func (br *BackendRegistry) Get(id string) (*BackendStatus, bool) {
	if br == nil {
		return nil, false
	}

	br.mu.RLock()
	defer br.mu.RUnlock()

	status, ok := br.statuses[id]
	return status, ok
}

func (br *BackendRegistry) Reconcile(backends []*config.BackendConfig) map[string]*BackendStatus {
	if br == nil {
		return nil
	}

	br.mu.Lock()
	defer br.mu.Unlock()

	next := make(map[string]*BackendStatus, len(backends))
	for _, backend := range backends {
		if backend == nil {
			continue
		}

		if status, ok := br.statuses[backend.Id]; ok {
			next[backend.Id] = status
			continue
		}

		next[backend.Id] = &BackendStatus{}
	}

	br.statuses = next
	return next
}
