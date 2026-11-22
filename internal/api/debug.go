package api

import (
	"net/http"
	"runtime"
	"runtime/debug"

	"github.com/labstack/echo/v4"
)

func (s *Server) debugMem(c echo.Context) error {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	limit := debug.SetMemoryLimit(-1)
	if limit < 0 {
		limit = 0
	}
	sqlite := s.DB.Stats()
	return c.JSON(http.StatusOK, map[string]any{
		"heap_alloc_mb": stats.HeapAlloc / 1024 / 1024,
		"heap_inuse_mb": stats.HeapInuse / 1024 / 1024,
		"sys_mb":        stats.Sys / 1024 / 1024,
		"goroutines":    runtime.NumGoroutine(),
		"gc_cycles":     stats.NumGC,
		"gomemlimit_mb": limit / 1024 / 1024,
		"sqlite": map[string]any{
			"open_connections": sqlite.OpenConnections,
			"in_use":           sqlite.InUse,
			"idle":             sqlite.Idle,
			"wait_count":       sqlite.WaitCount,
		},
	})
}
