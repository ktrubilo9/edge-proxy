package health

import (
	"context"
	"math/rand/v2"
	"time"
)

type Scheduler struct {
	interval time.Duration
	jitter   time.Duration
	random   func(int64) int64
}

func NewScheduler(interval, jitter time.Duration) *Scheduler {
	return newScheduler(interval, jitter, rand.Int64N)
}

func newScheduler(interval, jitter time.Duration, random func(int64) int64) *Scheduler {
	return &Scheduler{
		interval: interval,
		jitter:   jitter,
		random:   random,
	}
}

func (s *Scheduler) Wait(ctx context.Context) error {
	timer := time.NewTimer(s.nextDelay())
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) nextDelay() time.Duration {
	if s.jitter <= 0 {
		return s.interval
	}

	span := int64(2*s.jitter) + 1
	offset := time.Duration(s.random(span)) - s.jitter
	return s.interval + offset
}
