package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func (s *Server) getPins(c echo.Context) error {
	pins, err := s.DB.ListPins(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load pinned symbols"})
	}
	return c.JSON(http.StatusOK, pins)
}

func (s *Server) addPin(c echo.Context) error {
	var request struct {
		Symbol string `json:"symbol"`
		Name   string `json:"name"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	symbol := strings.ToUpper(strings.TrimSpace(request.Symbol))
	if symbol == "" || len(symbol) > 20 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid symbol"})
	}
	name := strings.TrimSpace(request.Name)
	if len(name) > 120 {
		name = name[:120]
	}
	if err := s.DB.AddPin(s.userID(c), symbol, name); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to pin symbol"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Symbol pinned"})
}

func (s *Server) removePin(c echo.Context) error {
	symbol := strings.ToUpper(strings.TrimSpace(c.QueryParam("symbol")))
	if symbol == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "symbol is required"})
	}
	if err := s.DB.RemovePin(s.userID(c), symbol); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to unpin symbol"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Symbol unpinned"})
}
