package health

import (
	"context"
	"time"
)

type Scheduler struct {
	interval time.Duration
}

func NewScheduler(interval time.Duration) *Scheduler {
	return &Scheduler{
		interval: interval,
	}
}

func (s *Scheduler) Wait(ctx context.Context) error {
	timer := time.NewTimer(s.interval)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
