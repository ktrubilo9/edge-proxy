package health

type WorkerPool interface {
	Start()
	Stop()
	Submit(job HealthCheckJob)
}

type HealthCheckJob struct {
	BackendID string
}

type WorkerFunc func(HealthCheckJob)

// Executes jobs immediately in the caller's goroutine
type SimpleWorkerPool struct {
	worker WorkerFunc
}

func NewSimpleWorkerPool(worker WorkerFunc) *SimpleWorkerPool {
	return &SimpleWorkerPool{
		worker: worker,
	}
}

func (p *SimpleWorkerPool) Start() {}
func (p *SimpleWorkerPool) Stop()  {}
func (p *SimpleWorkerPool) Submit(job HealthCheckJob) {
	p.worker(job)
}
