package health

import (
	"context"
	"edge-proxy/internal/config"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Prober interface {
	Probe(
		ctx context.Context,
		backend *config.BackendConfig,
		cfg config.HealthProbeConfig,
	) ProbeResult
}

type HTTPProber struct {
	client *http.Client
}

func NewHTTPProber(client *http.Client) *HTTPProber {
	return &HTTPProber{client: client}
}

type ProbeResult struct {
	Healthy    bool
	StatusCode int
	Duration   time.Duration
	Err        error
}

func (pr *HTTPProber) Probe(
	ctx context.Context,
	backend *config.BackendConfig,
	cfg config.HealthProbeConfig,
) ProbeResult {
	url := backend.URL + cfg.Path

	probeCtx, cancel := context.WithTimeout(
		ctx,
		time.Duration(cfg.TimeoutMs)*time.Millisecond,
	)
	defer cancel()

	req, err := http.NewRequestWithContext(
		probeCtx,
		strings.ToUpper(cfg.Method),
		url,
		nil,
	)
	if err != nil {
		return ProbeResult{
			Healthy: false,
			Err:     err,
		}
	}

	start := time.Now()

	resp, err := pr.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return ProbeResult{
			Healthy:  false,
			Duration: duration,
			Err:      err,
		}
	}

	defer resp.Body.Close()

	if !isSuccessCode(cfg.SuccessCodes, int32(resp.StatusCode)) {
		return ProbeResult{
			Healthy:    false,
			StatusCode: resp.StatusCode,
			Duration:   duration,
			Err: fmt.Errorf(
				"health check returned non-success status: %d",
				resp.StatusCode,
			),
		}
	}

	return ProbeResult{
		Healthy:    true,
		StatusCode: resp.StatusCode,
		Duration:   duration,
	}
}

func isSuccessCode(successCodes []int32, statusCode int32) bool {
	for _, code := range successCodes {
		if statusCode == code {
			return true
		}
	}

	return false
}
