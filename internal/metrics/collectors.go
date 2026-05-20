package metrics

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type ProxyResourceMetrics struct {
	GoroutineCount  uint64
	MemoryPercent   float32
	CPUPercent      float32
	LastCollectTime time.Time
}

type ProxyResourceCollector struct {
	metrics  *Metrics
	stopChan chan struct{}
	interval time.Duration
}

func NewProxyResourceCollector(m *Metrics, interval time.Duration) *ProxyResourceCollector {
	return &ProxyResourceCollector{
		metrics:  m,
		stopChan: make(chan struct{}),
		interval: interval,
	}
}

func (prc *ProxyResourceCollector) Start() {
	go func() {
		ticker := time.NewTicker(prc.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				prc.collect()
			case <-prc.stopChan:
				return
			}
		}
	}()
}

func (prc *ProxyResourceCollector) Stop() {
	close(prc.stopChan)
}

func (prc *ProxyResourceCollector) collect() {
	atomic.StoreUint64(&prc.metrics.Proxy.GoroutineCount, uint64(runtime.NumGoroutine()))
	prc.metrics.Proxy.CPUPercent = cpuPercent()
	prc.metrics.Proxy.MemoryPercent = MemoryPercent()
}

type BackendResourceCollector struct {
	metrics  *Metrics
	stopChan chan struct{}
	interval time.Duration
	timeout  time.Duration
}

func NewBackendResourceCollector(m *Metrics, interval, timeout time.Duration) *BackendResourceCollector {
	return &BackendResourceCollector{
		metrics:  m,
		stopChan: make(chan struct{}),
		interval: interval,
		timeout:  timeout,
	}
}

func (brc *BackendResourceCollector) Start() {
	go func() {
		ticker := time.NewTicker(brc.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				brc.collectAll()
			case <-brc.stopChan:
				return
			}
		}
	}()
}

func (brc *BackendResourceCollector) Stop() {
	close(brc.stopChan)
}

func (brc *BackendResourceCollector) collectAll() {
	brc.metrics.Backends.Range(func(url string, bm *BackendMetrics) bool {
		go func(backendURL string, backendMetrics *BackendMetrics) {
			remote, err := FetchMetrics(backendURL+"/admin/metrics", brc.timeout)
			if err != nil {
				return
			}

			StoreFloat64Bits(
				&bm.CpuPercentBits,
				remote.CpuPercent,
			)

			StoreFloat64Bits(
				&bm.MemoryPercentBits,
				remote.MemoryPercent,
			)
		}(url, bm)
		return true
	})
}

func readCPU() (idle, total uint64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 5 {
		return 0, 0
	}
	for i := 1; i < len(fields) && i <= 8; i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		if i == 4 {
			idle = val
		}
	}
	return
}

func cpuPercent() float32 {
	idle1, total1 := readCPU()
	if total1 == 0 {
		return 0
	}
	// Sample over a slightly longer interval to reduce jitter.
	time.Sleep(500 * time.Millisecond)
	idle2, total2 := readCPU()
	if total2 == total1 {
		return 0
	}

	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)

	if totalDelta == 0 {
		return 0
	}

	usage := (1.0 - idleDelta/totalDelta) * 100
	if usage < 0 {
		return 0
	}
	if usage > 100 {
		return 100
	}
	return float32(usage)
}

func MemoryPercent() float32 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	var total, available, free, buffers, cached uint64

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			available = val
		case "MemFree:":
			free = val
		case "Buffers:":
			buffers = val
		case "Cached:":
			cached = val
		}
	}

	if total == 0 {
		return 0
	}

	// Fall back to older kernel fields when MemAvailable is missing.
	if available == 0 {
		available = free + buffers + cached
	}

	used := total - available
	return float32(float64(used) / float64(total) * 100)
}
