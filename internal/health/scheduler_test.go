package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSchedulerNextDelayAppliesJitterBounds(t *testing.T) {
	const (
		interval = 10 * time.Second
		jitter   = 2 * time.Second
	)

	minimum := newScheduler(interval, jitter, func(int64) int64 { return 0 })
	if got, want := minimum.nextDelay(), interval-jitter; got != want {
		t.Fatalf("minimum delay = %s, want %s", got, want)
	}

	maximum := newScheduler(interval, jitter, func(limit int64) int64 { return limit - 1 })
	if got, want := maximum.nextDelay(), interval+jitter; got != want {
		t.Fatalf("maximum delay = %s, want %s", got, want)
	}
}

func TestSchedulerNextDelayWithoutJitterUsesInterval(t *testing.T) {
	scheduler := NewScheduler(5*time.Second, 0)
	if got := scheduler.nextDelay(); got != 5*time.Second {
		t.Fatalf("delay = %s, want 5s", got)
	}
}

func TestSchedulerWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewScheduler(time.Hour, 0).Wait(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
}
