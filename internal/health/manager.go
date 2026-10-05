package health

import (
	"context"
	"edge-proxy/internal/config"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/metrics"
	"edge-proxy/internal/proxy/runtime"
	"errors"
	"sync"
	"time"
)

var (
	ErrHealthManagerNotRunning = errors.New("health manager is not running")
	ErrHealthManagerRunning    = errors.New("health manager is already running")
)

type HealthManager struct {
	runtime HealthRuntime
	metrics *metrics.Metrics

	mu sync.RWMutex

	cfg       config.HealthCheckConfig
	scheduler *Scheduler
	workers   WorkerPool
	prober    *HTTPProber

	ctx    context.Context
	cancel context.CancelFunc

	started bool
}

func NewHealthManager(runtime HealthRuntime, metrics *metrics.Metrics) *HealthManager {
	return &HealthManager{
		runtime: runtime,
		metrics: metrics,
	}
}

func (hm *HealthManager) Start() error {
	hm.mu.Lock()
	if hm.started {
		hm.mu.Unlock()
		return ErrHealthManagerRunning
	}

	cfg := hm.runtime.GetHealthConfig()
	hm.cfg = cfg
	hm.started = true
	if !cfg.Enabled {
		hm.mu.Unlock()
		logger.Info("Health manager is disabled", nil)
		return nil
	}

	ctx, cancel, scheduler, workers, prober := hm.buildComponents(cfg)
	workers.Start()
	hm.ctx = ctx
	hm.cancel = cancel
	hm.scheduler = scheduler
	hm.prober = prober
	hm.workers = workers
	hm.mu.Unlock()

	logger.Info("Health manager started", map[string]interface{}{
		"interval_ms":         cfg.Schedule.IntervalMs,
		"timeout_ms":          cfg.Probe.TimeoutMs,
		"healthy_threshold":   cfg.Thresholds.Healthy,
		"unhealthy_threshold": cfg.Thresholds.Unhealthy,
		"path":                cfg.Probe.Path,
	})

	go hm.runLoop(ctx, scheduler, workers, cfg)

	return nil
}

func (hm *HealthManager) Stop() {
	hm.mu.Lock()

	if !hm.started {
		hm.mu.Unlock()
		return
	}

	cancel := hm.cancel
	workers := hm.workers
	prober := hm.prober

	hm.started = false
	hm.cancel = nil
	hm.ctx = nil
	hm.scheduler = nil
	hm.workers = nil
	hm.prober = nil

	hm.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if workers != nil {
		workers.Stop()
	}
	if prober != nil {
		prober.CloseIdleConnections()
	}

	logger.Info("Health manager stopped", nil)
}

// Main health checking loop
func (hm *HealthManager) runLoop(
	ctx context.Context,
	scheduler *Scheduler,
	workers WorkerPool,
	cfg config.HealthCheckConfig,
) {
	for {
		backends := hm.runtime.GetBackends()
		now := time.Now()
		for _, backend := range backends {
			if backend == nil || !backend.Enabled {
				continue
			}
			status, ok := hm.runtime.GetBackendStatus(backend.Id)
			if !ok || !shouldScheduleProbe(status.Snapshot(), cfg, now) {
				continue
			}

			if !workers.Submit(HealthCheckJob{
				BackendID: backend.Id,
			}) {
				logger.Debug("Health check was already pending or the queue was full", map[string]interface{}{
					"backend_id": backend.Id,
				})
			}
		}

		if err := scheduler.Wait(ctx); err != nil {
			return
		}
	}
}

// CheckBackend performs an immediate health check for a specific backend.
func (hm *HealthManager) CheckBackend(backendID string) {
	hm.mu.RLock()

	if !hm.started || hm.workers == nil {
		hm.mu.RUnlock()
		return
	}

	workers := hm.workers
	hm.mu.RUnlock()

	_ = workers.Submit(HealthCheckJob{
		BackendID: backendID,
	})
}

// processJob processes a single health check job.
func (hm *HealthManager) processJob(
	ctx context.Context,
	job HealthCheckJob,
	cfg config.HealthCheckConfig,
	prober *HTTPProber,
) {
	backend := hm.runtime.GetBackend(job.BackendID)
	if backend == nil || !backend.Enabled {
		return
	}

	status, ok := hm.runtime.GetBackendStatus(job.BackendID)
	if !ok {
		return
	}

	if !cfg.Enabled {
		return
	}

	result := prober.Probe(
		ctx,
		backend,
		cfg.Probe,
	)

	if hm.metrics != nil {
		hm.metrics.RecordHealthCheck(
			backend.URL,
			result.Healthy,
		)
	}

	changed := status.ApplyProbeResult(result.Healthy, result.Err, cfg.Thresholds, time.Now())

	if changed {
		snap := status.Snapshot()
		logger.Info("Backend status changed", map[string]interface{}{
			"backend":  backend.URL,
			"status":   logStatus(snap.HealthState),
			"last_err": snap.LastError,
		})
	}
}

func (hm *HealthManager) Reconcile(cfg config.HealthCheckConfig) error {
	hm.mu.Lock()

	if !hm.started {
		hm.mu.Unlock()
		return ErrHealthManagerNotRunning
	}

	oldCancel := hm.cancel
	oldWorkers := hm.workers
	oldProber := hm.prober

	var (
		ctx       context.Context
		cancel    context.CancelFunc
		scheduler *Scheduler
		workers   WorkerPool
		prober    *HTTPProber
	)

	if cfg.Enabled {
		ctx, cancel, scheduler, workers, prober = hm.buildComponents(cfg)
		workers.Start()
	}

	hm.cfg = cfg
	hm.ctx = ctx
	hm.cancel = cancel
	hm.scheduler = scheduler
	hm.workers = workers
	hm.prober = prober

	hm.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}

	if oldWorkers != nil {
		oldWorkers.Stop()
	}
	if oldProber != nil {
		oldProber.CloseIdleConnections()
	}

	if !cfg.Enabled {
		logger.Info("Health manager disabled", nil)
		return nil
	}

	logger.Info("Health manager reconfigured", map[string]interface{}{
		"interval_ms": cfg.Schedule.IntervalMs,
	})

	go hm.runLoop(ctx, scheduler, workers, cfg)

	return nil
}

func (hm *HealthManager) buildComponents(cfg config.HealthCheckConfig) (
	context.Context,
	context.CancelFunc,
	*Scheduler,
	WorkerPool,
	*HTTPProber,
) {
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := NewScheduler(
		time.Duration(cfg.Schedule.IntervalMs)*time.Millisecond,
		time.Duration(cfg.Schedule.JitterMs)*time.Millisecond,
	)
	prober := NewConfiguredHTTPProber(cfg.Transport)
	workers := NewBoundedWorkerPool(
		ctx,
		cfg.Concurrency.Workers,
		cfg.Concurrency.QueueSize,
		func(workerCtx context.Context, job HealthCheckJob) {
			hm.processJob(workerCtx, job, cfg, prober)
		},
	)
	return ctx, cancel, scheduler, workers, prober
}

func shouldScheduleProbe(
	status runtime.BackendStatusSnapshot,
	cfg config.HealthCheckConfig,
	now time.Time,
) bool {
	if !cfg.Recovery.Backoff.Enabled || status.HealthState != runtime.HealthUnhealthy {
		return true
	}
	if status.LastHealthCheck.IsZero() {
		return true
	}
	return !now.Before(status.LastHealthCheck.Add(recoveryBackoff(status, cfg)))
}

func recoveryBackoff(
	status runtime.BackendStatusSnapshot,
	cfg config.HealthCheckConfig,
) time.Duration {
	backoff := cfg.Recovery.Backoff
	delay := time.Duration(backoff.InitialMs) * time.Millisecond
	maximum := time.Duration(backoff.MaxMs) * time.Millisecond

	failuresAfterEjection := uint32(0)
	threshold := uint32(cfg.Thresholds.Unhealthy)
	if status.ConsecutiveFailures > threshold {
		failuresAfterEjection = status.ConsecutiveFailures - threshold
	}
	for range failuresAfterEjection {
		next := time.Duration(float64(delay) * backoff.Multiplier)
		if next >= maximum || next <= delay {
			return maximum
		}
		delay = next
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func logStatus(hs runtime.HealthState) string {
	switch hs {
	case runtime.HealthHealthy:
		return "healthy"
	case runtime.HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}
