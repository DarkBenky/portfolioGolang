package chat

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portfolio/internal/market"
	"portfolio/internal/store"
)

func testDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestFormatSourcesPrintsDate(t *testing.T) {
	results := []store.SearchResult{
		{Title: "Tariffs latest", URL: "https://example.com/a", Snippet: "fresh", Text: "body", Date: "2026-11-27T16:38:22Z"},
		{Title: "Old page", URL: "https://example.com/b", Snippet: "old", Text: "body"},
	}
	formatted := formatSources(results)
	if !strings.Contains(formatted, "Tariffs latest (2026-11-27T16:38:22Z)") {
		t.Fatalf("date missing from prompt text:\n%s", formatted)
	}
	if strings.Contains(formatted, "Old page (") {
		t.Fatalf("empty date must not print brackets:\n%s", formatted)
	}
}

func TestWebSearchKeepsProviderTextAndDate(t *testing.T) {
	db := testDB(t)
	service := &Service{db: db, market: market.NewClient()}
	if err := db.SaveWebSearch("cached query", "user-1", []store.SearchResult{{
		Title: "Cached", URL: "https://example.com/c", Snippet: "snippet",
		Text: "provider summary", Date: "2026-11-27T10:00:00Z",
	}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	text, found := service.webSearch("user-1", "cached query")
	if len(found) != 1 || found[0].Text != "provider summary" {
		t.Fatalf("cached results = %+v", found)
	}
	if found[0].Date != "2026-11-27T10:00:00Z" {
		t.Fatalf("date lost in cache round trip: %+v", found[0])
	}
	if !strings.Contains(text, "(2026-11-27T10:00:00Z)") {
		t.Fatalf("date missing from formatted output:\n%s", text)
	}
}

func TestWebSearchCacheExpiresAfterSixHours(t *testing.T) {
	db := testDB(t)
	if err := db.SaveWebSearch("stale query", "user-1", []store.SearchResult{{
		Title: "Stale", URL: "https://example.com/s", Snippet: "old",
	}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	old := time.Now().Add(-7 * time.Hour).Unix()
	if _, err := db.Exec(`UPDATE web_searches SET created_at = ? WHERE query_hash = ?`, old, "stale query"); err != nil {
		t.Fatalf("age row: %v", err)
	}
	if cached, ok := db.CachedWebSearch("stale query", 6*3600); ok {
		t.Fatalf("seven hour old row must not be served, got %+v", cached)
	}
}
