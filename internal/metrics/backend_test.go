package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBackendMetricsManager_Register(t *testing.T) {
	bm := &BackendMetricsManager{}

	url := "http://server1:3000"

	bm.Register(url)

	got := bm.Get(url)
	if got == nil {
		t.Fatalf("expected backend metrics to be registered for %q, got nil", url)
	}

	if got.Requests != 0 {
		t.Errorf("Requests = %d, want 0", got.Requests)
	}

	if got.Failures != 0 {
		t.Errorf("Failures = %d, want 0", got.Failures)
	}

	if got.ActiveConnections != 0 {
		t.Errorf("Active connections = %d, want 0", got.ActiveConnections)
	}
}

func TestBackendMetricsManager_RegisterExistingBackendKeepsMetrics(t *testing.T) {
	bm := &BackendMetricsManager{}
	url := "http://server1:3000"
	bm.Register(url)
	bm.Get(url).ActiveConnections = 5
	bm.Register(url)

	if got := bm.Get(url); got != nil && got.ActiveConnections != 5 {
		t.Errorf("Active connections = %d, want 5", got.ActiveConnections)
	}
}

func TestBackendMetricsManager_Deregister(t *testing.T) {
	bm := &BackendMetricsManager{}

	url := "http://server1:3000"

	bm.Register(url)
	bm.Deregister(url)

	got := bm.Get(url)
	if got != nil {
		t.Fatalf("expected backend metrics to be deregistered")
	}
}

func TestBackendMetricsManager_Get(t *testing.T) {
	bm := &BackendMetricsManager{}

	url := "http://server1:3000"

	bm.Register(url)

	metrics := bm.Get(url)
	metrics.ActiveConnections = 4
	metrics.Failures = 2

	got := bm.Get(url)
	if got.ActiveConnections != 4 {
		t.Errorf("Active connections = %d, want 4", got.ActiveConnections)
	}
	if got.Failures != 2 {
		t.Errorf("Failures = %d, want 2", got.Failures)
	}
}

func TestBackendMetricsManager_Range(t *testing.T) {
	bm := &BackendMetricsManager{}

	urls := []string{
		"http://server1:3000",
		"http://server2:3000",
		"http://server3:3000",
	}

	for _, url := range urls {
		bm.Register(url)
	}

	seen := make(map[string]bool)

	bm.Range(func(url string, metrics *BackendMetrics) bool {
		if metrics == nil {
			t.Errorf("metrics for %q = nil", url)
		}
		seen[url] = true
		return true
	})

	for _, url := range urls {
		if !seen[url] {
			t.Errorf("Range did not visit %q", url)
		}
	}
}

func TestBackendMetricsManager_RangeStopsWhenCallbackReturnsFalse(t *testing.T) {
	bm := &BackendMetricsManager{}

	urls := []string{
		"http://server1:3000",
		"http://server2:3000",
		"http://server3:3000",
	}

	for _, url := range urls {
		bm.Register(url)
	}

	count := 0

	bm.Range(func(url string, metrics *BackendMetrics) bool {
		count++
		return false
	})

	if count != 1 {
		t.Errorf("Range visited %d items, want 1", count)
	}
}

func TestFetchMetrics_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`{
			"cpu_percent": 42.5,
			"memory_percent": 61.2,
			"goroutines": 17
		}`))
	}))
	defer server.Close()

	got, err := FetchMetrics(server.URL+"/metrics", 1*time.Second)
	if err != nil {
		t.Fatalf("FetchMetrics returned error: %v", err)
	}

	if got.CpuPercent != 42.5 {
		t.Errorf("CpuPercent = %v, want 42.5", got.CpuPercent)
	}

	if got.MemoryPercent != 61.2 {
		t.Errorf("MemoryPercent = %v, want 61.2", got.MemoryPercent)
	}

	if got.Goroutines != 17 {
		t.Errorf("Goroutines = %v, want 17", got.Goroutines)
	}
}

func TestFetchMetrics_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	got, err := FetchMetrics(server.URL, time.Second)

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if got != nil {
		t.Errorf("metrics = %v, want nil", got)
	}
}

func TestFetchMetrics_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`non-json`))
	}))
	defer server.Close()

	got, err := FetchMetrics(server.URL, time.Second)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got != nil {
		t.Errorf("metrics = %v, want nil", got)
	}
}

func TestFetchMetrics_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cpu_percent":10,"memory_percent":20,"goroutines":3}`))
	}))
	defer server.Close()

	got, err := FetchMetrics(server.URL, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if got != nil {
		t.Errorf("metrics = %v, want nil", got)
	}
}

func TestFetchMetrics_InvalidURL(t *testing.T) {
	got, err := FetchMetrics("://invalid", time.Second)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Errorf("metrics = %v, want nil", got)
	}
}
