package metrics

import "sync"

type HTTPMetrics struct {
	RequestSizeTotal  uint64
	ResponseSizeTotal uint64

	StatusCodes struct {
		mu    sync.RWMutex
		items map[int]uint64
	}
	MethodsCount struct {
		mu    sync.RWMutex
		items map[string]uint64
	}
}

func (h *HTTPMetrics) RecordStatusCode(status int) {
	h.StatusCodes.mu.Lock()
	defer h.StatusCodes.mu.Unlock()
	h.StatusCodes.items[status]++
}

func (h *HTTPMetrics) RecordMethod(method string) {
	h.MethodsCount.mu.Lock()
	defer h.MethodsCount.mu.Unlock()
	h.MethodsCount.items[method]++
}
