package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"portfolio/internal/llm"
	"portfolio/internal/market"
	"portfolio/internal/store"
)

type Indexer struct {
	db          *store.DB
	llm         *llm.Client
	market      *market.Client
	embeddings  bool
	mu          sync.Mutex
	lastSig     string
	lastTrigger time.Time

	indexMu     sync.Mutex
	index       *corpusIndex
	indexCount  int64
	indexLatest int64
}

// corpusIndex is the tokenized chunk set kept in memory. Scoring from a cached index
// turns a chat retrieval from tens of milliseconds into microseconds.
type corpusIndex struct {
	chunks      []store.RagChunk
	tokens      [][]string
	frequencies []map[string]int
	vectors     [][]float32
	idf         map[string]float64
	averageLen  float64
}

// ragRetentionDays keeps the lexical corpus bounded. The legacy database held months
// of chunks plus their embeddings, which is most of its size.
const ragRetentionDays = 90

func New(db *store.DB, llmClient *llm.Client, marketClient *market.Client, embeddings bool) *Indexer {
	return &Indexer{db: db, llm: llmClient, market: marketClient, embeddings: embeddings}
}

type sourceItem struct {
	source  string
	ticker  string
	userID  string
	date    string
	content string
}

// Reindex rebuilds the lexical index. Embeddings are optional and off by default:
// BM25 costs nothing while semantic search only helps when a paid embedding model is
// enabled explicitly.
func (x *Indexer) Reindex(ctx context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	signature, err := x.signature()
	if err != nil {
		return err
	}
	if signature == x.lastSig {
		return nil
	}

	// Prune first: a legacy database holds months of chunks and their embeddings, and
	// loading them only to delete them costs time and memory.
	if err := x.db.PruneRagChunks(ragRetentionDays); err != nil {
		return err
	}
	items, err := x.collect()
	if err != nil {
		return err
	}
	existing, err := x.db.RagChunkHashes()
	if err != nil {
		return err
	}

	type pending struct {
		item  sourceItem
		chunk string
		hash  string
	}
	var toEmbed []pending
	current := make(map[string]bool, len(items))
	for _, item := range items {
		for _, chunk := range chunkText(item.content, 700, 80) {
			hash := store.HashString(chunk)
			current[hash] = true
			if existing[hash] {
				continue
			}
			toEmbed = append(toEmbed, pending{item: item, chunk: chunk, hash: hash})
		}
	}

	var stale []string
	for hash := range existing {
		if !current[hash] {
			stale = append(stale, hash)
		}
	}
	if err := x.db.DeleteRagChunks(stale); err != nil {
		return err
	}

	for _, entry := range toEmbed {
		chunk := store.RagChunk{
			ID:      entry.hash,
			Source:  entry.item.source,
			Ticker:  entry.item.ticker,
			UserID:  entry.item.userID,
			Date:    entry.item.date,
			Content: entry.chunk,
		}
		if x.embeddings && x.llm != nil && x.llm.Configured() {
			if embeddings, err := x.llm.Embed(ctx, []string{entry.chunk}); err == nil && len(embeddings) == 1 {
				chunk.Embedding = embeddings[0]
			}
		}
		if err := x.db.UpsertRagChunk(chunk); err != nil {
			return err
		}
	}

	x.lastSig = signature
	if len(toEmbed) > 0 {
		log.Printf("rag: indexed %d chunks (%d removed)", len(toEmbed), len(stale))
	}
	return nil
}

// Trigger starts a background reindex at most every five minutes.
func (x *Indexer) Trigger() {
	x.mu.Lock()
	if time.Since(x.lastTrigger) < 5*time.Minute {
		x.mu.Unlock()
		return
	}
	x.lastTrigger = time.Now()
	x.mu.Unlock()
	go func() {
		if err := x.Reindex(context.Background()); err != nil {
			log.Printf("rag: reindex failed: %v", err)
		}
	}()
}

func (x *Indexer) signature() (string, error) {
	parts := make([]string, 0, 8)
	appendPart := func(query string, args ...any) {
		var count int64
		var latest string
		if err := x.db.QueryRow(query, args...).Scan(&count, &latest); err != nil {
			parts = append(parts, "error")
			return
		}
		parts = append(parts, strconv.FormatInt(count, 10)+":"+latest)
	}
	cutoff := strconv.FormatInt(time.Now().AddDate(0, 0, -90).Unix(), 10)
	appendPart(`SELECT COUNT(*), COALESCE(MAX(published_at),'') FROM news WHERE published_at >= ?`, cutoff)
	appendPart(`SELECT COUNT(*), COALESCE(MAX(date),'') FROM daily_sentiment`)
	appendPart(`SELECT COUNT(*), COALESCE(MAX(date),'') FROM running_summary`)
	appendPart(`SELECT COUNT(*), COALESCE(MAX(ticker),'') FROM asset_details`)
	appendPart(`SELECT COUNT(*), COALESCE(MAX(created_at),0) FROM web_searches`)
	appendPart(`SELECT COUNT(*), COALESCE(MAX(updated_at),0) FROM situation_reports WHERE status = 'completed'`)
	return store.HashString(parts...), nil
}

func (x *Indexer) collect() ([]sourceItem, error) {
	var items []sourceItem
	cutoff := strconv.FormatInt(time.Now().AddDate(0, 0, -90).Unix(), 10)
	if rows, err := x.db.Query(`SELECT COALESCE(ticker,''), COALESCE(published_at,''), title,
		COALESCE(summary,''), COALESCE(text,'') FROM news WHERE published_at >= ?`, cutoff); err == nil {
		for rows.Next() {
			var ticker, published, title, summary, text string
			if rows.Scan(&ticker, &published, &title, &summary, &text) == nil {
				items = append(items, sourceItem{source: "news", ticker: ticker, date: published,
					content: strings.TrimSpace(title + ". " + summary + " " + text)})
			}
		}
		rows.Close()
	}
	if rows, err := x.db.Query(`SELECT ticker, date, summary FROM daily_sentiment`); err == nil {
		for rows.Next() {
			var ticker, date, summary string
			if rows.Scan(&ticker, &date, &summary) == nil && summary != "" {
				items = append(items, sourceItem{source: "daily_sentiment", ticker: ticker, date: date, content: summary})
			}
		}
		rows.Close()
	}
	if rows, err := x.db.Query(`SELECT user_id, date, summary FROM portfolio_daily_sentiment`); err == nil {
		for rows.Next() {
			var userID, date, summary string
			if rows.Scan(&userID, &date, &summary) == nil && summary != "" {
				items = append(items, sourceItem{source: "portfolio_daily_sentiment", userID: userID, date: date, content: summary})
			}
		}
		rows.Close()
	}
	if rows, err := x.db.Query(`SELECT user_id, date, summary FROM running_summary`); err == nil {
		for rows.Next() {
			var userID, date, summary string
			if rows.Scan(&userID, &date, &summary) == nil && summary != "" {
				items = append(items, sourceItem{source: "running_summary", userID: userID, date: date, content: summary})
			}
		}
		rows.Close()
	}
	if rows, err := x.db.Query(`SELECT ticker, COALESCE(market_cap,''), COALESCE(country,''), COALESCE(sector,''),
		COALESCE(eps,''), COALESCE(pe_ratio,''), COALESCE(dividend_yield,''), COALESCE(revenue,''),
		COALESCE(net_income,''), COALESCE(profit_margin,'') FROM asset_details`); err == nil {
		for rows.Next() {
			var ticker, marketCap, country, sector, eps, pe, dividend, revenue, netIncome, margin string
			if rows.Scan(&ticker, &marketCap, &country, &sector, &eps, &pe, &dividend, &revenue, &netIncome, &margin) == nil && ticker != "" {
				content := fmt.Sprintf("Fundamentals for %s: market cap %s, country %s, sector %s, EPS %s, P/E %s, dividend yield %s, revenue %s, net income %s, profit margin %s",
					ticker, marketCap, country, sector, eps, pe, dividend, revenue, netIncome, margin)
				items = append(items, sourceItem{source: "asset_details", ticker: ticker, content: content})
			}
		}
		rows.Close()
	}
	if rows, err := x.db.Query(`SELECT user_id, query, results, created_at FROM web_searches`); err == nil {
		for rows.Next() {
			var userID, query, payload string
			var createdAt int64
			if rows.Scan(&userID, &query, &payload, &createdAt) == nil && payload != "" {
				var results []store.SearchResult
				if json.Unmarshal([]byte(payload), &results) == nil && len(results) > 0 {
					items = append(items, sourceItem{source: "web_search", userID: userID,
						date: time.Unix(createdAt, 0).Format("2006-01-02"), content: fmt.Sprintf("Web search for %q:\n%s", query, formatResults(results))})
				}
			}
		}
		rows.Close()
	}
	if rows, err := x.db.Query(`SELECT user_id, date, COALESCE(summary,''), COALESCE(content,'')
		FROM situation_reports WHERE status = 'completed'`); err == nil {
		for rows.Next() {
			var userID, date, summary, content string
			if rows.Scan(&userID, &date, &summary, &content) == nil && content != "" {
				items = append(items, sourceItem{source: "situation_report", userID: userID, date: date, content: content})
			}
		}
		rows.Close()
	}
	return items, nil
}

func formatResults(results []store.SearchResult) string {
	var sb strings.Builder
	for _, result := range results {
		sb.WriteString("- " + llm.Truncate(result.Title, 120))
		if result.Snippet != "" {
			sb.WriteString(": " + llm.Truncate(result.Snippet, 300))
		}
		if result.Text != "" {
			sb.WriteString("\n  " + llm.Truncate(result.Text, 600))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func chunkText(text string, size int, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= size {
		return []string{text}
	}
	var chunks []string
	for start := 0; start < len(text); {
		end := min(start+size, len(text))
		chunks = append(chunks, strings.TrimSpace(text[start:end]))
		if end == len(text) {
			break
		}
		start = end - overlap
	}
	return chunks
}

type Result struct {
	Chunk store.RagChunk
	Score float64
}

// Search ranks chunks with BM25 over the cached corpus. Chunk ownership and ticker
// filters are applied at query time so the cache is shared by every user.
func (x *Indexer) Search(userID string, tickers []string, query string, limit int) ([]Result, error) {
	index, err := x.corpus()
	if err != nil {
		return nil, err
	}
	if index == nil {
		return nil, nil
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	tickerSet := map[string]bool{}
	for _, ticker := range tickers {
		tickerSet[ticker] = true
	}

	// Vectors are only loaded and only paid for when embeddings are enabled; the
	// default path is keyword only and has no model cost.
	var queryVector []float32
	if x.embeddings && len(index.vectors) > 0 && x.llm != nil && x.llm.Configured() {
		if vectors, embedErr := x.llm.Embed(context.Background(), []string{query}); embedErr == nil && len(vectors) == 1 {
			queryVector = vectors[0]
		}
	}

	const k1, b = 1.5, 0.75
	results := make([]Result, 0, 32)
	for i, chunk := range index.chunks {
		if chunk.UserID != "" && chunk.UserID != userID {
			continue
		}
		if len(tickerSet) > 0 && chunk.Ticker != "" && !tickerSet[chunk.Ticker] {
			continue
		}
		tokens := index.tokens[i]
		frequencies := index.frequencies[i]
		score := 0.0
		for _, term := range terms {
			frequency := float64(frequencies[term])
			if frequency == 0 {
				continue
			}
			idf := index.idf[term]
			score += idf * frequency * (k1 + 1) / (frequency + k1*(1-b+b*float64(len(tokens))/index.averageLen))
		}
		if queryVector != nil {
			if similarity := cosineSimilarity(queryVector, index.vectors[i]); similarity > 0.2 {
				score += float64(similarity)
			}
		}
		if score > 0 {
			results = append(results, Result{Chunk: chunk, Score: score})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (x *Indexer) corpus() (*corpusIndex, error) {
	count, latest, err := x.db.RagVersion()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	x.indexMu.Lock()
	defer x.indexMu.Unlock()
	if x.index != nil && x.indexCount == count && x.indexLatest == latest {
		return x.index, nil
	}

	chunks, err := x.db.AllRagChunks(x.embeddings)
	if err != nil {
		return nil, err
	}
	index := &corpusIndex{
		chunks:      chunks,
		tokens:      make([][]string, len(chunks)),
		frequencies: make([]map[string]int, len(chunks)),
		idf:         map[string]float64{},
	}
	if x.embeddings {
		index.vectors = make([][]float32, len(chunks))
		for i, chunk := range chunks {
			index.vectors[i] = chunk.Embedding
		}
	}
	documentFrequency := map[string]int{}
	totalLength := 0
	for i, chunk := range chunks {
		tokens := tokenize(chunk.Content)
		index.tokens[i] = tokens
		totalLength += len(tokens)
		frequencies := make(map[string]int, len(tokens))
		for _, token := range tokens {
			frequencies[token]++
		}
		index.frequencies[i] = frequencies
		for token := range frequencies {
			documentFrequency[token]++
		}
	}
	index.averageLen = float64(totalLength) / float64(len(chunks))
	for term, frequency := range documentFrequency {
		index.idf[term] = math.Log(1 + (float64(len(chunks))-float64(frequency)+0.5)/(float64(frequency)+0.5))
	}
	x.index = index
	x.indexCount = count
	x.indexLatest = latest
	return index, nil
}

func tokenize(value string) []string {
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(words))
	for _, word := range words {
		if len(word) < 2 || stopWords[word] {
			continue
		}
		tokens = append(tokens, word)
	}
	return tokens
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true, "not": true, "you": true,
	"with": true, "this": true, "that": true, "from": true, "have": true, "was": true, "were": true,
	"has": true, "had": true, "will": true, "would": true, "can": true, "could": true, "its": true,
	"their": true, "they": true, "our": true, "your": true, "about": true, "into": true, "over": true,
}

func cosineSimilarity(a []float32, b []float32) float32 {
	length := min(len(a), len(b))
	var dot, normA, normB float64
	for i := 0; i < length; i++ {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
