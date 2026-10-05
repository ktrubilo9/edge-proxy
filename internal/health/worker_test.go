package health

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedWorkerPoolLimitsConcurrencyAndDeduplicates(t *testing.T) {
	release := make(chan struct{})
	started := make(chan string, 3)
	var active atomic.Int32
	var maximum atomic.Int32
	var completed atomic.Int32

	pool := NewBoundedWorkerPool(context.Background(), 2, 4, func(_ context.Context, job HealthCheckJob) {
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		started <- job.BackendID
		<-release
		active.Add(-1)
		completed.Add(1)
	})
	pool.Start()
	t.Cleanup(pool.Stop)

	if !pool.Submit(HealthCheckJob{BackendID: "backend-1"}) ||
		!pool.Submit(HealthCheckJob{BackendID: "backend-2"}) ||
		!pool.Submit(HealthCheckJob{BackendID: "backend-3"}) {
		t.Fatal("expected initial jobs to be accepted")
	}

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start in time")
		}
	}
	if pool.Submit(HealthCheckJob{BackendID: "backend-1"}) {
		t.Fatal("duplicate backend job was accepted")
	}
	if got := maximum.Load(); got != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", got)
	}

	close(release)
	waitForHealthTest(t, time.Second, func() bool {
		return completed.Load() == 3
	})
}

func TestBoundedWorkerPoolRejectsWorkWhenQueueIsFull(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var startedOnce sync.Once
	pool := NewBoundedWorkerPool(context.Background(), 1, 1, func(_ context.Context, _ HealthCheckJob) {
		startedOnce.Do(func() { close(started) })
		<-release
	})
	pool.Start()
	t.Cleanup(pool.Stop)

	if !pool.Submit(HealthCheckJob{BackendID: "running"}) {
		t.Fatal("running job was rejected")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start in time")
	}
	if !pool.Submit(HealthCheckJob{BackendID: "queued"}) {
		t.Fatal("queued job was rejected")
	}
	if pool.Submit(HealthCheckJob{BackendID: "overflow"}) {
		t.Fatal("job was accepted after the queue became full")
	}
	close(release)
}

func TestBoundedWorkerPoolStopCancelsRunningWork(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	pool := NewBoundedWorkerPool(context.Background(), 1, 1, func(ctx context.Context, _ HealthCheckJob) {
		close(started)
		<-ctx.Done()
		close(cancelled)
	})
	pool.Start()
	if !pool.Submit(HealthCheckJob{BackendID: "backend"}) {
		t.Fatal("job was rejected")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start in time")
	}

	pool.Stop()
	pool.Stop()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("running worker was not cancelled")
	}
	if pool.Submit(HealthCheckJob{BackendID: "after-stop"}) {
		t.Fatal("job was accepted after stop")
	}
}

func waitForHealthTest(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
