package lb

import (
	"math"
	"math/rand/v2"
	"sync/atomic"

	"edge-proxy/internal/config"
	"edge-proxy/internal/metrics"
)

// AdaptiveLB implements a load balancer that scores backends using
// runtime metrics and selects a backend probabilistically using softmax.
type AdaptiveLB struct {
	metrics *metrics.Metrics

	// temperature controls the softness of the softmax selection.
	temperature float64

	// latencyWeight determines how much latency affects the score.
	latencyWeight float64

	// errorWeight scales the impact of backend error rate.
	errorWeight float64

	// failStreakWeight increases penalty for consecutive failures.
	failStreakWeight float64

	// connsWeight controls the impact of active connections.
	connsWeight float64

	// weightFactor adjusts the importance of configured backend weight.
	weightFactor float64

	// minResourceFactor ensures resource-based scaling never drops to zero.
	minResourceFactor float64
}

// NewAdaptiveLB creates a new AdaptiveLB using the provided metrics store.
func NewAdaptiveLB(m *metrics.Metrics) *AdaptiveLB {
	return &AdaptiveLB{
		metrics:           m,
		temperature:       0.2,
		latencyWeight:     2.0,
		errorWeight:       100.0,
		failStreakWeight:  5.0,
		connsWeight:       2.0,
		weightFactor:      0.1,
		minResourceFactor: 0.001,
	}
}

// Next returns the next backend to use from a list of candidates.
// It returns an error if no backends are available.
func (alb *AdaptiveLB) Next(backends []*config.BackendConfig) (*config.BackendConfig, error) {
	if len(backends) == 0 {
		return nil, ErrNoAvailableBackend
	}

	scored := alb.scoreBackends(backends)
	if len(scored) == 0 {
		return backends[rand.IntN(len(backends))], nil
	}

	return alb.softmaxPick(scored), nil
}

func (alb *AdaptiveLB) scoreBackends(backends []*config.BackendConfig) []ScoredBackend {
	scored := make([]ScoredBackend, 0, len(backends))

	for _, b := range backends {
		if !b.Enabled {
			continue
		}

		bm := alb.metrics.Backends.Get(b.URL)
		if bm == nil {
			continue
		}

		score := alb.computeScore(b, bm)
		scored = append(scored, ScoredBackend{
			Backend: b,
			Score:   score,
		})
	}

	return scored
}

func (alb *AdaptiveLB) computeScore(b *config.BackendConfig, bm *metrics.BackendMetrics) float64 {
	latency := alb.readEWMA(&bm.LatencyEWMABits)
	errorRate := alb.readEWMA(&bm.ErrorRateEWMABits)
	conns := float64(atomic.LoadUint64(&bm.ActiveConnections))
	failStreak := float64(atomic.LoadUint32(&bm.ConsecutiveFailures))

	base := 1.0 / (1.0 +
		latency/alb.latencyWeight +
		errorRate*alb.errorWeight +
		failStreak*alb.failStreakWeight +
		conns/alb.connsWeight)

	weightBonus := 1.0 + float64(b.Weight)*alb.weightFactor

	cpuPercent := math.Float64frombits(bm.CpuPercentBits)
	memPercent := math.Float64frombits(bm.MemoryPercentBits)
	cpuFactor := alb.resourceFactor(cpuPercent)
	memFactor := alb.resourceFactor(memPercent)

	score := base * weightBonus * cpuFactor * memFactor

	if score < 0.001 {
		score = 0.001
	}
	return score
}

func (alb *AdaptiveLB) readEWMA(p *uint64) float64 {
	bits := atomic.LoadUint64(p)
	if bits == 0 {
		return 0
	}
	return math.Float64frombits(bits)
}

func (alb *AdaptiveLB) resourceFactor(percent float64) float64 {
	if percent <= 0 {
		return 1.0
	}
	factor := 1.0 - float64(percent)
	if factor < alb.minResourceFactor {
		factor = alb.minResourceFactor
	}
	return factor
}

func (alb *AdaptiveLB) softmaxPick(scored []ScoredBackend) *config.BackendConfig {
	if len(scored) == 1 {
		return scored[0].Backend
	}

	var sum float64
	for i := range scored {
		scored[i].Score = math.Exp((scored[i].Score * 100) / alb.temperature)
		sum += scored[i].Score
	}

	r := rand.Float64() * sum
	for _, s := range scored {
		r -= s.Score
		if r <= 0 {
			return s.Backend
		}
	}
	return scored[len(scored)-1].Backend
}

// ScoredBackend pairs a backend with its computed score.
type ScoredBackend struct {
	Backend *config.BackendConfig
	Score   float64
}
