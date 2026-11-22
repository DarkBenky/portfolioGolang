package rag

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"portfolio/internal/store"
)

func newTestIndexer(t *testing.T) (*Indexer, *store.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Indexer{db: db}, db
}

func seed(t *testing.T, db *store.DB, chunks ...string) {
	t.Helper()
	for i, content := range chunks {
		err := db.UpsertRagChunk(store.RagChunk{
			ID:      store.HashString(content, string(rune('a'+i))),
			Source:  "news",
			UserID:  "user-1",
			Date:    "2026-01-01",
			Content: content,
		})
		if err != nil {
			t.Fatalf("seed chunk: %v", err)
		}
	}
}

func TestChunkText(t *testing.T) {
	text := strings.Repeat("word ", 300)
	chunks := chunkText(text, 700, 80)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk) > 700 {
			t.Errorf("chunk exceeds size limit: %d", len(chunk))
		}
	}
}

func TestChunkTextEmpty(t *testing.T) {
	if chunks := chunkText("   ", 100, 10); chunks != nil {
		t.Errorf("expected no chunks, got %v", chunks)
	}
}

func TestTokenizeDropsStopWords(t *testing.T) {
	tokens := tokenize("The quick brown fox and the lazy dog")
	joined := strings.Join(tokens, ",")
	if strings.Contains(joined, "the") || strings.Contains(joined, "and") {
		t.Errorf("stop words survived: %v", tokens)
	}
	if !strings.Contains(joined, "quick") || !strings.Contains(joined, "fox") {
		t.Errorf("content words missing: %v", tokens)
	}
}

func TestSearchRanksRelevantChunkFirst(t *testing.T) {
	indexer, db := newTestIndexer(t)
	seed(t, db,
		"Apple launched a new phone with an improved camera and display",
		"Nvidia reported record data center revenue and raised guidance for AI accelerators",
		"The company hired a new office manager for the Berlin branch",
	)
	results, err := indexer.Search("user-1", nil, "nvidia data center revenue", 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if !strings.Contains(results[0].Chunk.Content, "Nvidia") {
		t.Errorf("expected the Nvidia chunk first, got %q", results[0].Chunk.Content)
	}
}

func TestSearchRespectsUserScope(t *testing.T) {
	indexer, db := newTestIndexer(t)
	seed(t, db, "Nvidia reported record data center revenue")
	results, err := indexer.Search("user-2", nil, "nvidia revenue", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("another user's chunks must not leak, got %d", len(results))
	}
}

func TestSearchTickerFilter(t *testing.T) {
	indexer, db := newTestIndexer(t)
	if err := db.UpsertRagChunk(store.RagChunk{ID: "a", Source: "news", UserID: "user-1", Ticker: "AAPL",
		Date: "2026-01-01", Content: "Apple revenue beat expectations"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertRagChunk(store.RagChunk{ID: "b", Source: "news", UserID: "user-1", Ticker: "MSFT",
		Date: "2026-01-01", Content: "Microsoft revenue beat expectations"}); err != nil {
		t.Fatal(err)
	}
	results, err := indexer.Search("user-1", []string{"MSFT"}, "revenue beat", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Chunk.Ticker != "MSFT" {
		t.Errorf("ticker filter failed: %+v", results)
	}
}

func TestReindexIsIdempotent(t *testing.T) {
	indexer, db := newTestIndexer(t)
	if err := db.AddNews(store.News{IDNews: "n1", Title: "Nvidia beats estimates", PublishedAt: "1790000000",
		Summary: "Record data center revenue", Text: "Nvidia reported record revenue.", Ticker: "NVDA"}); err != nil {
		t.Fatal(err)
	}
	if err := indexer.Reindex(context.Background()); err != nil {
		t.Fatalf("first reindex: %v", err)
	}
	if err := indexer.Reindex(context.Background()); err != nil {
		t.Fatalf("second reindex: %v", err)
	}
	results, err := indexer.Search("", nil, "nvidia revenue", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected an indexed chunk")
	}
}

func BenchmarkSearch(b *testing.B) {
	db, err := store.Open(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	text := "Nvidia reported record data center revenue and raised guidance for AI accelerators. "
	for i := 0; i < 2000; i++ {
		db.UpsertRagChunk(store.RagChunk{
			ID: store.HashString(text, string(rune(i))), Source: "news", UserID: "user-1",
			Date: "2026-01-01", Content: text + string(rune(i)),
		})
	}
	indexer := &Indexer{db: db}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		results, err := indexer.Search("user-1", nil, "nvidia revenue guidance", 6)
		if err != nil || len(results) == 0 {
			b.Fatalf("search failed: %v", err)
		}
	}
}
