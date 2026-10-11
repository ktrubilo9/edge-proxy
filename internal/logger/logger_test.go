package logger

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ktrubilo9/edge-proxy/internal/config"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestAsyncLoggerReportsWriterFailureOnce(t *testing.T) {
	writeErr := errors.New("forced writer failure")
	var errorOutput bytes.Buffer

	logger := &AsyncLogger{
		writer:    failingWriter{err: writeErr},
		errorOut:  &errorOutput,
		entryChan: make(chan LogEntry),
		done:      make(chan struct{}),
	}
	logger.configPtr.Store(&config.LoggingConfig{
		Enabled: true,
		Level:   "info",
		Async:   true,
	})
	logger.level.Store(int32(LevelInfo))

	logger.wg.Add(1)
	go logger.process()

	logger.entryChan <- LogEntry{
		Level:   LevelInfo,
		Message: "test entry",
	}
	close(logger.done)
	logger.wg.Wait()
	logger.reportWriterError("second failure", errors.New("another writer failure"))

	got := errorOutput.String()
	if !strings.Contains(got, "logger output failed") {
		t.Fatalf("diagnostic output %q does not report the logger failure", got)
	}
	if !strings.Contains(got, writeErr.Error()) {
		t.Fatalf("diagnostic output %q does not contain %q", got, writeErr)
	}
	if count := strings.Count(got, "logger output failed"); count != 1 {
		t.Fatalf("logger failure reported %d times, want 1", count)
	}
}

func TestSynchronousLoggerReportsWriterFailure(t *testing.T) {
	writeErr := errors.New("forced writer failure")
	var errorOutput bytes.Buffer

	logger := &AsyncLogger{
		writer:   failingWriter{err: writeErr},
		errorOut: &errorOutput,
	}

	logger.writeEntry(LogEntry{
		Level:   LevelInfo,
		Message: "test entry",
	})

	got := errorOutput.String()
	if !strings.Contains(got, "logger output failed") {
		t.Fatalf("diagnostic output %q does not report the logger failure", got)
	}
	if !strings.Contains(got, writeErr.Error()) {
		t.Fatalf("diagnostic output %q does not contain %q", got, writeErr)
	}
}
