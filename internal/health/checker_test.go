package health

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
	"errors"
	"testing"
)

func newBackendStatusForTest(active bool) *runtime.BackendStatus {
	status := &runtime.BackendStatus{}
	status.Active.Store(active)
	return status
}

func TestEvaluatorDeactivatesBackendAtUnhealthyThreshold(t *testing.T) {
	evaluator := Evaluator{}
	status := newBackendStatusForTest(true)

	thresholds := config.HealthThresholdConfig{
		Healthy:   1,
		Unhealthy: 2,
	}

	failure := ProbeResult{
		Healthy: false,
		Err:     errors.New("backend unavailable"),
	}

	evaluator.Evaluate(status, failure, thresholds)

	if !status.Active.Load() {
		t.Fatal("backend became inactive before reaching unhealthy threshold")
	}

	if got := status.ErrorCount.Load(); got != 1 {
		t.Fatalf("error count = %d, want 1", got)
	}

	evaluator.Evaluate(status, failure, thresholds)

	if status.Active.Load() {
		t.Fatal("backend stayed active after reaching unhealthy threshold")
	}

	if got := status.ErrorCount.Load(); got != 2 {
		t.Fatalf("error count = %d, want 2", got)
	}

	if got := status.GetLastError(); got == "" {
		t.Fatal("last error was not recorded")
	}
}

func TestEvaluatorReactivatesHealthyBackend(t *testing.T) {
	evaluator := Evaluator{}
	status := newBackendStatusForTest(false)

	status.ErrorCount.Store(3)
	status.SetLastError("backend unavailable")

	thresholds := config.HealthThresholdConfig{
		Healthy:   1,
		Unhealthy: 3,
	}

	success := ProbeResult{
		Healthy: true,
	}

	evaluator.Evaluate(status, success, thresholds)

	if !status.Active.Load() {
		t.Fatal("healthy backend was not reactivated")
	}

	if got := status.ErrorCount.Load(); got != 0 {
		t.Fatalf("error count = %d, want 0", got)
	}

	if got := status.GetLastError(); got != "" {
		t.Fatalf("last error = %q, want empty", got)
	}
}
