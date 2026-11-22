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
	embed := s.RAG.EmbedStats()
	embed.Running = s.RAG.Backfilling()
	return c.JSON(http.StatusOK, map[string]any{
		"total": stats.Total, "with_embeddings": stats.WithEmbeddings,
		"content_bytes": stats.ContentBytes, "vector_bytes": stats.VectorBytes,
		"oldest_date": stats.OldestDate, "newest_date": stats.NewestDate,
		"sources": stats.Sources, "tickers": stats.Tickers, "embedding": embed,
	})
}

// embedArticles starts a background pass that fills in missing vectors. Stored
// articles and vectors are never removed, so this only ever grows the corpus.
func (s *Server) embedArticles(c echo.Context) error {
	if !s.LLM.Configured() {
		return c.JSON(http.StatusConflict, map[string]string{"error": "Embedding model is not configured"})
	}
	stats := s.RAG.EmbedStats()
	if stats.Pending == 0 {
		return c.JSON(http.StatusOK, map[string]any{"message": "Every article already has an embedding", "embedding": stats})
	}
	started := s.RAG.StartBackfill()
	message := "Embedding started"
	if !started {
		message = "Embedding already running"
	}
	return c.JSON(http.StatusOK, map[string]any{"message": message, "embedding": s.RAG.EmbedStats()})
}
