package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var serverID string
var cpuWorkers []chan struct{}
var memoryHog [][]byte

func init() {
	hostname, _ := os.Hostname()
	serverID = fmt.Sprintf("%s.%d", hostname, rand.Intn(100))
}

type SystemMetrics struct {
	CpuPercent    float32 `json:"cpu_percent"`
	MemoryPercent float32 `json:"memory_percent"`
}

func readCPU() (idle, total uint64) {
	data, _ := os.ReadFile("/proc/stat")
	fields := strings.Fields(strings.Split(string(data), "\n")[0])
	for i, v := range fields[1:] {
		val, _ := strconv.ParseUint(v, 10, 64)
		total += val
		if i == 3 {
			idle = val
		}
	}
	return
}

func cpuPercent() float32 {
	idle1, total1 := readCPU()
	time.Sleep(200 * time.Millisecond)
	idle2, total2 := readCPU()

	idleTicks := float32(idle2 - idle1)
	totalTicks := float32(total2 - total1)
	return (1.0 - idleTicks/totalTicks) * 100
}

func MemoryPercent() float32 {
	data, _ := os.ReadFile("/proc/meminfo")
	lines := strings.Split(string(data), "\n")
	var total, available float64
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, _ = strconv.ParseFloat(fields[1], 64)
		case "MemAvailable:":
			available, _ = strconv.ParseFloat(fields[1], 64)
		}
	}

	return float32((total - available) / total * 100)
}

func main() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, serverID)
	})

	// Used by the proxy health checker.
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Used by the proxy to collect backend resource metrics.
	http.HandleFunc("/admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		cpu := cpuPercent()
		mem := MemoryPercent()
		metrics := SystemMetrics{
			CpuPercent:    cpu,
			MemoryPercent: mem,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
	})

	http.HandleFunc("/api/admin/cpu", func(w http.ResponseWriter, r *http.Request) {
		countStr := r.URL.Query().Get("workers")
		count, _ := strconv.Atoi(countStr)
		if count <= 0 {
			count = 1
		}

		for i := 0; i < count; i++ {
			stop := make(chan struct{})
			cpuWorkers = append(cpuWorkers, stop)

			go func(stop chan struct{}) {
				for {
					select {
					case <-stop:
						return
					default:
						_ = 1 + 1
					}
				}
			}(stop)
		}

		fmt.Fprintf(w, "Started %d CPU workers\n", count)
	})

	http.HandleFunc("/api/admin/cpu/stop", func(w http.ResponseWriter, r *http.Request) {
		for _, ch := range cpuWorkers {
			close(ch)
		}
		cpuWorkers = nil
		fmt.Fprintln(w, "Stopped all CPU workers")
	})

	http.HandleFunc("/api/admin/memory", func(w http.ResponseWriter, r *http.Request) {
		mbStr := r.URL.Query().Get("mb")
		mb, _ := strconv.Atoi(mbStr)
		if mb <= 0 {
			mb = 100
		}

		block := make([]byte, mb*1024*1024)
		for i := range block {
			block[i] = byte(rand.Intn(256))
		}

		memoryHog = append(memoryHog, block)

		fmt.Fprintf(w, "Allocated %d MB memory\n", mb)
	})

	http.HandleFunc("/api/admin/memory/clear", func(w http.ResponseWriter, r *http.Request) {
		memoryHog = nil
		runtime.GC()
		fmt.Fprintln(w, "Cleared allocated memory")
	})

	http.HandleFunc("/delay", func(w http.ResponseWriter, r *http.Request) {
		secondsStr := r.URL.Query().Get("seconds")
		if secondsStr == "" {
			secondsStr = "1"
		}
		seconds, err := strconv.Atoi(secondsStr)
		if err != nil {
			seconds = 1
		}
		time.Sleep(time.Duration(seconds) * time.Second)
		fmt.Fprintln(w, serverID)
	})

	http.ListenAndServe(":3000", nil)
}
