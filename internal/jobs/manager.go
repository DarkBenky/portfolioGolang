package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"portfolio/internal/bills"
	"portfolio/internal/llm"
	"portfolio/internal/market"
	"portfolio/internal/prices"
	"portfolio/internal/rag"
	"portfolio/internal/store"
)

type Manager struct {
	Cfg    Settings
	DB     *store.DB
	Prices *prices.Store
	Market *market.Client
	LLM    *llm.Client
	RAG    *rag.Indexer

	fetchSem chan struct{}

	situationMu  sync.Mutex
	situationRun map[string]bool

	cooldownMu sync.Mutex
	cooldowns  map[string]time.Time
}

type Settings struct {
	Retention5mDays      int
	Retention1hDays      int
	Lookback5mDays       int
	SummaryMaxChars      int
	ChatSummaryMaxTokens int
}

func New(db *store.DB, priceStore *prices.Store, marketClient *market.Client, llmClient *llm.Client, indexer *rag.Indexer, settings Settings) *Manager {
	return &Manager{
		Cfg:          settings,
		DB:           db,
		Prices:       priceStore,
		Market:       marketClient,
		LLM:          llmClient,
		RAG:          indexer,
		fetchSem:     make(chan struct{}, 3),
		situationRun: map[string]bool{},
		cooldowns:    map[string]time.Time{},
	}
}

// Start launches the periodic pipelines. Every loop waits one full interval before the
// first run so a restart does not stampede the external APIs.
func (m *Manager) Start() {
	go m.every(10*time.Minute, m.FetchAllPrices)
	go m.every(45*time.Minute, m.FetchAllNews)
	go m.every(24*time.Hour, m.compact)
	go m.every(6*time.Hour, func() {
		if err := m.Prices.Checkpoint(); err != nil {
			log.Printf("jobs: price checkpoint: %v", err)
		}
	})
	go m.every(24*time.Hour, m.UpdateAllSentiments)
	go m.every(30*time.Minute, m.RefreshAllETFs)
	go m.every(1*time.Hour, m.RefreshAssetDetails)
	go m.every(30*time.Minute, func() {
		if err := m.RAG.Reindex(context.Background()); err != nil {
			log.Printf("jobs: rag reindex: %v", err)
		}
	})
	go m.every(30*time.Minute, m.SituationTick)
	go m.startAutoReportLoop()
}

func (m *Manager) every(interval time.Duration, task func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		task()
	}
}

func (m *Manager) acquire() func() {
	m.fetchSem <- struct{}{}
	return func() { <-m.fetchSem }
}

func (m *Manager) FetchAllPrices() {
	tickers, err := m.DB.UniqueTickers()
	if err != nil {
		log.Printf("jobs: tickers: %v", err)
		return
	}
	var wg sync.WaitGroup
	for _, ticker := range tickers {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			release := m.acquire()
			defer release()
			if err := m.FetchPrices(symbol); err != nil {
				log.Printf("jobs: prices %s: %v", symbol, err)
			}
		}(ticker)
		time.Sleep(200 * time.Millisecond)
	}
	wg.Wait()
}

// FetchPrices pulls the most recent intraday bars. The window is clamped to the
// retention period so a long outage cannot fill the disk with junk.
func (m *Manager) FetchPrices(ticker string) error {
	last, err := m.Prices.LastTimestamp(ticker)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	earliest := now.AddDate(0, 0, -m.Cfg.Lookback5mDays)
	start := now.AddDate(0, 0, -7)
	if last > 0 {
		start = time.Unix(last, 0).UTC()
	}
	if start.Before(earliest) || start.After(now) {
		start = earliest
	}
	bars, _, err := m.Market.Chart(ticker, start.Unix(), now.Unix(), "5m")
	if err != nil {
		return err
	}
	converted := toBars(ticker, bars, now)
	if len(converted) == 0 {
		return nil
	}
	return m.Prices.Upsert(converted)
}

// BackfillHistory loads hourly bars for the lookback window. Compaction folds the old
// part into daily bars afterwards.
func (m *Manager) BackfillHistory(ticker string) error {
	release := m.acquire()
	defer release()
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -729)
	bars, _, err := m.Market.Chart(ticker, start.Unix(), now.Unix(), "1h")
	if err != nil {
		return err
	}
	return m.Prices.Upsert(toBars(ticker, bars, now))
}

func (m *Manager) EnsureHistory(ticker string) error {
	hasData, err := m.Prices.HasDataBefore(ticker, time.Now().AddDate(0, 0, -30).Unix())
	if err != nil {
		return err
	}
	if hasData {
		return nil
	}
	return m.BackfillHistory(ticker)
}

func toBars(ticker string, bars []market.ChartBar, now time.Time) []prices.Bar {
	limit := now.Unix()
	result := make([]prices.Bar, 0, len(bars))
	for _, bar := range bars {
		if bar.Timestamp <= 0 || bar.Timestamp > limit || bar.Close == 0 {
			continue
		}
		result = append(result, prices.Bar{
			Ticker: ticker, Date: bar.Timestamp, Open: bar.Open, Close: bar.Close,
			High: bar.High, Low: bar.Low, Volume: bar.Volume,
		})
	}
	return result
}

func (m *Manager) compact() {
	if err := m.Prices.Compact(m.Cfg.Retention5mDays, m.Cfg.Retention1hDays); err != nil {
		log.Printf("jobs: compaction: %v", err)
	}
}

func (m *Manager) FetchAllNews() {
	holdings, err := m.DB.GetAllETFHoldings()
	if err != nil {
		return
	}
	for _, holding := range holdings {
		assets, err := m.DB.GetAssetsForHolding(holding.IDHolding)
		if err != nil || len(assets) == 0 {
			if err := m.FetchNews(holding.Ticker, holding.Ticker, 10); err != nil {
				log.Printf("jobs: news %s: %v", holding.Ticker, err)
			}
			continue
		}
		limit := min(len(assets), 10)
		for _, asset := range assets[:limit] {
			if asset.Ticker == "" {
				continue
			}
			if err := m.FetchNews(asset.Ticker, holding.Ticker, 3); err != nil {
				log.Printf("jobs: composite news %s: %v", asset.Ticker, err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	etfSet := map[string]bool{}
	for _, holding := range holdings {
		etfSet[holding.Ticker] = true
	}
	tickers, err := m.DB.UniqueTickers()
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, ticker := range tickers {
		if etfSet[ticker] {
			continue
		}
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			release := m.acquire()
			defer release()
			if err := m.FetchNews(symbol, symbol, 10); err != nil {
				log.Printf("jobs: news %s: %v", symbol, err)
			}
		}(ticker)
		time.Sleep(250 * time.Millisecond)
	}
	wg.Wait()
}

// FetchNews stores only unseen headlines. Sentiment comes from the local lexicon so
// news ingestion stays free.
func (m *Manager) FetchNews(sourceTicker string, storeAs string, limit int) error {
	items, err := m.Market.News(sourceTicker, limit)
	if err != nil {
		return err
	}
	assets, _ := m.DB.GetAssetsByTicker(storeAs)
	holdings, _ := m.DB.GetHoldingsByTicker(storeAs)
	for _, item := range items {
		text := m.Market.ArticleText(item.URL)
		summary := item.Summary
		if summary == "" {
			summary = llm.Truncate(text, 400)
		}
		exists, err := m.DB.NewsExists(item.Title, summary, llm.Truncate(text, 500))
		if err != nil || exists {
			continue
		}
		title := item.Title
		if storeAs != sourceTicker {
			title = fmt.Sprintf("[via %s] %s", sourceTicker, item.Title)
		}
		news := store.News{
			IDNews:      store.NewID(),
			Title:       title,
			Link:        item.URL,
			PublishedAt: strconv.FormatInt(item.PublishedAt, 10),
			Summary:     summary,
			Text:        text,
			Author:      item.Author,
			Ticker:      storeAs,
			Sentiment:   market.ScoreSentiment(item.Title, summary, text),
		}
		if len(assets) > 0 {
			news.IDAsset = assets[0].IDAsset
		}
		if len(holdings) > 0 {
			news.IDHolding = holdings[0].IDHolding
		}
		if err := m.DB.AddNews(news); err != nil {
			log.Printf("jobs: add news %q: %v", item.Title, err)
		}
	}
	return nil
}

func (m *Manager) RefreshAllETFs() {
	holdings, err := m.DB.GetAllETFHoldings()
	if err != nil {
		return
	}
	for _, holding := range holdings {
		if err := m.RefreshETF(holding); err != nil {
			log.Printf("jobs: etf %s: %v", holding.Ticker, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// RefreshETF replaces the composition of one ETF. The composition tables are small,
// so a full replace is simpler and cheaper than diffing.
func (m *Manager) RefreshETF(holding store.Holding) error {
	release := m.acquire()
	defer release()
	composition, err := m.Market.ETFComposition(holding.Ticker, holding.ISIN, holding.Name)
	if err != nil {
		return err
	}
	if len(composition.Holdings) == 0 && len(composition.Sectors) == 0 {
		return nil
	}
	if composition.ISIN != "" && composition.ISIN != "N/A" && composition.ISIN != holding.ISIN {
		_ = m.DB.UpdateHoldingISIN(holding.IDHolding, composition.ISIN)
	}
	if err := m.DB.DeleteAssetsForHolding(holding.IDHolding); err != nil {
		return err
	}
	if err := m.DB.DeleteBreakdownForHolding(holding.IDHolding); err != nil {
		return err
	}
	for _, item := range composition.Holdings {
		ticker := item.Ticker
		if ticker == "" && item.ISIN != "" {
			if resolved := m.Market.ISINToTicker(item.ISIN); resolved != "" {
				ticker = resolved
			}
		}
		if err := m.DB.AddAsset(store.Asset{
			IDAsset: store.NewID(), Name: item.Name, Ticker: ticker, ISIN: item.ISIN,
			Exchange: item.Exchange, Sector: item.Sector, Region: item.Region, IDHolding: holding.IDHolding,
		}); err != nil {
			return err
		}
	}
	for name, percentage := range composition.Sectors {
		if err := m.DB.AddSector(store.Sector{Name: name, Percentage: percentage, IDHolding: holding.IDHolding}); err != nil {
			return err
		}
	}
	for name, percentage := range composition.Regions {
		if err := m.DB.AddRegion(store.Region{Name: name, Percentage: percentage, IDHolding: holding.IDHolding}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) UpdateAllSentiments() {
	tickers, err := m.DB.UniqueTickers()
	if err != nil {
		return
	}
	today := store.Today()
	for _, ticker := range tickers {
		if err := m.UpdateTickerSentiment(ticker); err != nil {
			log.Printf("jobs: sentiment %s: %v", ticker, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	users, err := m.DB.GetAllUsers()
	if err != nil {
		return
	}
	for _, user := range users {
		if err := m.UpdatePortfolioSentiment(user.ID, today); err != nil {
			log.Printf("jobs: portfolio sentiment %s: %v", user.ID, err)
		}
	}
}

// UpdateTickerSentiment writes one row per ticker per day. Existing rows are reused
// unless the day gained news, which keeps repeat runs free.
func (m *Manager) UpdateTickerSentiment(ticker string) error {
	today := store.Today()
	existing, err := m.DB.GetDailySentiment(ticker, today)
	if err == nil && existing != nil && existing.Summary != "" {
		return nil
	}
	items, err := m.DB.NewsForTickerToday(ticker, today)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		items, err = m.DB.NewsForTicker(ticker, 30)
		if err != nil || len(items) == 0 {
			return nil
		}
	}
	articles := make([]llm.DailyArticle, 0, len(items))
	for _, item := range items {
		articles = append(articles, llm.DailyArticle{
			Title: item.Title, Summary: item.Summary, Text: item.Text, Sentiment: item.Sentiment,
		})
	}
	result := m.LLM.DailySummary(context.Background(), "system", llm.DailyInput{
		Ticker: ticker, Date: today, Articles: articles,
	}, m.Cfg.SummaryMaxChars)
	if strings.TrimSpace(result.Summary) == "" {
		return nil
	}
	return m.DB.UpsertDailySentiment(store.DailySentiment{
		IDSentiment: store.NewID(), Ticker: ticker, Date: today,
		Summary: result.Summary, Sentiment: result.Sentiment,
	})
}

func (m *Manager) UpdatePortfolioSentiment(userID string, date string) error {
	existing, err := m.DB.GetPortfolioSentiment(userID, date)
	if err == nil && existing != nil && existing.Summary != "" {
		return nil
	}
	holdings, err := m.DB.GetHoldingsByUser(userID)
	if err != nil {
		return err
	}
	var briefs []llm.HoldingBrief
	seen := map[string]bool{}
	for _, holding := range holdings {
		if seen[holding.Ticker] {
			continue
		}
		seen[holding.Ticker] = true
		sentiment, err := m.DB.GetDailySentiment(holding.Ticker, date)
		if err != nil || sentiment == nil || strings.TrimSpace(sentiment.Summary) == "" {
			continue
		}
		briefs = append(briefs, llm.HoldingBrief{Ticker: holding.Ticker, Summary: sentiment.Summary, Sentiment: sentiment.Sentiment})
	}
	if len(briefs) == 0 {
		return nil
	}
	result := m.LLM.PortfolioSummary(context.Background(), userID, date, briefs)
	return m.DB.UpsertPortfolioSentiment(store.PortfolioDailySentiment{
		IDSentiment: store.NewID(), UserID: userID, Date: date, Summary: result.Summary, Sentiment: result.Sentiment,
	})
}

// UpdateRunningSummary regenerates the multi day outlook used by the summary view.
func (m *Manager) UpdateRunningSummary(userID string, windowDays int) error {
	date := store.Today()
	startDate := time.Now().UTC().AddDate(0, 0, -windowDays).Format("2006-01-02")
	holdings, err := m.DB.GetHoldingsByUser(userID)
	if err != nil {
		return err
	}
	var briefs []llm.HoldingBrief
	seen := map[string]bool{}
	for _, holding := range holdings {
		if seen[holding.Ticker] {
			continue
		}
		seen[holding.Ticker] = true
		history, err := m.DB.DailySentimentHistory(holding.Ticker, 1)
		if err != nil || len(history) == 0 {
			continue
		}
		latest := history[0]
		if latest.Date < startDate {
			continue
		}
		briefs = append(briefs, llm.HoldingBrief{Ticker: holding.Ticker, Summary: latest.Summary, Sentiment: latest.Sentiment})
	}
	if len(briefs) == 0 {
		return fmt.Errorf("no holding summaries available for the window")
	}

	sectors := map[string]llm.SectorAllocation{}
	for _, holding := range holdings {
		if !holding.ETF {
			continue
		}
		holdingSectors, _, err := m.DB.GetBreakdownForHolding(holding.IDHolding)
		if err != nil {
			continue
		}
		for _, sector := range holdingSectors {
			entry := sectors[sector.Name]
			entry.Percentage += sector.Percentage
			entry.Tickers = append(entry.Tickers, holding.Ticker)
			sectors[sector.Name] = entry
		}
	}

	result := m.LLM.RunningSummary(context.Background(), userID, date, windowDays, briefs, sectors)
	err = m.DB.UpsertRunningSummary(store.RunningSummary{
		ID: store.NewID(), UserID: userID, WindowDays: windowDays, Date: date, Summary: result.Summary,
		Sentiment: result.Sentiment, SectorPredictions: result.SectorPredictions, ThemePredictions: result.ThemePredictions,
	})
	if err == nil {
		m.RAG.Trigger()
	}
	return err
}

func (m *Manager) RefreshAssetDetails() {
	tickers, err := m.DB.UniqueTickers()
	if err != nil {
		return
	}
	for _, ticker := range tickers {
		if err := m.UpdateAssetDetails(ticker); err != nil {
			log.Printf("jobs: details %s: %v", ticker, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (m *Manager) UpdateAssetDetails(ticker string) error {
	release := m.acquire()
	defer release()
	details, err := m.Market.Details(ticker, "")
	if err != nil || details == nil {
		return err
	}
	record := store.AssetDetails{
		Ticker: ticker, ISIN: details.ISIN, MarketCap: details.MarketCap, MarketCapEur: details.MarketCapEur,
		Country: details.Country, Sector: details.Sector,
		Eps: fmt.Sprintf("%.2f", details.Eps), PbRatio: fmt.Sprintf("%.2f", details.PbRatio),
		PeRatio: fmt.Sprintf("%.2f", details.PeRatio), DividendYield: fmt.Sprintf("%.2f", details.DividendYield),
		Revenue: details.Revenue, NetIncome: details.NetIncome, ProfitMargin: fmt.Sprintf("%.2f", details.ProfitMargin),
		Date: fmt.Sprintf("%d", time.Now().UTC().Unix()),
	}
	record.Hash = store.HashString(record.Ticker, record.MarketCap, record.MarketCapEur, record.Country,
		record.Sector, record.Eps, record.PbRatio, record.PeRatio, record.DividendYield, record.Revenue,
		record.NetIncome, record.ProfitMargin)
	latest, err := m.DB.LatestAssetDetails(ticker)
	if err != nil {
		return err
	}
	if latest != nil && latest.Hash == record.Hash {
		return nil
	}
	return m.DB.AddAssetDetails(record)
}

func (m *Manager) cooldown(key string, duration time.Duration) bool {
	m.cooldownMu.Lock()
	defer m.cooldownMu.Unlock()
	if last, ok := m.cooldowns[key]; ok && time.Since(last) < duration {
		return false
	}
	m.cooldowns[key] = time.Now()
	return true
}

// Situation generation mirrors the original two stage flow: gather with web search,
// then compose a JSON report. Searches per run are capped to bound cost.
func (m *Manager) GenerateSituation(task store.SituationTask) {
	m.situationMu.Lock()
	if m.situationRun[task.ID] {
		m.situationMu.Unlock()
		return
	}
	m.situationRun[task.ID] = true
	m.situationMu.Unlock()
	defer func() {
		m.situationMu.Lock()
		delete(m.situationRun, task.ID)
		m.situationMu.Unlock()
	}()

	today := store.Today()
	existing, _ := m.DB.GetSituationReport(task.ID, today)
	if existing != nil && existing.Status == "running" && time.Since(time.Unix(existing.UpdatedAt, 0)) < 15*time.Minute {
		return
	}

	report := &store.SituationReport{
		ID: store.NewID(), TaskID: task.ID, UserID: task.UserID, Subject: task.Subject,
		Date: today, Status: "running", CreatedAt: store.Now(), UpdatedAt: store.Now(),
	}
	if err := m.DB.UpsertSituationReport(report); err != nil {
		log.Printf("situation: create report: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	messages := []llm.Message{
		{Role: "system", Content: situationSystemPrompt()},
		{Role: "user", Content: situationGatherPrompt(task)},
	}
	tools := []llm.Tool{webSearchTool()}
	var gathered []store.SearchResult
	var status string
	searches := 0
	failed := false

	for round := 0; round < 4; round++ {
		completion, err := m.LLM.Chat(ctx, llm.KindSituation, "system", messages, tools, nil)
		if err != nil {
			log.Printf("situation: gather error: %v", err)
			failed = true
			break
		}
		if len(completion.ToolCalls) == 0 {
			break
		}
		messages = append(messages, llm.Message{Role: "assistant", ToolCalls: completion.ToolCalls})
		for _, call := range completion.ToolCalls {
			var args struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
			if searches >= 8 || strings.TrimSpace(args.Query) == "" {
				messages = append(messages, llm.Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name,
					Content: "Search budget exhausted."})
				continue
			}
			searches++
			results, searchStatus := m.searchWithCache(ctx, "system", args.Query)
			status = searchStatus
			gathered = append(gathered, results...)
			messages = append(messages, llm.Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name,
				Content: formatDetailed(results, 6000)})
		}
	}

	compose := []llm.Message{
		{Role: "system", Content: "You are an analyst producing a situation report for " + today + ". Output ONLY a JSON object with keys \"summary\" and \"report\". No extra text."},
		{Role: "user", Content: situationComposePrompt(task, gathered, status, m.DB.PreviousSituationSummary(task.ID, today))},
	}
	content, summary := "", ""
	if completion, err := m.LLM.Chat(ctx, llm.KindSituation, "system", compose, nil, nil); err == nil {
		content, summary = parseSituationJSON(completion.Content)
	} else {
		log.Printf("situation: compose error: %v", err)
		failed = true
	}
	if strings.TrimSpace(content) == "" {
		content = "I could not generate this situation report today. Please retry."
	}
	if strings.TrimSpace(summary) == "" {
		summary = llm.Truncate(content, 200)
	}

	if len(gathered) == 0 && !failed {
		attempts := m.DB.BumpSituationAttempts(task.ID, today)
		if attempts < 3 {
			content = fmt.Sprintf("Web search returned no results for this run (attempt %d of 3). The report will be retried automatically. Source status: %s", attempts, status)
			summary = "No web results yet, retrying later today."
			failed = true
		} else {
			content = "No live web results could be gathered for this report today after 3 attempts. Source status: " + status +
				".\n\nNo specific developments could be verified from live sources."
			summary = "Live web sources were unavailable today."
		}
	}

	searchJSON, _ := json.Marshal(gathered)
	reportStatus := "completed"
	if failed {
		reportStatus = "failed"
	}
	if err := m.DB.UpdateSituationReportResult(report.ID, content, summary, reportStatus, len(gathered), string(searchJSON)); err != nil {
		log.Printf("situation: save report: %v", err)
	}
	_ = m.DB.SaveSituationNews(task.ID, task.UserID, today, gathered)
	if !failed {
		_ = m.DB.UpdateSituationTaskLastRun(task.ID, today)
		m.RAG.Trigger()
	}
}

func (m *Manager) searchWithCache(ctx context.Context, userID string, query string) ([]store.SearchResult, string) {
	key := strings.ToLower(strings.TrimSpace(query))
	if cached, ok := m.DB.CachedWebSearch(key, 7*86400); ok {
		return cached, "cached"
	}
	results, status := m.Market.SearchWithContent(query, 5)
	stores := make([]store.SearchResult, 0, len(results))
	for _, result := range results {
		stores = append(stores, store.SearchResult{Title: result.Title, URL: result.URL, Snippet: result.Snippet, Text: result.Text})
	}
	if len(stores) > 0 {
		_ = m.DB.SaveWebSearch(key, userID, stores)
	}
	return stores, status
}

func webSearchTool() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.ToolFunction{
		Name:        "web_search",
		Description: "Search the live web for current information about companies, markets, news or events.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string", "description": "Search query."}},
			"required":   []string{"query"},
		},
	}}
}

func situationSystemPrompt() string {
	return "You are an intelligence analyst producing a daily situation report on a single subject. Today is " +
		store.Today() + ". You have a web_search tool. Sub-topics are areas to investigate, not mandatory sections. " +
		"Include a section only when you have concrete, dated findings. Use specific facts: dates, locations, numbers, names. " +
		"Never invent events and never write generic filler."
}

func situationGatherPrompt(task store.SituationTask) string {
	var sb strings.Builder
	sb.WriteString("SUBJECT: " + task.Subject + "\n")
	if len(task.SubTopics) > 0 {
		sb.WriteString("SUB-TOPICS (areas to investigate, not required sections):\n")
		for _, topic := range task.SubTopics {
			sb.WriteString("- " + topic + "\n")
		}
	}
	sb.WriteString("\nRun web searches for the subject and each sub-topic. Look for specific, dated events: dates, locations, numbers, names, policy changes. Search current dates. Use the web_search tool. Note when nothing is found for a sub-topic.")
	return sb.String()
}

func situationComposePrompt(task store.SituationTask, results []store.SearchResult, status string, previous string) string {
	var sb strings.Builder
	sb.WriteString("SUBJECT: " + task.Subject + "\n")
	if len(task.SubTopics) > 0 {
		sb.WriteString("SUB-TOPICS:\n")
		for _, topic := range task.SubTopics {
			sb.WriteString("- " + topic + "\n")
		}
	}
	if previous != "" {
		sb.WriteString("\n" + previous + "\n")
	}
	sb.WriteString("\nWeb search results and article content gathered today:\n")
	sb.WriteString(formatDetailed(results, 12000))
	if len(results) == 0 {
		sb.WriteString("\n\nSearch source status: " + status)
	}
	sb.WriteString("\n\nWrite the report. Include a section (## header) ONLY for sub-topics with specific dated findings. Skip empty topics and do not invent. Use concrete facts with dates and numbers. If fewer than two results were gathered, say briefly which sources were unavailable and keep it short. Respond with a single JSON object: {\"summary\": \"...\", \"report\": \"...\"} where report is markdown.")
	return sb.String()
}

func formatDetailed(results []store.SearchResult, maxChars int) string {
	if len(results) == 0 {
		return "No results found. Do not search again; answer from context and note that live results were unavailable."
	}
	var sb strings.Builder
	for _, result := range results {
		sb.WriteString("- " + llm.Truncate(result.Title, 120))
		if result.Snippet != "" {
			sb.WriteString(": " + llm.Truncate(result.Snippet, 300))
		}
		if result.Text != "" {
			sb.WriteString("\n  Content: " + llm.Truncate(result.Text, 600))
		}
		if result.URL != "" {
			sb.WriteString("\n  URL: " + result.URL)
		}
		sb.WriteString("\n")
	}
	return llm.Truncate(sb.String(), maxChars)
}

func parseSituationJSON(text string) (string, string) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		var payload struct {
			Summary string `json:"summary"`
			Report  string `json:"report"`
		}
		if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err == nil {
			return strings.TrimSpace(payload.Report), strings.TrimSpace(payload.Summary)
		}
	}
	return strings.TrimSpace(text), ""
}

func (m *Manager) SituationTick() {
	tasks, err := m.DB.ListEnabledSituationTasks()
	if err != nil {
		return
	}
	hour := time.Now().Hour()
	today := store.Today()
	for _, task := range tasks {
		if !m.situationDue(task, today, hour) {
			continue
		}
		copy := task
		go m.GenerateSituation(copy)
	}
}

func (m *Manager) situationDue(task store.SituationTask, today string, hour int) bool {
	if task.LastRunDate != today {
		return hour >= task.DailyHour
	}
	report, err := m.DB.GetSituationReport(task.ID, today)
	if err != nil || report == nil {
		return hour >= task.DailyHour
	}
	if report.Status == "completed" {
		return false
	}
	age := time.Since(time.Unix(report.UpdatedAt, 0))
	if report.Status == "running" && age < 15*time.Minute {
		return false
	}
	if report.Status == "failed" && age < 30*time.Minute {
		return false
	}
	return true
}

// startAutoReportLoop writes weekly and monthly expense reports once per period.
func (m *Manager) startAutoReportLoop() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		m.autoExpenseReports()
	}
}

func (m *Manager) autoExpenseReports() {
	users, err := m.DB.GetAllUsers()
	if err != nil {
		return
	}
	for _, user := range users {
		for _, period := range []string{"week", "month"} {
			if err := m.AutoExpenseReport(user.ID, period); err != nil {
				log.Printf("jobs: expense report %s %s: %v", user.ID, period, err)
			}
		}
	}
}

// AutoExpenseReport only generates a report when the period has no stored report yet.
func (m *Manager) AutoExpenseReport(userID string, period string) error {
	if !m.cooldown("report:"+userID+":"+period, 30*time.Minute) {
		return nil
	}
	startDate, endDate, err := PeriodDates(period)
	if err != nil {
		return err
	}
	exists, err := bills.ReportExistsForPeriod(userID, period, startDate, endDate)
	if err != nil || exists {
		return err
	}
	expenses, err := bills.GetExpensesByUserID(userID)
	if err != nil || len(expenses) == 0 {
		return nil
	}
	stats, _ := bills.GetBankTransactionStats(userID)
	var data strings.Builder
	total := 0.0
	byCategory := map[string]float64{}
	for _, expense := range expenses {
		if expense.Date < startDate || expense.Date > endDate {
			continue
		}
		total += expense.Amount
		byCategory[expense.Category] += expense.Amount
	}
	if total == 0 {
		return nil
	}
	fmt.Fprintf(&data, "Period total: %.2f\n", total)
	for category, amount := range byCategory {
		fmt.Fprintf(&data, "- %s: %.2f\n", category, amount)
	}
	if stats != nil {
		fmt.Fprintf(&data, "Bank totals: in %.2f out %.2f net %.2f\n",
			numberOf(stats["total_in"]), numberOf(stats["total_out"]), numberOf(stats["net"]))
	}
	summary, err := m.LLM.ExpenseReport(context.Background(), userID, period, startDate, endDate, data.String())
	if err != nil || summary == "" {
		return err
	}
	return bills.SaveExpenseReport(bills.ExpenseReport{
		Period: period, PeriodStart: startDate, PeriodEnd: endDate, Summary: summary, UserID: userID,
	})
}

func numberOf(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	}
	return 0
}

// PeriodDates resolves the current calendar period boundaries.
func PeriodDates(period string) (string, string, error) {
	now := time.Now()
	switch period {
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -weekday+1)
		return monday.Format("2006-01-02"), monday.AddDate(0, 0, 6).Format("2006-01-02"), nil
	case "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start.Format("2006-01-02"), start.AddDate(0, 1, -1).Format("2006-01-02"), nil
	case "quarter":
		month := ((int(now.Month()) - 1) / 3 * 3) + 1
		start := time.Date(now.Year(), time.Month(month), 1, 0, 0, 0, 0, now.Location())
		return start.Format("2006-01-02"), start.AddDate(0, 3, -1).Format("2006-01-02"), nil
	case "year":
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		return start.Format("2006-01-02"), start.AddDate(1, 0, -1).Format("2006-01-02"), nil
	default:
		return "", "", fmt.Errorf("unknown period %q", period)
	}
}
