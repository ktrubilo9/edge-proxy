package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ktrubilo9/edge-proxy/internal/config"
)

type LogLevel int

type LogEntry struct {
	Time    time.Time
	Level   LogLevel
	Message string
	Fields  map[string]interface{}
}

type AsyncLogger struct {
	configPtr      atomic.Pointer[config.LoggingConfig]
	level          atomic.Int32
	writer         io.Writer
	errorOut       io.Writer
	entryChan      chan LogEntry
	wg             sync.WaitGroup
	done           chan struct{}
	mu             sync.Mutex
	dropped        uint64
	writeErrorOnce sync.Once
}

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	instance *AsyncLogger
	once     sync.Once
)

func Init(config *config.LoggingConfig) error {
	var initErr error
	once.Do(func() {
		instance = &AsyncLogger{
			writer:    os.Stdout,
			errorOut:  os.Stderr,
			entryChan: make(chan LogEntry, config.BufferSize),
			done:      make(chan struct{}),
		}

		instance.configPtr.Store(config)
		instance.level.Store(int32(parseLevel(config.Level)))

		if !config.Enabled {
			instance.writer = io.Discard
		}

		if config.Async {
			instance.wg.Add(1)
			go instance.process()
		}

		instance.logInternal(LevelInfo, "Logger initialized", map[string]interface{}{
			"level":  config.Level,
			"async":  config.Async,
			"buffer": config.BufferSize,
		})
	})
	return initErr
}

func (l *AsyncLogger) UpdateConfig(cfg *config.LoggingConfig) {
	l.configPtr.Store(cfg)
	l.level.Store(int32(parseLevel(cfg.Level)))
}

func parseLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func levelToString(level LogLevel) string {
	switch level {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func (l *AsyncLogger) shouldLog(lvl LogLevel) bool {
	cfg := l.configPtr.Load()
	if !cfg.Enabled {
		return false
	}
	return lvl >= LogLevel(l.level.Load())
}

func (l *AsyncLogger) formatEntry(entry LogEntry) string {
	var sb strings.Builder
	sb.WriteString(entry.Time.Format("2006-01-02T15:04:05.000Z07:00"))
	sb.WriteString(" [")
	sb.WriteString(levelToString(entry.Level))
	sb.WriteString("] ")
	sb.WriteString(entry.Message)

	sb.WriteString(" |")
	for k, v := range entry.Fields {
		_, _ = fmt.Fprintf(&sb, " %s=%v", k, v)
	}
	return sb.String()
}

func (l *AsyncLogger) logInternal(lvl LogLevel, msg string, fields map[string]interface{}) {
	if !l.shouldLog(lvl) {
		return
	}

	entry := LogEntry{
		Time:    time.Now(),
		Level:   lvl,
		Message: msg,
		Fields:  fields,
	}

	cfg := l.configPtr.Load()

	if cfg.Async {
		select {
		case l.entryChan <- entry:
		default:
			atomic.AddUint64(&l.dropped, 1)
		}
	} else {
		l.writeEntry(entry)
	}
}

func (l *AsyncLogger) process() {
	defer l.wg.Done()

	writer := bufio.NewWriter(l.writer)
	defer l.flush(writer, "final flush")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case entry := <-l.entryChan:
			formatted := l.formatEntry(entry)
			if _, err := writer.WriteString(formatted); err != nil {
				l.reportWriterError("write log entry", err)
				continue
			}
			if err := writer.WriteByte('\n'); err != nil {
				l.reportWriterError("write log entry separator", err)
				continue
			}

			if writer.Buffered() > 4096 {
				l.flush(writer, "flush full buffer")
			}
		case <-ticker.C:
			l.flush(writer, "periodic flush")
			cfg := l.configPtr.Load()
			if cfg.Enabled {
				if dropped := atomic.SwapUint64(&l.dropped, 0); dropped > 0 {
					if _, err := fmt.Fprintf(writer, "%s [WARN] %d log entries dropped (buffer full)\n", time.Now().Format(time.RFC3339Nano), dropped); err != nil {
						l.reportWriterError("write dropped-entry warning", err)
					}
				}
			} else {
				atomic.SwapUint64(&l.dropped, 0)
			}
		case <-l.done:
			return
		}
	}
}

func (l *AsyncLogger) flush(writer *bufio.Writer, operation string) {
	if err := writer.Flush(); err != nil {
		l.reportWriterError(operation, err)
	}
}

func (l *AsyncLogger) reportWriterError(operation string, err error) {
	if err == nil {
		return
	}

	l.writeErrorOnce.Do(func() {
		errorOut := l.errorOut
		if errorOut == nil {
			errorOut = os.Stderr
		}
		_, _ = fmt.Fprintf(
			errorOut,
			"%s [ERROR] logger output failed operation=%q error=%q\n",
			time.Now().Format(time.RFC3339Nano),
			operation,
			err,
		)
	})
}

func (l *AsyncLogger) writeEntry(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	formatted := l.formatEntry(entry)
	if _, err := fmt.Fprintln(l.writer, formatted); err != nil {
		l.reportWriterError("write log entry", err)
	}
}

func LogRequest(start time.Time, clientIP, method, path string, status int, backend string, duration time.Duration, errStr string, fields map[string]interface{}) {
	if instance == nil {
		return
	}
	cfg := instance.configPtr.Load()
	if cfg == nil || !cfg.Enabled {
		return
	}

	if fields == nil {
		fields = make(map[string]interface{})
	}

	fields["client_ip"] = clientIP
	fields["method"] = method
	fields["path"] = path
	fields["status"] = status
	fields["backend"] = backend
	fields["duration"] = fmt.Sprintf("%.2fms", float64(duration.Microseconds())/1000)

	if errStr != "" {
		fields["error"] = errStr
	}

	level := LevelInfo
	if status >= 500 {
		level = LevelError
	} else if status >= 400 {
		level = LevelWarn
	}

	instance.logInternal(level, "HTTP request", fields)
}

func Debug(msg string, fields map[string]interface{}) {
	if instance != nil {
		instance.logInternal(LevelDebug, msg, fields)
	}
}

func Info(msg string, fields map[string]interface{}) {
	if instance != nil {
		instance.logInternal(LevelInfo, msg, fields)
	}
}

func Warn(msg string, fields map[string]interface{}) {
	if instance != nil {
		instance.logInternal(LevelWarn, msg, fields)
	}
}

func Error(msg string, fields map[string]interface{}) {
	if instance != nil {
		instance.logInternal(LevelError, msg, fields)
	}
}

func UpdateConfig(cfg *config.LoggingConfig) {
	if instance != nil {
		instance.UpdateConfig(cfg)
	}
}

func BackendHealth(backend string, healthy bool, err string) {
	if instance == nil {
		return
	}
	cfg := instance.configPtr.Load()
	if cfg == nil || !cfg.Enabled {
		return
	}

	status := "healthy"
	level := LevelInfo
	if !healthy {
		status = "unhealthy"
		level = LevelError
	}

	fields := map[string]interface{}{
		"backend": backend,
		"status":  status,
	}

	if err != "" {
		fields["error"] = err
	}

	instance.logInternal(level, "Health check", fields)
}

func Stop() {
	if instance == nil {
		return
	}

	Info("Logger shutting down", nil)
	close(instance.done)
	instance.wg.Wait()

	if _, err := fmt.Fprintln(os.Stdout, time.Now().Format(time.RFC3339Nano)+" [INFO] Logger stopped"); err != nil {
		instance.reportWriterError("write shutdown confirmation", err)
	}
}
