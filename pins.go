package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type PinnedSymbol struct {
	Symbol    string `json:"symbol"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	CreatedAt int64  `json:"created_at"`
}

func createPinsTable(sqlDB *sql.DB) error {
	_, err := sqlDB.Exec(`
		CREATE TABLE IF NOT EXISTS pinned_symbols (
			user_id TEXT NOT NULL,
			symbol TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (user_id, symbol)
		)
	`)
	if err != nil {
		return err
	}
	_, err = sqlDB.Exec(`CREATE INDEX IF NOT EXISTS idx_pinned_symbols_user ON pinned_symbols(user_id)`)
	return err
}

func (database *DB) listPinnedSymbols(userID string) ([]PinnedSymbol, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	rows, err := database.Query(`
		SELECT symbol, name, sort_order, created_at
		FROM pinned_symbols
		WHERE user_id = ?
		ORDER BY sort_order ASC, created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]PinnedSymbol, 0)
	for rows.Next() {
		var p PinnedSymbol
		if err := rows.Scan(&p.Symbol, &p.Name, &p.SortOrder, &p.CreatedAt); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (database *DB) addPinnedSymbol(userID, symbol, name string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	_, err := database.Exec(`
		INSERT INTO pinned_symbols (user_id, symbol, name, sort_order, created_at)
		VALUES (?, ?, ?, 0, ?)
		ON CONFLICT (user_id, symbol) DO UPDATE SET name = excluded.name
	`, userID, symbol, name, time.Now().Unix())
	return err
}

func (database *DB) removePinnedSymbol(userID, symbol string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	_, err := database.Exec(`DELETE FROM pinned_symbols WHERE user_id = ? AND symbol = ?`, userID, symbol)
	return err
}

func getPinnedSymbolsHandler(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)

	pins, err := db.listPinnedSymbols(claims.UserID)
	if err != nil {
		log.Printf("Error listing pinned symbols: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load pinned symbols"})
	}
	return c.JSON(http.StatusOK, pins)
}

func addPinnedSymbolHandler(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)

	var req struct {
		Symbol string `json:"symbol"`
		Name   string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" || len(symbol) > 20 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid symbol"})
	}
	name := strings.TrimSpace(req.Name)
	if len(name) > 120 {
		name = name[:120]
	}

	if err := db.addPinnedSymbol(claims.UserID, symbol, name); err != nil {
		log.Printf("Error pinning symbol %s: %v", symbol, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to pin symbol"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Symbol pinned"})
}

func removePinnedSymbolHandler(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)

	symbol := strings.ToUpper(strings.TrimSpace(c.QueryParam("symbol")))
	if symbol == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "symbol is required"})
	}

	if err := db.removePinnedSymbol(claims.UserID, symbol); err != nil {
		log.Printf("Error unpinning symbol %s: %v", symbol, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to unpin symbol"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Symbol unpinned"})
}
