package main

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// parseMemorySize accepts values such as 1200MiB, 1.5GiB, 800MB or plain bytes.
func parseMemorySize(value string) (int64, bool) {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	if cleaned == "" {
		return 0, false
	}
	suffixes := []struct {
		suffix string
		factor float64
	}{
		{"gib", 1024 * 1024 * 1024}, {"gb", 1000 * 1000 * 1000},
		{"mib", 1024 * 1024}, {"mb", 1000 * 1000},
		{"kib", 1024}, {"kb", 1000},
	}
	for _, entry := range suffixes {
		if strings.HasSuffix(cleaned, entry.suffix) {
			number := strings.TrimSpace(strings.TrimSuffix(cleaned, entry.suffix))
			parsed, err := strconv.ParseFloat(number, 64)
			if err != nil || parsed <= 0 {
				return 0, false
			}
			return int64(parsed * entry.factor), true
		}
	}
	parsed, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return int64(parsed), true
}

// GOMEMLIMIT from .env would be read too late by the runtime, so it is applied here.
func applyEnvMemoryLimit() {
	raw := strings.TrimSpace(os.Getenv("GOMEMLIMIT"))
	if raw == "" {
		return
	}
	limit, ok := parseMemorySize(raw)
	if !ok {
		log.Printf("GOMEMLIMIT=%q is not a valid size, ignoring it", raw)
		return
	}
	debug.SetMemoryLimit(limit)
	log.Printf("Go heap limit set to %d MB from GOMEMLIMIT", limit/1024/1024)
}

func memorySnapshot() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	limit := debug.SetMemoryLimit(-1)
	if limit < 0 {
		limit = 0
	}

	sqlite := map[string]interface{}{}
	if db != nil {
		s := db.Stats()
		sqlite = map[string]interface{}{
			"open_connections": s.OpenConnections,
			"in_use":           s.InUse,
			"idle":             s.Idle,
			"wait_count":       s.WaitCount,
		}
	}

	return map[string]interface{}{
		"heap_alloc_mb": m.HeapAlloc / 1024 / 1024,
		"heap_inuse_mb": m.HeapInuse / 1024 / 1024,
		"sys_mb":        m.Sys / 1024 / 1024,
		"goroutines":    runtime.NumGoroutine(),
		"gc_cycles":     m.NumGC,
		"gomemlimit_mb": limit / 1024 / 1024,
		"sqlite":        sqlite,
	}
}

func debugMemHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, memorySnapshot())
}

func logMemoryPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		snap := memorySnapshot()
		sqlite, _ := snap["sqlite"].(map[string]interface{})
		log.Printf("mem: heap=%vMB sys=%vMB goroutines=%v gc=%v sqlite_open=%v sqlite_inuse=%v sqlite_wait=%v",
			snap["heap_alloc_mb"], snap["sys_mb"], snap["goroutines"], snap["gc_cycles"],
			sqlite["open_connections"], sqlite["in_use"], sqlite["wait_count"])
	}
}
