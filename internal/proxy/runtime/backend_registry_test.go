package runtime

import (
	"edge-proxy/internal/config"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBackendRegistryReconcileAddsNewBackends(t *testing.T) {
	registry := NewBackendRegistry()

	registry.Reconcile([]*config.BackendConfig{
		{Id: "backend-1", URL: "http://backend-1"},
	})

	status, ok := registry.Get("backend-1")
	if !ok || status == nil {
		t.Fatal("expected reconciled backend status")
	}
}

func TestBackendRegistryReconcilePreservesExistingStatus(t *testing.T) {
	registry := NewBackendRegistry()
	backends := []*config.BackendConfig{{Id: "backend-1", URL: "http://backend-1"}}
	registry.Reconcile(backends)

	original, ok := registry.Get("backend-1")
	if !ok {
		t.Fatal("expected initial backend status")
	}

	thresholds := config.HealthThresholdConfig{Healthy: 1, Unhealthy: 5}
	failure := errors.New("temporary failure")
	for i := 0; i < 3; i++ {
		original.ApplyProbeResult(false, failure, thresholds, time.Now())
	}

	registry.Reconcile(backends)

	current, ok := registry.Get("backend-1")
	if !ok {
		t.Fatal("expected preserved backend status")
	}
	if current != original {
		t.Fatal("reconcile replaced the existing backend status")
	}

	snap := current.Snapshot()
	if snap.ConsecutiveFailures != 3 {
		t.Fatalf("consecutive failures = %d, want 3", snap.ConsecutiveFailures)
	}
	if got := current.GetLastError(); got != "temporary failure" {
		t.Fatalf("last error = %q, want %q", got, "temporary failure")
	}
}

func TestBackendRegistryReconcileRemovesMissingBackends(t *testing.T) {
	registry := NewBackendRegistry()
	registry.Reconcile([]*config.BackendConfig{
		{Id: "backend-1", URL: "http://backend-1"},
		{Id: "backend-2", URL: "http://backend-2"},
	})

	registry.Reconcile([]*config.BackendConfig{
		{Id: "backend-2", URL: "http://backend-2"},
	})

	if _, ok := registry.Get("backend-1"); ok {
		t.Fatal("removed backend status is still registered")
	}
	if _, ok := registry.Get("backend-2"); !ok {
		t.Fatal("remaining backend status was removed")
	}
}

func TestBackendRegistryReconcileIgnoresNilBackends(t *testing.T) {
	registry := NewBackendRegistry()

	registry.Reconcile([]*config.BackendConfig{
		nil,
		{Id: "backend-1", URL: "http://backend-1"},
	})

	if _, ok := registry.Get("backend-1"); !ok {
		t.Fatal("valid backend was not registered")
	}
}

func TestBackendRegistrySupportsConcurrentReadsAndReconcile(t *testing.T) {
	registry := NewBackendRegistry()
	registry.Reconcile([]*config.BackendConfig{{Id: "backend-1", URL: "http://backend-1"}})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = registry.Get("backend-1")
			}
		}()
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.Reconcile([]*config.BackendConfig{
				{Id: "backend-1", URL: "http://backend-1"},
				{Id: "backend-2", URL: "http://backend-2"},
			})
		}()
	}

	wg.Wait()

	if _, ok := registry.Get("backend-1"); !ok {
		t.Fatal("existing backend status disappeared")
	}
	if _, ok := registry.Get("backend-2"); !ok {
		t.Fatal("new backend status was not registered")
	}
}
