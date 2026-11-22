package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"portfolio/internal/bills"
	"portfolio/internal/llm"
	"portfolio/internal/market"
	"portfolio/internal/prices"
	"portfolio/internal/rag"
	"portfolio/internal/store"
)

type Service struct {
	llm    *llm.Client
	db     *store.DB
	market *market.Client
	prices *prices.Store
	rag    *rag.Indexer
}

func New(llmClient *llm.Client, db *store.DB, marketClient *market.Client, priceStore *prices.Store, indexer *rag.Indexer) *Service {
	return &Service{llm: llmClient, db: db, market: marketClient, prices: priceStore, rag: indexer}
}

type Emitter func(event map[string]any)

const (
	maxToolRounds  = 4
	maxWebSearches = 3
	historyKeep    = 10
	contextBudget  = 9000
	maxContextHR   = 6
)

// Run drives one chat turn. Local database tools are offered before web search so the
// model answers portfolio questions without external calls.
func (s *Service) Run(ctx context.Context, userID string, conversationID string, question string, emit Emitter) (string, []store.SearchResult) {
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "system", Content: s.context(userID, question)},
	}
	history, err := s.db.CompactChatHistory(conversationID, historyKeep)
	if err == nil {
		for _, message := range history {
			if message.Role == "user" || (message.Role == "assistant" && message.Status == "done") {
				messages = append(messages, llm.Message{Role: message.Role, Content: llm.Truncate(message.Content, 2000)})
			}
		}
	}
	messages = append(messages, llm.Message{Role: "user", Content: question})

	tools := localTools()
	var sources []store.SearchResult
	webSearches := 0
	finalAnswer := ""

	for round := 0; round < maxToolRounds; round++ {
		completion, err := s.llm.Chat(ctx, llm.KindChat, userID, messages, tools, func(delta string) {
			emit(map[string]any{"delta": delta})
		})
		if err != nil {
			log.Printf("chat: model error: %v", err)
			emit(map[string]any{"error": "Model request failed. Please try again."})
			return "", nil
		}

		if len(completion.ToolCalls) == 0 {
			if queries := extractWebSearchQueries(completion.Content); len(queries) > 0 && webSearches < maxWebSearches {
				for _, query := range queries {
					if webSearches >= maxWebSearches {
						break
					}
					webSearches++
					emit(map[string]any{"status": "Searching the web..."})
					result, found := s.webSearch(userID, query)
					sources = append(sources, found...)
					if len(found) > 0 {
						emit(map[string]any{"search_results": found})
					}
					messages = append(messages, llm.Message{Role: "user", Content: "Web search results for \"" + query + "\":\n" + result})
				}
				continue
			}
			finalAnswer = strings.TrimSpace(stripDSML(completion.Content))
			break
		}

		messages = append(messages, llm.Message{Role: "assistant", ToolCalls: completion.ToolCalls})
		for _, call := range completion.ToolCalls {
			result := s.callTool(userID, call, &webSearches, &sources, emit)
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    result,
			})
		}
	}

	if strings.TrimSpace(finalAnswer) == "" {
		emit(map[string]any{"error": "I could not generate an answer. Please try again."})
		return "", trimSources(sources)
	}
	return finalAnswer, trimSources(sources)
}

func trimSources(sources []store.SearchResult) []store.SearchResult {
	if len(sources) > 8 {
		sources = sources[len(sources)-8:]
	}
	result := make([]store.SearchResult, 0, len(sources))
	for _, source := range sources {
		result = append(result, store.SearchResult{Title: source.Title, URL: source.URL, Snippet: llm.Truncate(source.Snippet, 200)})
	}
	return result
}

func (s *Service) callTool(userID string, call llm.ToolCall, webSearches *int, sources *[]store.SearchResult, emit Emitter) string {
	var args map[string]any
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	text := func(key string) string {
		value, _ := args[key].(string)
		return strings.TrimSpace(value)
	}

	switch call.Function.Name {
	case "get_portfolio":
		return s.portfolioSummary(userID)
	case "get_price_history":
		return s.priceHistory(text("ticker"), intArg(args, "days", 365))
	case "get_news":
		return s.news(text("ticker"), intArg(args, "limit", 5))
	case "get_spending":
		return s.spending(userID, intArg(args, "days", 90))
	case "search_documents":
		return s.searchDocuments(userID, text("query"))
	case "web_search":
		if *webSearches >= maxWebSearches {
			return "Web search budget for this turn is exhausted. Answer with the context you have."
		}
		*webSearches++
		emit(map[string]any{"status": "Searching the web..."})
		result, found := s.webSearch(userID, text("query"))
		*sources = append(*sources, found...)
		if len(found) > 0 {
			emit(map[string]any{"search_results": found})
		}
		return result
	default:
		return "Unknown tool."
	}
}

func intArg(args map[string]any, key string, fallback int) int {
	value, ok := args[key].(float64)
	if !ok || value <= 0 {
		return fallback
	}
	return int(value)
}

func (s *Service) webSearch(userID string, query string) (string, []store.SearchResult) {
	key := strings.ToLower(strings.TrimSpace(query))
	if key == "" {
		return "Empty search query.", nil
	}
	if cached, ok := s.db.CachedWebSearch(key, 7*86400); ok {
		return formatSources(cached), cached
	}
	results, status := s.market.Search(query, 5)
	if len(results) == 0 {
		return "Web search returned no results (" + status + "). Do not search again; answer from context.", nil
	}
	for i := range results {
		if i < 3 {
			results[i].Text = s.market.ArticleText(results[i].URL)
		}
	}
	stores := make([]store.SearchResult, 0, len(results))
	for _, result := range results {
		stores = append(stores, store.SearchResult{Title: result.Title, URL: result.URL, Snippet: result.Snippet, Text: result.Text})
	}
	_ = s.db.SaveWebSearch(key, userID, stores)
	return formatSources(stores), stores
}

func formatSources(results []store.SearchResult) string {
	var sb strings.Builder
	for _, result := range results {
		sb.WriteString("- " + llm.Truncate(result.Title, 120))
		if result.Snippet != "" {
			sb.WriteString(": " + llm.Truncate(result.Snippet, 300))
		}
		if result.Text != "" {
			sb.WriteString("\n  " + llm.Truncate(result.Text, 700))
		}
		if result.URL != "" {
			sb.WriteString("\n  " + result.URL)
		}
		sb.WriteString("\n")
	}
	return llm.Truncate(sb.String(), 5000)
}

func (s *Service) portfolioSummary(userID string) string {
	holdings, err := s.db.GetHoldingsByUser(userID)
	if err != nil || len(holdings) == 0 {
		return "No holdings recorded."
	}
	var tickers []string
	totalCost := 0.0
	var sb strings.Builder
	for _, holding := range holdings {
		tickers = append(tickers, holding.Ticker)
		totalCost += holding.PurchasePrice * holding.Quantity
		fmt.Fprintf(&sb, "- %s (%s): %g shares at %g %s, ETF=%v, TER=%g%%\n",
			holding.Name, holding.Ticker, holding.Quantity, holding.PurchasePrice, holding.Currency, holding.ETF, holding.TER)
	}
	closes, _ := s.prices.LatestCloses(tickers)
	totalValue := 0.0
	sb.WriteString("\nLatest prices:\n")
	for _, ticker := range tickers {
		if closePrice, ok := closes[ticker]; ok {
			sb.WriteString(fmt.Sprintf("- %s: %.4f\n", ticker, closePrice))
		}
	}
	for _, holding := range holdings {
		if closePrice, ok := closes[holding.Ticker]; ok {
			totalValue += closePrice * holding.Quantity
		}
	}
	if totalCost > 0 {
		fmt.Fprintf(&sb, "\nCost basis %.2f, current value %.2f, gain %.2f%%\n",
			totalCost, totalValue, (totalValue-totalCost)/totalCost*100)
	}
	return llm.TrimContext(sb.String(), 3000)
}

func (s *Service) priceHistory(ticker string, days int) string {
	ticker = s.market.ResolveTicker(ticker)
	if ticker == "" {
		return "A ticker is required."
	}
	if days <= 0 || days > 3650 {
		days = 365
	}
	now := time.Now().UTC().Unix()
	series, dates, err := s.prices.SeriesWithDates(ticker, now-int64(days)*86400, now)
	if err != nil || len(series) == 0 {
		return "No stored price history for " + ticker + "."
	}
	first, last := series[0], series[len(series)-1]
	peak, trough := series[0], series[0]
	for _, value := range series {
		peak = max(peak, value)
		trough = min(trough, value)
	}
	return fmt.Sprintf("%s: %d bars from %s to %s. First %.4f, last %.4f, change %.2f%%, peak %.4f, trough %.4f.",
		ticker, len(series), time.Unix(dates[0], 0).UTC().Format("2006-01-02"),
		time.Unix(dates[len(dates)-1], 0).UTC().Format("2006-01-02"),
		first, last, (last-first)/first*100, peak, trough)
}

func (s *Service) news(ticker string, limit int) string {
	ticker = s.market.ResolveTicker(ticker)
	if ticker == "" {
		recent, err := s.db.RecentNews(min(limit, 5), 0)
		if err != nil || len(recent) == 0 {
			return "No stored news."
		}
		var sb strings.Builder
		for _, item := range recent {
			fmt.Fprintf(&sb, "- [%s] %s (%s)\n", item.Ticker, item.Title, item.PublishedAt)
		}
		return sb.String()
	}
	items, err := s.db.NewsForTicker(ticker, limit)
	if err != nil || len(items) == 0 {
		return "No stored news for " + ticker + "."
	}
	var sb strings.Builder
	for _, item := range items {
		fmt.Fprintf(&sb, "- %s (%s, sentiment %.2f)\n", item.Title, item.PublishedAt, item.Sentiment)
		if item.Summary != "" {
			sb.WriteString("  " + llm.Truncate(item.Summary, 300) + "\n")
		}
	}
	return sb.String()
}

func (s *Service) spending(userID string, days int) string {
	expenses, err := bills.GetExpensesByUserID(userID)
	if err != nil {
		return "No spending data."
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	byCategory := map[string]float64{}
	total := 0.0
	for _, expense := range expenses {
		if expense.Date < cutoff {
			continue
		}
		byCategory[expense.Category] += expense.Amount
		total += expense.Amount
	}
	stats, _ := bills.GetBankTransactionStats(userID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Manual expenses in the last %d days: total %.2f across %d categories.\n", days, total, len(byCategory))
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.Slice(categories, func(i, j int) bool { return byCategory[categories[i]] > byCategory[categories[j]] })
	for _, category := range categories {
		fmt.Fprintf(&sb, "- %s: %.2f\n", category, byCategory[category])
	}
	if len(stats) > 0 {
		fmt.Fprintf(&sb, "Bank: in %.2f, out %.2f, net %.2f\n",
			float64Of(stats["total_in"]), float64Of(stats["total_out"]), float64Of(stats["net"]))
	}
	return sb.String()
}

func float64Of(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int64:
		return float64(typed)
	case int:
		return float64(typed)
	}
	return 0
}

func (s *Service) searchDocuments(userID string, query string) string {
	results, err := s.rag.Search(userID, nil, query, maxContextHR)
	if err != nil || len(results) == 0 {
		return "No stored documents matched that query."
	}
	var sb strings.Builder
	for i, result := range results {
		label := result.Chunk.Source
		if result.Chunk.Ticker != "" {
			label += " " + result.Chunk.Ticker
		}
		fmt.Fprintf(&sb, "[%d] (%s, %s): %s\n", i+1, label, result.Chunk.Date, llm.Truncate(result.Chunk.Content, 500))
	}
	return sb.String()
}

// context builds a bounded snapshot. The persona prompt stays first so provider side
// prompt caching can reuse the stable prefix.
func (s *Service) context(userID string, question string) string {
	var sb strings.Builder
	sb.WriteString("USER PORTFOLIO CONTEXT\n")
	sb.WriteString(s.portfolioSummary(userID))
	sb.WriteString("\nSPENDING\n")
	sb.WriteString(llm.Truncate(s.spending(userID, 90), 1500))

	holdings, _ := s.db.GetHoldingsByUser(userID)
	for _, holding := range holdings {
		if sentiment, err := s.db.GetDailySentiment(holding.Ticker, store.Today()); err == nil && sentiment != nil {
			fmt.Fprintf(&sb, "\nSENTIMENT %s (%.2f): %s", holding.Ticker, sentiment.Sentiment,
				llm.Truncate(sentiment.Summary, 500))
		}
	}
	if portfolio, err := s.db.GetPortfolioSentiment(userID, store.Today()); err == nil && portfolio != nil {
		fmt.Fprintf(&sb, "\nPORTFOLIO SUMMARY (%.2f): %s", portfolio.Sentiment, llm.Truncate(portfolio.Summary, 900))
	}
	if summary, err := s.db.LatestRunningSummary(userID, 30); err == nil && summary != nil {
		fmt.Fprintf(&sb, "\nRUNNING SUMMARY: %s", llm.Truncate(summary.Summary, 1200))
	}
	if results, err := s.rag.Search(userID, nil, question, 4); err == nil && len(results) > 0 {
		sb.WriteString("\nRELEVANT STORED DOCUMENTS\n")
		for _, result := range results {
			fmt.Fprintf(&sb, "- (%s %s): %s\n", result.Chunk.Source, result.Chunk.Ticker,
				llm.Truncate(result.Chunk.Content, 400))
		}
	}
	return llm.TrimContext(sb.String(), contextBudget)
}

const systemPrompt = `You are a careful finance assistant for a personal portfolio and expense tracker.

Rules:
- Answer from the supplied portfolio context and the tools. Never invent prices, holdings or expenses.
- Prefer local tools (get_portfolio, get_price_history, get_news, get_spending, search_documents) over web_search. Only search the web when the stored context cannot answer.
- Be concise: short paragraphs, no filler, no repetition of the context.
- Plain text or light markdown. Never emit tool call markup or XML tags.
- State uncertainty plainly and do not give absolute financial advice.`

func localTools() []llm.Tool {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required}
	}
	stringProp := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	numberProp := func(description string) map[string]any {
		return map[string]any{"type": "number", "description": description}
	}
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name: "get_portfolio", Description: "Current holdings, quantities, cost basis and latest prices.",
			Parameters: object(map[string]any{})}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "get_price_history", Description: "Stored price statistics for a ticker over a number of days.",
			Parameters: object(map[string]any{
				"ticker": stringProp("Ticker symbol or ISIN"),
				"days":   numberProp("Lookback in days, default 365"),
			}, "ticker")}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "get_news", Description: "Stored news headlines and summaries for a ticker.",
			Parameters: object(map[string]any{
				"ticker": stringProp("Ticker symbol, empty for latest portfolio news"),
				"limit":  numberProp("Number of items, default 5"),
			})}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "get_spending", Description: "Manual expenses and bank transaction totals for a period.",
			Parameters: object(map[string]any{
				"days": numberProp("Lookback in days, default 90"),
			})}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "search_documents", Description: "Search stored news, summaries, reports and fundamentals.",
			Parameters: object(map[string]any{
				"query": stringProp("Keywords to search"),
			}, "query")}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "web_search", Description: "Search the live web. Use only when local context is insufficient.",
			Parameters: object(map[string]any{
				"query": stringProp("Search query"),
			}, "query")}},
	}
}

var (
	dsmlPattern  = regexp.MustCompile(`(?s)<invoke\s+name="web_search">.*?</invoke>`)
	dsmlMarker   = regexp.MustCompile(`(?s)<｜DSML｜｜invoke>.*`)
	queryPattern = regexp.MustCompile(`(?s)<invoke\s+name="web_search">.*?<parameter\s+name="query">(.*?)</parameter>.*?</invoke>`)
)

func extractWebSearchQueries(content string) []string {
	var queries []string
	for _, match := range queryPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			if query := strings.Join(strings.Fields(match[1]), " "); query != "" {
				queries = append(queries, query)
			}
		}
	}
	return queries
}

func stripDSML(content string) string {
	content = dsmlPattern.ReplaceAllString(content, "")
	content = dsmlMarker.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}
