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

// Trace is the record of one answer: every prompt part the model received and every
// tool it called. The UI renders it next to the message so an answer can be audited.
type Trace struct {
	Model            string      `json:"model"`
	System           string      `json:"system"`
	Context          string      `json:"context"`
	History          int         `json:"history"`
	Tools            []string    `json:"tools"`
	Steps            []TraceStep `json:"steps"`
	Sources          int         `json:"sources"`
	DurationMS       int64       `json:"duration_ms"`
	PromptTokens     int         `json:"prompt_tokens"`
	CompletionTokens int         `json:"completion_tokens"`
	CachedTokens     int         `json:"cached_tokens"`
	CostUSD          float64     `json:"cost_usd"`
}

type TraceStep struct {
	Round            int         `json:"round"`
	Reasoning        string      `json:"reasoning,omitempty"`
	Content          string      `json:"content,omitempty"`
	ToolCalls        []TraceTool `json:"tool_calls,omitempty"`
	PromptTokens     int         `json:"prompt_tokens"`
	CompletionTokens int         `json:"completion_tokens"`
	CachedTokens     int         `json:"cached_tokens"`
	CostUSD          float64     `json:"cost_usd"`
	DurationMS       int64       `json:"duration_ms"`
}

type TraceTool struct {
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Result     string `json:"result"`
	DurationMS int64  `json:"duration_ms"`
}

const (
	maxToolRounds  = 3
	maxWebSearches = 3
	historyKeep    = 10
	contextBudget  = 9000
	maxContextHR   = 6
)

// Run drives one chat turn. Local database tools are offered before web search so the
// model answers portfolio questions without external calls.
func (s *Service) Run(ctx context.Context, userID string, conversationID string, question string, emit Emitter) (string, []store.SearchResult, *Trace) {
	started := time.Now()
	contextText := s.context(userID, question)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "system", Content: contextText},
	}
	tools := localTools()
	trace := &Trace{Model: s.llm.Model(llm.KindChat), System: systemPrompt,
		Context: contextText, Tools: toolNames(tools)}

	history, err := s.db.CompactChatHistory(conversationID, historyKeep)
	if err == nil {
		// The question is saved before the stream starts, so drop the trailing copy.
		if n := len(history); n > 0 && history[n-1].Role == "user" && history[n-1].Content == question {
			history = history[:n-1]
		}
		for _, message := range history {
			if message.Role == "user" || (message.Role == "assistant" && message.Status == "done") {
				messages = append(messages, llm.Message{Role: message.Role, Content: llm.Truncate(message.Content, 2000)})
				trace.History++
			}
		}
	}
	messages = append(messages, llm.Message{Role: "user", Content: question})

	var sources []store.SearchResult
	webSearches := 0
	finalAnswer := ""

	// A pasted link is read before the first model call. Answering "I cannot open
	// links" is never useful, and the text is then in the prompt whether or not the
	// model decides to call a tool.
	if pages := s.prefetchLinks(userID, question, emit); len(pages) > 0 {
		messages = append(messages, llm.Message{Role: "system", Content: pages})
	}

	for round := 0; round < maxToolRounds; round++ {
		step := TraceStep{Round: round + 1}
		stepStart := time.Now()
		// The last round runs without tools: a model that keeps exploring otherwise
		// ends the loop with tool calls and an empty answer.
		roundTools := tools
		if round == maxToolRounds-1 {
			roundTools = nil
		}
		completion, err := s.llm.Chat(ctx, llm.KindChat, userID, messages, roundTools, llm.Handlers{
			OnDelta: func(delta string) { emit(map[string]any{"delta": delta}) },
			OnReasoning: func(delta string) {
				step.Reasoning += delta
				emit(map[string]any{"reasoning": delta})
			},
		})
		if err != nil {
			log.Printf("chat: model error: %v", err)
			trace.DurationMS = time.Since(started).Milliseconds()
			emit(map[string]any{"error": "Model request failed. Please try again."})
			return "", nil, trace
		}
		step.Content = completion.Content
		step.PromptTokens = completion.PromptTokens
		step.CompletionTokens = completion.CompletionTokens
		step.CachedTokens = completion.CachedTokens
		step.CostUSD = completion.CostUSD
		step.DurationMS = time.Since(stepStart).Milliseconds()
		trace.PromptTokens += completion.PromptTokens
		trace.CompletionTokens += completion.CompletionTokens
		trace.CachedTokens += completion.CachedTokens
		trace.CostUSD += completion.CostUSD

		if len(completion.ToolCalls) == 0 {
			if queries := extractWebSearchQueries(completion.Content); len(queries) > 0 && webSearches < maxWebSearches {
				for _, query := range queries {
					if webSearches >= maxWebSearches {
						break
					}
					webSearches++
					emit(map[string]any{"status": "Searching the web..."})
					searchStart := time.Now()
					result, found := s.webSearch(userID, query)
					step.ToolCalls = append(step.ToolCalls, TraceTool{Name: "web_search",
						Arguments: query, Result: llm.Truncate(result, 1200),
						DurationMS: time.Since(searchStart).Milliseconds()})
					sources = append(sources, found...)
					if len(found) > 0 {
						emit(map[string]any{"search_results": found})
					}
					messages = append(messages, llm.Message{Role: "user", Content: "Web search results for \"" + query + "\":\n" + result})
				}
				trace.Steps = append(trace.Steps, step)
				continue
			}
			trace.Steps = append(trace.Steps, step)
			finalAnswer = strings.TrimSpace(stripDSML(completion.Content))
			break
		}

		messages = append(messages, llm.Message{Role: "assistant", ToolCalls: completion.ToolCalls})
		for _, call := range completion.ToolCalls {
			emit(map[string]any{"tool": map[string]any{"name": call.Function.Name, "arguments": call.Function.Arguments}})
			toolStart := time.Now()
			result := s.callTool(userID, call, &webSearches, &sources, emit)
			step.ToolCalls = append(step.ToolCalls, TraceTool{Name: call.Function.Name,
				Arguments: call.Function.Arguments, Result: llm.Truncate(result, 1200),
				DurationMS: time.Since(toolStart).Milliseconds()})
			emit(map[string]any{"tool_result": map[string]any{"name": call.Function.Name,
				"duration_ms": time.Since(toolStart).Milliseconds(), "preview": llm.Truncate(result, 240)}})
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    result,
			})
		}
		trace.Steps = append(trace.Steps, step)
	}

	trace.Sources = len(sources)
	if strings.TrimSpace(finalAnswer) == "" {
		// tencent/hy4-flash sometimes spends the whole completion budget on
		// reasoning and returns no text. Ask once more with thinking disabled so
		// a failed turn still produces an answer.
		emit(map[string]any{"status": "Writing the answer..."})
		retryStart := time.Now()
		retryMessages := append(append([]llm.Message{}, messages...), llm.Message{
			Role: "user", Content: "Answer the question now in plain text. Do not call any tool and do not think out loud.",
		})
		if completion, err := s.llm.Answer(ctx, userID, retryMessages); err == nil {
			answer := strings.TrimSpace(stripDSML(completion.Content))
			trace.Steps = append(trace.Steps, TraceStep{
				Round: len(trace.Steps) + 1, Content: answer,
				PromptTokens: completion.PromptTokens, CompletionTokens: completion.CompletionTokens,
				CachedTokens: completion.CachedTokens, CostUSD: completion.CostUSD,
				DurationMS: time.Since(retryStart).Milliseconds(),
			})
			trace.PromptTokens += completion.PromptTokens
			trace.CompletionTokens += completion.CompletionTokens
			trace.CachedTokens += completion.CachedTokens
			trace.CostUSD += completion.CostUSD
			if answer != "" {
				finalAnswer = answer
				emit(map[string]any{"delta": answer})
			}
		}
	}
	trace.DurationMS = time.Since(started).Milliseconds()
	if strings.TrimSpace(finalAnswer) == "" {
		emit(map[string]any{"error": "I could not generate an answer. Please try again."})
		return "", trimSources(sources), trace
	}
	return finalAnswer, trimSources(sources), trace
}

func toolNames(tools []llm.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Function.Name)
	}
	return names
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
	case "read_url":
		return s.readURL(userID, text("url"), emit, sources)
	case "get_allocation":
		return s.allocation(userID)
	case "get_fundamentals":
		return s.fundamentals(text("ticker"))
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

var urlPattern = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

// readURL downloads one page, keeps it for retrieval and returns its text to the
// model. The page is stored like a web search so the index can use it later.
func (s *Service) readURL(userID string, target string, emit Emitter, sources *[]store.SearchResult) string {
	emit(map[string]any{"status": "Reading the link..."})
	text, title, err := s.market.PageText(target)
	if err != nil {
		return "Could not read " + llm.Truncate(target, 120) + ": " + err.Error() +
			". Answer from the stored context instead and say the page was unreachable."
	}
	result := store.SearchResult{Title: title, URL: target, Snippet: llm.Truncate(text, 300), Text: text}
	*sources = append(*sources, result)
	emit(map[string]any{"search_results": []store.SearchResult{result}})
	s.storePage(userID, target, result)
	return "Page " + target + ":\n" + llm.Truncate(text, 6000)
}

// storePage keeps a read page for later turns and refreshes the index. Without the
// trigger the page would only become searchable at the next scheduled reindex.
func (s *Service) storePage(userID string, key string, result store.SearchResult) {
	_ = s.db.SaveWebSearch(strings.ToLower(key), userID, []store.SearchResult{result})
	if s.rag != nil {
		s.rag.Trigger()
	}
}

// prefetchLinks reads the links contained in the question so the model never has
// to claim it cannot open them. Returns an empty string when there is no link.
func (s *Service) prefetchLinks(userID string, question string, emit Emitter) string {
	matches := urlPattern.FindAllString(question, 2)
	if len(matches) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("The user pasted these links. Their content is below; use it directly and do not say that you cannot open links.\n")
	read := 0
	for _, match := range matches {
		text, title, err := s.market.PageText(strings.TrimRight(match, ".,;:"))
		if err != nil {
			fmt.Fprintf(&sb, "- %s: could not be read (%v).\n", match, err)
			continue
		}
		read++
		emit(map[string]any{"search_results": []store.SearchResult{{
			Title: title, URL: match, Snippet: llm.Truncate(text, 300), Text: text,
		}}})
		s.storePage(userID, match, store.SearchResult{Title: title, URL: match, Snippet: llm.Truncate(text, 300), Text: text})
		fmt.Fprintf(&sb, "- %s (%s):\n%s\n", match, title, llm.Truncate(text, 4000))
	}
	if read == 0 {
		return ""
	}
	return sb.String()
}

func (s *Service) webSearch(userID string, query string) (string, []store.SearchResult) {
	key := strings.ToLower(strings.TrimSpace(query))
	if key == "" {
		return "Empty search query.", nil
	}
	// Six hours keeps repeated questions cheap while a "latest news" ask does not
	// replay results from the previous week.
	if cached, ok := s.db.CachedWebSearch(key, 6*3600); ok {
		return formatSources(cached), cached
	}
	results, status := s.market.Search(query, 5)
	if len(results) == 0 {
		return "Web search returned no results (" + status + "). Do not search again; answer from context.", nil
	}
	for i := range results {
		// A provider that already returned text keeps it.
		if i < 3 && results[i].Text == "" {
			results[i].Text = s.market.ArticleText(results[i].URL)
		}
	}
	stores := make([]store.SearchResult, 0, len(results))
	for _, result := range results {
		stores = append(stores, store.SearchResult{Title: result.Title, URL: result.URL, Snippet: result.Snippet, Text: result.Text, Date: result.Date})
	}
	_ = s.db.SaveWebSearch(key, userID, stores)
	return formatSources(stores), stores
}

func formatSources(results []store.SearchResult) string {
	var sb strings.Builder
	for _, result := range results {
		sb.WriteString("- " + llm.Truncate(result.Title, 120))
		if result.Date != "" {
			sb.WriteString(" (" + llm.Truncate(result.Date, 25) + ")")
		}
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

// allocation gives the weights the snapshot lacks, so the model can attribute a
// move to a sector or a position instead of saying it has no breakdown.
func (s *Service) allocation(userID string) string {
	holdings, err := s.db.GetHoldingsByUser(userID)
	if err != nil || len(holdings) == 0 {
		return "No holdings recorded."
	}
	tickers := make([]string, 0, len(holdings))
	for _, holding := range holdings {
		tickers = append(tickers, holding.Ticker)
	}
	closes, _ := s.prices.LatestCloses(tickers)
	values := make(map[string]float64, len(holdings))
	total := 0.0
	for _, holding := range holdings {
		price := holding.PurchasePrice
		if closePrice, ok := closes[holding.Ticker]; ok && closePrice > 0 {
			price = closePrice
		}
		values[holding.IDHolding] = price * holding.Quantity
		total += values[holding.IDHolding]
	}
	if total <= 0 {
		return "No prices available for the allocation."
	}
	sectors := map[string]float64{}
	regions := map[string]float64{}
	var sb strings.Builder
	sb.WriteString("Position weights, largest first:\n")
	ordered := append([]store.Holding{}, holdings...)
	sort.Slice(ordered, func(i, j int) bool {
		return values[ordered[i].IDHolding] > values[ordered[j].IDHolding]
	})
	for _, holding := range ordered {
		weight := values[holding.IDHolding] / total * 100
		fmt.Fprintf(&sb, "- %s %s: %.1f%% (%.2f)\n", holding.Ticker, holding.Name, weight, values[holding.IDHolding])
		sectorRows, regionRows, breakdownErr := s.db.GetBreakdownForHolding(holding.IDHolding)
		if breakdownErr != nil {
			continue
		}
		for _, sector := range sectorRows {
			sectors[sector.Name] += weight * sector.Percentage / 100
		}
		for _, region := range regionRows {
			regions[region.Name] += weight * region.Percentage / 100
		}
	}
	writeWeights := func(label string, weights map[string]float64) {
		names := make([]string, 0, len(weights))
		for name := range weights {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool { return weights[names[i]] > weights[names[j]] })
		fmt.Fprintf(&sb, "\n%s:\n", label)
		for _, name := range names {
			fmt.Fprintf(&sb, "- %s: %.1f%%\n", name, weights[name])
		}
	}
	if len(sectors) > 0 {
		writeWeights("Sector exposure", sectors)
	}
	if len(regions) > 0 {
		writeWeights("Region exposure", regions)
	}
	return sb.String()
}

// fundamentals exposes the stored asset details, which the snapshot does not carry.
func (s *Service) fundamentals(ticker string) string {
	ticker = s.market.ResolveTicker(ticker)
	if ticker == "" {
		return "A ticker is required."
	}
	detail, err := s.db.LatestAssetDetails(ticker)
	if err != nil || detail == nil {
		return "No stored fundamentals for " + ticker + "."
	}
	return fmt.Sprintf("%s fundamentals as of %s: market cap %s, country %s, sector %s, EPS %s, P/E %s, P/B %s, dividend yield %s, revenue %s, net income %s, profit margin %s.",
		ticker, detail.Date, detail.MarketCap, detail.Country, detail.Sector, detail.Eps,
		detail.PeRatio, detail.PbRatio, detail.DividendYield, detail.Revenue,
		detail.NetIncome, detail.ProfitMargin)
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
- The context already lists every holding with prices and cost basis, so simple portfolio questions need no tool call.
- Pick the tool that matches the question:
  - get_portfolio, get_price_history, get_news, get_spending for recorded facts.
  - get_allocation for weights, sector or region exposure and "which holding is biggest". Do not answer that you lack a breakdown without calling it.
  - get_fundamentals for valuation and company figures such as P/E, EPS or market cap.
  - search_documents for anything said in stored articles, summaries or reports.
  - read_url for any link the user pastes and for a search result that looks relevant. The page text is already in the context, so read it there before calling.
  - web_search for market moves, causes of a drop, company events and anything that happened after the stored articles. If the user asks why something moved and the stored corpus does not explain it, search the web instead of saying the data is missing.
- Never claim that you cannot open a link, and never answer a shared link from its title alone.
- Use at most three tool calls before answering.
- Be concise: short paragraphs, no filler, no repetition of the context, no restating the question.
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
			Name: "get_allocation", Description: "Position weights plus aggregated sector and region exposure. Use it for 'how much am I in tech', 'which holding is biggest' and to attribute a move to a sector.",
			Parameters: object(map[string]any{})}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "get_fundamentals", Description: "Stored fundamentals for a ticker: market cap, country, sector, EPS, P/E, P/B, dividend yield, revenue, net income, profit margin.",
			Parameters: object(map[string]any{
				"ticker": stringProp("Ticker symbol or ISIN"),
			}, "ticker")}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "read_url", Description: "Download one web page and return its text. Use it for any link the user pastes, and for a result that looks relevant.",
			Parameters: object(map[string]any{
				"url": stringProp("Absolute http or https link"),
			}, "url")}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "web_search", Description: "Search the live web for news and market events that are not in the stored corpus, such as why a stock moved today, recent company announcements or macro headlines.",
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
