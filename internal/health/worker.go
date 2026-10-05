package health

import (
	"context"
	"sync"
)

type WorkerPool interface {
	Start()
	Stop()
	Submit(job HealthCheckJob) bool
}

type HealthCheckJob struct {
	BackendID string
}

type WorkerFunc func(context.Context, HealthCheckJob)

// BoundedWorkerPool limits concurrent probes and coalesces checks for a backend
// while one is queued or running.
type BoundedWorkerPool struct {
	ctx    context.Context
	cancel context.CancelFunc
	worker WorkerFunc
	jobs   chan HealthCheckJob
	count  int

	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	mu      sync.Mutex
	pending map[string]struct{}
	stopped bool
}

func NewBoundedWorkerPool(
	parent context.Context,
	workers int,
	queueSize int,
	worker WorkerFunc,
) *BoundedWorkerPool {
	ctx, cancel := context.WithCancel(parent)
	return &BoundedWorkerPool{
		ctx:     ctx,
		cancel:  cancel,
		worker:  worker,
		jobs:    make(chan HealthCheckJob, queueSize),
		count:   workers,
		pending: make(map[string]struct{}, queueSize),
	}
}

func (p *BoundedWorkerPool) Start() {
	p.startOnce.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.stopped {
			return
		}

		for range p.count {
			p.wg.Add(1)
			go p.run()
		}
	})
}

func (p *BoundedWorkerPool) Stop() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		p.mu.Unlock()
		p.cancel()
		p.wg.Wait()
	})
}

// Submit is deliberately non-blocking. A full queue means the current cycle is
// skipped; the scheduler will try again rather than accumulating stale probes.
func (p *BoundedWorkerPool) Submit(job HealthCheckJob) bool {
	if job.BackendID == "" {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return false
	}
	if _, exists := p.pending[job.BackendID]; exists {
		return false
	}
	p.pending[job.BackendID] = struct{}{}

	select {
	case <-p.ctx.Done():
		delete(p.pending, job.BackendID)
		return false
	case p.jobs <- job:
		return true
	default:
		delete(p.pending, job.BackendID)
		return false
	}
}

func (p *BoundedWorkerPool) run() {
	defer p.wg.Done()
	for {
		if p.ctx.Err() != nil {
			return
		}

		select {
		case <-p.ctx.Done():
			return
		case job := <-p.jobs:
			p.worker(p.ctx, job)
			p.complete(job.BackendID)
		}
	}
}

func (p *BoundedWorkerPool) complete(backendID string) {
	p.mu.Lock()
	delete(p.pending, backendID)
	p.mu.Unlock()
}
