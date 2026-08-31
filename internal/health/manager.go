package health

import (
	"context"
	"edge-proxy/internal/config"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/metrics"
	"errors"
	"net/http"
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
	prober    Prober
	evaluator *Evaluator

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
	defer hm.mu.Unlock()

	if hm.started {
		return ErrHealthManagerRunning
	}

	cfg := hm.runtime.GetHealthConfig()

	if !cfg.Enabled {
		logger.Info("Health manager is disabled", nil)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	scheduler := NewScheduler(
		time.Duration(cfg.Schedule.IntervalMs) * time.Millisecond,
	)

	prober := NewHTTPProber(&http.Client{})
	evaluator := &Evaluator{}

	workers := NewSimpleWorkerPool(
		hm.processJob,
	)

	workers.Start()

	hm.ctx = ctx
	hm.cancel = cancel
	hm.cfg = cfg
	hm.scheduler = scheduler
	hm.prober = prober
	hm.evaluator = evaluator
	hm.workers = workers
	hm.started = true

	logger.Info("Health manager started", map[string]interface{}{
		"interval_ms":         cfg.Schedule.IntervalMs,
		"timeout_ms":          cfg.Probe.TimeoutMs,
		"healthy_threshold":   cfg.Thresholds.Healthy,
		"unhealthy_threshold": cfg.Thresholds.Unhealthy,
		"path":                cfg.Probe.Path,
	})

	go hm.runLoop(ctx, scheduler, workers)

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

	hm.started = false
	hm.cancel = nil
	hm.ctx = nil
	hm.scheduler = nil
	hm.workers = nil
	hm.prober = nil
	hm.evaluator = nil

	hm.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if workers != nil {
		workers.Stop()
	}

	logger.Info("Health manager stopped", nil)
}

// Main health checking loop
func (hm *HealthManager) runLoop(
	ctx context.Context,
	scheduler *Scheduler,
	workers WorkerPool,
) {
	for {
		if err := scheduler.Wait(ctx); err != nil {
			return
		}

		backends := hm.runtime.GetBackends()

		for _, backend := range backends {
			if backend == nil || !backend.Enabled {
				continue
			}

			workers.Submit(HealthCheckJob{
				BackendID: backend.Id,
			})
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

	workers.Submit(HealthCheckJob{
		BackendID: backendID,
	})
}

// processJob processes a single health check job.
func (hm *HealthManager) processJob(job HealthCheckJob) {
	backend := hm.runtime.GetBackend(job.BackendID)
	if backend == nil || !backend.Enabled {
		return
	}

	status, ok := hm.runtime.GetBackendStatus(job.BackendID)
	if !ok {
		return
	}

	hm.mu.RLock()
	cfg := hm.cfg
	prober := hm.prober
	evaluator := hm.evaluator
	hm.mu.RUnlock()

	if !cfg.Enabled || prober == nil || evaluator == nil {
		return
	}

	result := prober.Probe(
		context.Background(),
		backend,
		cfg.Probe,
	)

	if hm.metrics != nil {
		hm.metrics.RecordHealthCheck(
			backend.URL,
			result.Healthy,
		)
	}

	changed := evaluator.Evaluate(
		status,
		result,
		cfg.Thresholds,
	)

	if changed {
		logger.Info("Backend status changed", map[string]interface{}{
			"backend":     backend.URL,
			"active":      status.Active.Load(),
			"healthy":     result.Healthy,
			"error_count": status.ErrorCount.Load(),
		})
	}

	status.LastHealthCheck.Store(time.Now().Unix())
}

func (hm *HealthManager) Reconcile(cfg config.HealthCheckConfig) error {
	hm.mu.Lock()

	if !hm.started {
		hm.cfg = cfg
		hm.mu.Unlock()
		return nil
	}

	oldCancel := hm.cancel
	oldWorkers := hm.workers

	var (
		ctx       context.Context
		cancel    context.CancelFunc
		scheduler *Scheduler
		workers   WorkerPool
		prober    Prober
		evaluator *Evaluator
	)

	if cfg.Enabled {
		ctx, cancel = context.WithCancel(context.Background())

		scheduler = NewScheduler(
			time.Duration(cfg.Schedule.IntervalMs) * time.Millisecond,
		)

		prober = NewHTTPProber(&http.Client{})
		evaluator = &Evaluator{}

		workers = NewSimpleWorkerPool(
			hm.processJob,
		)

		workers.Start()
	}

	hm.cfg = cfg
	hm.ctx = ctx
	hm.cancel = cancel
	hm.scheduler = scheduler
	hm.workers = workers
	hm.prober = prober
	hm.evaluator = evaluator

	hm.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}

	if oldWorkers != nil {
		oldWorkers.Stop()
	}

	if !cfg.Enabled {
		logger.Info("Health manager disabled", nil)
		return nil
	}

	logger.Info("Health manager reconfigured", map[string]interface{}{
		"interval_ms": cfg.Schedule.IntervalMs,
	})

	go hm.runLoop(ctx, scheduler, workers)

	return nil
}
