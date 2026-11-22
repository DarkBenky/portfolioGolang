package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"portfolio/internal/store"
)

// listArticles exposes the stored knowledge base so the embeddings and article text
// stay inspectable instead of being a black box behind chat retrieval.
func (s *Server) listArticles(c echo.Context) error {
	articles, total, err := s.DB.Articles(s.userID(c), store.ArticleFilter{
		Query:  strings.TrimSpace(c.QueryParam("query")),
		Source: strings.TrimSpace(c.QueryParam("source")),
		Ticker: strings.TrimSpace(c.QueryParam("ticker")),
		Limit:  queryInt(c, "limit", 40),
		Offset: queryOffset(c),
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load articles"})
	}
	return c.JSON(http.StatusOK, map[string]any{"articles": articles, "total": total,
		"limit": queryInt(c, "limit", 40), "offset": queryOffset(c)})
}

func (s *Server) articleStats(c echo.Context) error {
	stats, err := s.DB.ArticleStats(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load article statistics"})
	}
	return c.JSON(http.StatusOK, stats)
}
