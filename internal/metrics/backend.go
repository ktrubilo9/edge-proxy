package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type BackendMetrics struct {
	Requests           uint64
	Failures           uint64
	ActiveConnections  uint64
	Timeouts           uint64
	HealthChecks       uint64
	FailedHealthChecks uint64

	LatencyEWMABits   uint64
	ErrorRateEWMABits uint64

	CpuPercentBits    uint64
	MemoryPercentBits uint64

	ConsecutiveFailures uint32
	ConsecutiveSuccess  uint32
	LastFailureTime     int64
}

type RemoteSystemMetrics struct {
	CpuPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	Goroutines    uint64  `json:"goroutines"`
}

type BackendMetricsManager struct {
	backends sync.Map
}

func FetchMetrics(url string, timeout time.Duration) (*RemoteSystemMetrics, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var metrics RemoteSystemMetrics
	if err := json.Unmarshal(body, &metrics); err != nil {
		return nil, err
	}

	return &metrics, nil
}

func (b *BackendMetricsManager) Register(url string) {
	b.backends.LoadOrStore(url, &BackendMetrics{})
}

func (b *BackendMetricsManager) Deregister(url string) {
	b.backends.Delete(url)
}

func (b *BackendMetricsManager) Get(url string) *BackendMetrics {
	if val, ok := b.backends.Load(url); ok {
		return val.(*BackendMetrics)
	}
	return nil
}

func (b *BackendMetricsManager) Range(f func(url string, metrics *BackendMetrics) bool) {
	b.backends.Range(func(key, value interface{}) bool {
		return f(key.(string), value.(*BackendMetrics))
	})
}
