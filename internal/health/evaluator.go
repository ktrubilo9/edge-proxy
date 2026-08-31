package health

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
)

type Evaluator struct {
}

func (e *Evaluator) Evaluate(
	status *runtime.BackendStatus,
	result ProbeResult,
	cfg config.HealthThresholdConfig,
) (changed bool) {
	oldActive := status.Active.Load()
	if result.Healthy {
		status.Active.Store(true)
		status.ErrorCount.Store(0)
		status.SetLastError("")
	} else {
		status.ErrorCount.Add(1)
		if status.ErrorCount.Load() >= uint32(cfg.Unhealthy) {
			status.Active.Store(false)
			status.SetLastError(result.Err.Error())
		}
	}
	newActive := status.Active.Load()
	return oldActive != newActive
}
