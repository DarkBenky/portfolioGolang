package api

import (
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

func (s *Server) getStorage(c echo.Context) error {
	dir := s.Cfg.DataDir
	if dir == "" {
		dir = "."
	}
	disk := readDisk(dir)
	files := directoryFiles([]string{
		s.Cfg.SQLitePath,
		s.Cfg.SQLitePath + "-wal",
		s.Cfg.SQLitePath + "-shm",
		s.Cfg.PriceDBPath,
	})
	var databaseBytes int64
	for _, file := range files {
		databaseBytes += file.Bytes
	}
	return c.JSON(http.StatusOK, map[string]any{
		"disk":           disk,
		"files":          files,
		"database_bytes": databaseBytes,
		"directory":      filepath.Clean(dir),
	})
}
