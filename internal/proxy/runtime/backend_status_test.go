package runtime

import (
	"edge-proxy/internal/config"
	"errors"
	"testing"
	"time"
)

func TestApplyProbeResultDeactivatesBackendAtUnhealthyThreshold(t *testing.T) {
	status := &BackendStatus{}

	thresholds := config.HealthThresholdConfig{
		Healthy:   1,
		Unhealthy: 2,
	}

	status.ApplyProbeResult(true, nil, thresholds, time.Now())
	if !status.IsActive() {
		t.Fatal("backend was not activated by an initial healthy probe")
	}

	failure := errors.New("backend unavailable")
	status.ApplyProbeResult(false, failure, thresholds, time.Now())
	if !status.IsActive() {
		t.Fatal("backend became inactive before reaching the unhealthy threshold")
	}
	if got := status.Snapshot().ConsecutiveFailures; got != 1 {
		t.Fatalf("consecutive failures = %d, want 1", got)
	}

	status.ApplyProbeResult(false, failure, thresholds, time.Now())
	if status.IsActive() {
		t.Fatal("backend stayed active after reaching the unhealthy threshold")
	}
	if got := status.Snapshot().ConsecutiveFailures; got != 2 {
		t.Fatalf("consecutive failures = %d, want 2", got)
	}
	if got := status.GetLastError(); got == "" {
		t.Fatal("last error was not recorded")
	}
}

func TestApplyProbeResultReactivatesHealthyBackend(t *testing.T) {
	status := &BackendStatus{}

	thresholds := config.HealthThresholdConfig{
		Healthy:   1,
		Unhealthy: 3,
	}

	failure := errors.New("backend unavailable")
	for i := 0; i < 3; i++ {
		status.ApplyProbeResult(false, failure, thresholds, time.Now())
	}

	if status.IsActive() {
		t.Fatal("backend did not become inactive after reaching the unhealthy threshold")
	}

	changed := status.ApplyProbeResult(true, nil, thresholds, time.Now())

	if !changed {
		t.Fatal("expected a healthy probe to change backend state after it was inactive")
	}
	if !status.IsActive() {
		t.Fatal("healthy backend was not reactivated")
	}

	snap := status.Snapshot()
	if snap.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive failures = %d, want 0", snap.ConsecutiveFailures)
	}
	if snap.LastError != "" {
		t.Fatalf("last error = %q, want empty", snap.LastError)
	}
}
