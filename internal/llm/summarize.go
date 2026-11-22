package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"portfolio/internal/store"
)

type DailyArticle struct {
	Title     string
	Summary   string
	Text      string
	Sentiment float64
}

type DailyInput struct {
	Ticker   string
	Date     string
	Articles []DailyArticle
}

type SummaryResult struct {
	Summary   string
	Sentiment float64
}

type HoldingBrief struct {
	Ticker    string
	Summary   string
	Sentiment float64
}

type RunningResult struct {
	Summary           string
	SectorPredictions string
	ThemePredictions  string
	Sentiment         float64
}

func sentimentLabel(value float64) string {
	switch {
	case value > 0.2:
		return "Bullish"
	case value < -0.2:
		return "Bearish"
	default:
		return "Neutral"
	}
}

// DailySummary summarises one ticker for one day. The cache key covers the article
// set, so regenerating the same day costs nothing.
func (c *Client) DailySummary(ctx context.Context, userID string, input DailyInput, maxContextChars int) SummaryResult {
	if len(input.Articles) == 0 {
		return SummaryResult{
			Summary: fmt.Sprintf("## Summary\nNo news available for %s on %s.", input.Ticker, input.Date),
		}
	}

	labels := []string{"n/a", "n/a", "n/a", input.Ticker, input.Date}
	sentimentSum := 0.0
	var items []string
	used := 0
	for i, article := range input.Articles {
		sentimentSum += article.Sentiment
		content := strings.TrimSpace(article.Summary)
		if content == "" {
			content = strings.TrimSpace(article.Title)
		}
		if text := strings.TrimSpace(article.Text); text != "" && len(content) < 240 && used+len(text) < maxContextChars {
			content = text
		}
		if used+len(content) > maxContextChars {
			content = Truncate(content, max(0, maxContextChars-used))
		}
		used += len(content)
		marker := "~"
		if article.Sentiment > 0.2 {
			marker = "+"
		} else if article.Sentiment < -0.2 {
			marker = "-"
		}
		items = append(items, fmt.Sprintf("Article %d [%s %.2f]:\n%s", i+1, marker, article.Sentiment, content))
		if used >= maxContextChars {
			break
		}
	}
	average := sentimentSum / float64(len(input.Articles))
	label := sentimentLabel(average)

	prompt := fmt.Sprintf(`Analyze these %d news articles about %s from %s and create a clear, factual summary.

Articles:
%s

Overall market sentiment: %s (%.2f)

Write a concise summary in markdown with these sections:

## Summary
Write 3-4 sentences summarizing the most important developments with specific facts, numbers, and dates from the articles.

## Key Points
- List 3-5 bullet points with concrete details (earnings numbers, percentage changes, product launches, analyst ratings, etc.)

## Outlook
Write 1-2 sentences about potential impact on stock price or what investors should monitor.

Focus only on facts from the articles. Include specific numbers, percentages, dates, and company names. Do not add speculation or make up information.`,
		len(input.Articles), input.Ticker, input.Date, strings.Join(items, "\n\n"), label, average)

	cacheKey := store.HashString(strings.Join(labels, "|"), fmt.Sprint(len(input.Articles)), strings.Join(items, "\n"))
	text, err := c.Complete(ctx, KindDailySummary, userID,
		"You write factual, compact market news digests. Never invent facts.", prompt, cacheKey)
	if err != nil || len(strings.TrimSpace(text)) < 50 {
		return SummaryResult{Summary: fallbackDaily(input, label, average), Sentiment: average}
	}
	return SummaryResult{Summary: text, Sentiment: average}
}

func fallbackDaily(input DailyInput, label string, average float64) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Summary\n%s had %d news items on %s. Sentiment: %s (%.2f)\n\n",
		input.Ticker, len(input.Articles), input.Date, label, average)
	limit := min(len(input.Articles), 5)
	for i := 0; i < limit; i++ {
		sb.WriteString("- " + Truncate(input.Articles[i].Title, 200) + "\n")
	}
	return sb.String()
}

// PortfolioSummary produces the cross holding overview and appends the per holding
// detail, keeping the same structure the dashboard expects.
func (c *Client) PortfolioSummary(ctx context.Context, userID string, date string, briefs []HoldingBrief) SummaryResult {
	if len(briefs) == 0 {
		return SummaryResult{Summary: fmt.Sprintf("No holding summaries available for your portfolio on %s.", date)}
	}
	sentimentSum := 0.0
	seen := map[string]bool{}
	var tickers []string
	var blocks []string
	var short []string
	for _, brief := range briefs {
		sentimentSum += brief.Sentiment
		if !seen[brief.Ticker] {
			seen[brief.Ticker] = true
			tickers = append(tickers, brief.Ticker)
		}
		blocks = append(blocks, brief.Ticker+"\n"+brief.Summary+"\n")
		short = append(short, fmt.Sprintf("%s: %s (%.2f) - %s", brief.Ticker, sentimentLabel(brief.Sentiment),
			brief.Sentiment, strings.ReplaceAll(Truncate(brief.Summary, 400), "\n", " ")))
	}
	average := sentimentSum / float64(len(briefs))
	label := sentimentLabel(average)

	if len(short) > 30 {
		short = short[:30]
	}
	prompt := fmt.Sprintf(`You are analyzing a portfolio with these holdings on %s: %s

Overall portfolio sentiment: %s (%.2f)

Here are pre-generated summaries for each holding:

%s

Write a concise portfolio overview in markdown with exactly these sections:

## Portfolio Summary
Write 3-4 sentences highlighting the most impactful news across the portfolio. Mention which holdings had the most significant developments and the overall tone. Include specific facts and numbers from the summaries.

## Market Impact
Write 2-3 sentences about how these developments may affect the portfolio and what investors should monitor going forward.

Keep it focused on the big picture. Detailed per-holding summaries are appended separately, so do not repeat individual article lists. Only include facts from the provided summaries. Do not speculate or invent information.`,
		date, strings.Join(tickers, ", "), label, average, strings.Join(short, "\n\n"))

	cacheKey := store.HashString("portfolio-v2", date, strings.Join(tickers, ","), strings.Join(short, "\n"))
	overview, err := c.Complete(ctx, KindPortfolioSummary, userID,
		"You are a portfolio analyst. Only use the supplied summaries.", prompt, cacheKey)
	if err != nil || len(strings.TrimSpace(overview)) < 50 {
		overview = fmt.Sprintf("## Portfolio Summary\nYour portfolio had summarized news across %d holdings on %s. Overall sentiment: %s (%.2f).\n\n## Market Impact\nReview individual holding details below for specific developments affecting your positions.",
			len(tickers), date, label, average)
	}

	combined := fmt.Sprintf("Portfolio Update - %s\n\n%s\n\n---\n\n## Individual Holdings Detail\n\n%s",
		label, overview, strings.Join(blocks, "\n"))
	return SummaryResult{Summary: combined, Sentiment: average}
}

// RunningSummary asks for a strict JSON object and degrades to a short text summary
// when the model returns something unparsable.
func (c *Client) RunningSummary(ctx context.Context, userID string, date string, windowDays int, briefs []HoldingBrief, sectors map[string]SectorAllocation) RunningResult {
	if len(briefs) == 0 {
		return RunningResult{Summary: fmt.Sprintf("No holding data available for the last %d days.", windowDays)}
	}
	sentimentSum := 0.0
	var lines []string
	used := 0
	for _, brief := range briefs {
		if len(lines) >= 40 {
			break
		}
		sentimentSum += brief.Sentiment
		line := fmt.Sprintf("%s [%s %.2f]: %s", brief.Ticker, sentimentLabel(brief.Sentiment), brief.Sentiment,
			strings.ReplaceAll(Truncate(brief.Summary, 400), "\n", " "))
		if used+len(line) > 12000 {
			break
		}
		lines = append(lines, line)
		used += len(line) + 2
	}
	average := sentimentSum / float64(len(briefs))

	var sectorLines []string
	for name, allocation := range sectors {
		sectorLines = append(sectorLines, fmt.Sprintf("%s (%.1f%% of portfolio): %s", name,
			allocation.Percentage, strings.Join(allocation.Tickers, ", ")))
	}
	sectorsBlock := strings.Join(sectorLines, "\n")
	if sectorsBlock == "" {
		sectorsBlock = "No sector data available."
	}

	prompt := fmt.Sprintf(`You are a senior financial analyst producing a running summary report covering the last %d days ending %s.

## Ticker Summaries Across the Period
%s

## Portfolio Sector Allocation
%s

Overall portfolio sentiment across the window: %s (%.2f)

Produce a comprehensive report. You MUST respond ONLY with a valid JSON object using this exact schema:

{
  "executive_summary": "3-4 sentence overview of the most impactful developments across the entire %d-day window. Include specific facts, dates, numbers and company names.",
  "key_themes": [{"theme": "Theme Name", "description": "2-3 sentences with evidence from the data.", "impact_sentiment": 0.5}],
  "sector_predictions": [{"sector": "Sector Name", "scenarios": [
     {"label": "Very Positive", "probability": 10, "description": "driver and impact"},
     {"label": "Positive", "probability": 35, "description": "driver and impact"},
     {"label": "Neutral", "probability": 30, "description": "driver and impact"},
     {"label": "Negative", "probability": 20, "description": "driver and impact"},
     {"label": "Very Negative", "probability": 5, "description": "driver and impact"}]}],
  "theme_predictions": [{"theme": "Theme Name", "scenarios": [
     {"label": "Very Positive", "probability": 10, "description": "driver"},
     {"label": "Positive", "probability": 30, "description": "driver"},
     {"label": "Neutral", "probability": 35, "description": "driver"},
     {"label": "Negative", "probability": 20, "description": "driver"},
     {"label": "Very Negative", "probability": 5, "description": "driver"}]}],
  "portfolio_outlook": {"scenarios": [
     {"label": "Very Positive", "probability": 10, "description": "driver"},
     {"label": "Positive", "probability": 30, "description": "driver"},
     {"label": "Neutral", "probability": 35, "description": "driver"},
     {"label": "Negative", "probability": 20, "description": "driver"},
     {"label": "Very Negative", "probability": 5, "description": "driver"}]}
}

RULES: probabilities per scenario set must sum to 100; impact_sentiment between -1 and 1; only use supplied facts; respond with JSON only.`,
		windowDays, date, strings.Join(lines, "\n\n"), sectorsBlock, sentimentLabel(average), average, windowDays)

	cacheKey := store.HashString("running-v2", date, fmt.Sprint(windowDays), strings.Join(lines, "\n"), sectorsBlock)
	raw, err := c.Complete(ctx, KindRunningSummary, userID,
		"You are a portfolio analyst. Respond with JSON only.", prompt, cacheKey)
	if err != nil {
		return RunningResult{Summary: fallbackRunning(windowDays, briefs, average), Sentiment: average}
	}
	parsed, ok := parseRunning(raw)
	if !ok {
		return RunningResult{Summary: fallbackRunning(windowDays, briefs, average), Sentiment: average}
	}
	parsed.Sentiment = average
	return parsed
}

type SectorAllocation struct {
	Percentage float64
	Tickers    []string
}

func parseRunning(raw string) (RunningResult, bool) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return RunningResult{}, false
	}
	var payload struct {
		ExecutiveSummary  string           `json:"executive_summary"`
		KeyThemes         []map[string]any `json:"key_themes"`
		SectorPredictions []map[string]any `json:"sector_predictions"`
		ThemePredictions  []map[string]any `json:"theme_predictions"`
		PortfolioOutlook  map[string]any   `json:"portfolio_outlook"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &payload); err != nil {
		return RunningResult{}, false
	}

	var combined strings.Builder
	combined.WriteString(payload.ExecutiveSummary)
	if len(payload.KeyThemes) > 0 {
		combined.WriteString("\n\n## Key Themes\n")
		for _, theme := range payload.KeyThemes {
			name, _ := theme["theme"].(string)
			description, _ := theme["description"].(string)
			if name != "" {
				fmt.Fprintf(&combined, "\n### %s\n%s\n", name, description)
			}
		}
	}
	writeScenarios := func(title string, items []map[string]any) {
		if len(items) == 0 {
			return
		}
		combined.WriteString("\n\n## " + title + "\n")
		for _, item := range items {
			label, _ := item["sector"].(string)
			if label == "" {
				label, _ = item["theme"].(string)
			}
			fmt.Fprintf(&combined, "\n### %s\n", label)
			writeScenarioList(&combined, item["scenarios"])
		}
	}
	writeScenarios("Sector Predictions", payload.SectorPredictions)
	writeScenarios("Theme Predictions", payload.ThemePredictions)
	if payload.PortfolioOutlook != nil {
		combined.WriteString("\n\n## Portfolio Outlook\n")
		writeScenarioList(&combined, payload.PortfolioOutlook["scenarios"])
	}

	sectorJSON, _ := json.Marshal(payload.SectorPredictions)
	themeJSON, _ := json.Marshal(payload.ThemePredictions)
	return RunningResult{
		Summary:           combined.String(),
		SectorPredictions: string(sectorJSON),
		ThemePredictions:  string(themeJSON),
	}, true
}

func writeScenarioList(sb *strings.Builder, raw any) {
	scenarios, ok := raw.([]any)
	if !ok {
		return
	}
	for _, scenario := range scenarios {
		item, ok := scenario.(map[string]any)
		if !ok {
			continue
		}
		label, _ := item["label"].(string)
		probability, _ := item["probability"].(float64)
		description, _ := item["description"].(string)
		fmt.Fprintf(sb, "- **%s** (%d%%): %s\n", label, int(probability), description)
	}
}

func fallbackRunning(windowDays int, briefs []HoldingBrief, average float64) string {
	var tickers []string
	for i, brief := range briefs {
		if i >= 10 {
			break
		}
		tickers = append(tickers, brief.Ticker)
	}
	return fmt.Sprintf("Over the last %d days, your portfolio showed %s sentiment (%.2f). Key holdings included %s.",
		windowDays, strings.ToLower(sentimentLabel(average)), average, strings.Join(tickers, ", "))
}

// ExpenseReport turns aggregated spending numbers into a short readable report.
func (c *Client) ExpenseReport(ctx context.Context, userID string, period string, periodStart string, periodEnd string, data string) (string, error) {
	prompt := fmt.Sprintf(`You are a personal finance analyst. Write a concise expense report in markdown for the %s period (%s to %s).

Data:
%s

Use exactly these sections:

## Overview
2-3 sentences about total spending and the largest category.

## Breakdown
- Bullet points per category with amounts and share of spending.

## Observations
- 2-4 bullets with concrete, data backed observations and one actionable suggestion.

Only use the numbers provided. Do not invent values.`, period, periodStart, periodEnd, data)

	cacheKey := store.HashString("expense-v2", period, periodStart, periodEnd, data)
	return c.Complete(ctx, KindExpenseReport, userID,
		"You are a factual personal finance analyst.", prompt, cacheKey)
}

// ClassifyExpense maps an unknown merchant description to one of the allowed categories.
func (c *Client) ClassifyExpense(ctx context.Context, userID string, description string, categories []string) string {
	prompt := fmt.Sprintf("Merchant description: %s\n\nCategories: %s\n\nReply with exactly one category name from the list and nothing else.",
		Truncate(description, 300), strings.Join(categories, ", "))
	cacheKey := store.HashString("classify-v2", description, strings.Join(categories, ","))
	answer, err := c.Complete(ctx, KindClassify, userID, "You classify bank transactions. Reply with one category name only.", prompt, cacheKey)
	if err != nil {
		return ""
	}
	answer = strings.TrimSpace(strings.Trim(answer, `."'`))
	for _, category := range categories {
		if strings.EqualFold(category, answer) {
			return category
		}
	}
	return ""
}
