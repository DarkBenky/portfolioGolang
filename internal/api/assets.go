package api

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"portfolio/internal/analytics"
	"portfolio/internal/prices"
	"portfolio/internal/store"
)

type quoteResult struct {
	Ticker             string  `json:"ticker"`
	Name               string  `json:"name"`
	Exchange           string  `json:"exchange"`
	ISIN               string  `json:"isin"`
	Currency           string  `json:"currency"`
	Type               string  `json:"type"`
	Price              float64 `json:"price"`
	TER                string  `json:"ter,omitempty"`
	DistributionPolicy string  `json:"distribution_policy,omitempty"`
}

func (s *Server) search(c echo.Context) error {
	identifier := strings.TrimSpace(c.QueryParam("identifier"))
	searchType := defaultQuery(c, "search_type", "ticker")
	if identifier == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Identifier parameter is required."})
	}

	if searchType == "isin" {
		if found, err := s.Market.TickerSearch(identifier, 5); err == nil && len(found) > 0 {
			quotes := make([]quoteResult, 0, len(found))
			for _, quote := range found {
				quotes = append(quotes, quoteResult{Ticker: quote.Ticker, Name: quote.Name,
					Exchange: quote.Exchange, ISIN: identifier, Currency: quote.Currency,
					Type: quote.Type, Price: quote.Price})
			}
			return c.JSON(http.StatusOK, quotes)
		}
		quotes := make([]quoteResult, 0, 1)
		if ticker := s.Market.ISINToTicker(identifier); ticker != "" {
			quotes = append(quotes, quoteResult{Ticker: ticker, Name: ticker, ISIN: identifier})
		}
		return c.JSON(http.StatusOK, quotes)
	}

	found, err := s.Market.TickerSearch(identifier, 10)
	if err != nil {
		return c.JSON(http.StatusOK, []quoteResult{})
	}
	quotes := make([]quoteResult, 0, len(found))
	for _, quote := range found {
		quotes = append(quotes, quoteResult{
			Ticker: quote.Ticker, Name: quote.Name, Exchange: quote.Exchange,
			ISIN: quote.ISIN, Currency: quote.Currency, Type: quote.Type, Price: quote.Price,
		})
	}
	if len(quotes) > 8 {
		quotes = quotes[:8]
	}
	sort.SliceStable(quotes, func(i, j int) bool {
		return strings.EqualFold(quotes[i].Ticker, identifier) && !strings.EqualFold(quotes[j].Ticker, identifier)
	})
	if len(quotes) > 0 && quotes[0].Type == "ETF" && quotes[0].ISIN != "" {
		if composition, err := s.Market.ETFComposition(quotes[0].Ticker, quotes[0].ISIN, quotes[0].Name); err == nil {
			quotes[0].TER = composition.TER
			quotes[0].DistributionPolicy = composition.Policy
		}
	}
	return c.JSON(http.StatusOK, quotes)
}

func (s *Server) convertCurrency(c echo.Context) error {
	amount := queryFloat(c, "amount", 0)
	from := defaultQuery(c, "from_currency", "USD")
	to := defaultQuery(c, "to_currency", "USD")
	converted, err := s.Market.CurrencyConversion(amount, from, to)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"amount": amount, "from_currency": from, "to_currency": to, "converted_amount": converted,
	})
}

func (s *Server) getPrice(c echo.Context) error {
	ticker := strings.TrimSpace(c.QueryParam("ticker"))
	if ticker == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ticker is required"})
	}
	interval := defaultQuery(c, "interval", "5m")
	if !prices.Supported(interval) {
		interval = "5m"
	}
	candles, err := s.assetCandles(ticker, interval)
	if err != nil {
		log.Printf("api: get_price %s: %v", ticker, err)
		return c.JSON(http.StatusOK, []prices.Candle{})
	}
	return c.JSON(http.StatusOK, candles)
}

// resolveTicker accepts a ticker or an ISIN. Holdings and assets already carry the
// mapping, so it is answered locally; only unknown ISINs reach the remote lookup.
func (s *Server) resolveTicker(identifier string) string {
	if ticker := s.DB.TickerForISIN(strings.TrimSpace(identifier)); ticker != "" {
		return ticker
	}
	return s.Market.ResolveTicker(identifier)
}

func (s *Server) getAssetHistory(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}
	interval := defaultQuery(c, "interval", "1d")
	if !prices.Supported(interval) {
		interval = "1d"
	}
	candles, err := s.assetCandles(identifier, interval)
	if err != nil {
		log.Printf("api: asset history %s: %v", identifier, err)
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}
	return c.JSON(http.StatusOK, candles)
}

// assetCandles reads stored bars for the interval and falls back to a live fetch when
// the ticker has no history yet. Fetched bars are stored so the next call is local.
func (s *Server) assetCandles(identifier string, interval string) ([]prices.Candle, error) {
	ticker := s.resolveTicker(identifier)
	now := time.Now().UTC()
	lookback := prices.IntervalLookback(interval, s.Cfg.PriceLookback5mDays)
	start := int64(0)
	if lookback > 0 {
		start = now.Add(-time.Duration(lookback) * time.Second).Unix()
	}
	candles, err := s.Prices.Candles(ticker, start, now.Unix(), interval)
	if err == nil && len(candles) > 0 {
		return candles, nil
	}
	bars, _, chartErr := s.Market.Chart(ticker, start, now.Unix(), interval)
	if chartErr != nil {
		if err != nil {
			return nil, err
		}
		return candles, nil
	}
	stored := make([]prices.Bar, 0, len(bars))
	for _, bar := range bars {
		if bar.Timestamp <= 0 || bar.Close == 0 {
			continue
		}
		stored = append(stored, prices.Bar{Ticker: ticker, Date: bar.Timestamp, Open: bar.Open,
			Close: bar.Close, High: bar.High, Low: bar.Low, Volume: bar.Volume})
	}
	if len(stored) > 0 {
		if upsertErr := s.Prices.Upsert(stored); upsertErr != nil {
			log.Printf("api: could not store %s bars: %v", ticker, upsertErr)
		}
	}
	return s.Prices.Candles(ticker, start, now.Unix(), interval)
}

func (s *Server) getAssetValue(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}
	latest, err := s.Prices.LatestCloses([]string{s.resolveTicker(identifier)})
	ticker := s.resolveTicker(identifier)
	if err != nil || latest[ticker] == 0 {
		return c.String(http.StatusInternalServerError, "Error retrieving latest price")
	}
	return c.JSON(http.StatusOK, map[string]float64{"latest_price": latest[ticker]})
}

func (s *Server) getAssetChange(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker parameter is required")
	}
	ticker := s.resolveTicker(identifier)
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	quantity := 0.0
	for _, holding := range holdings {
		if holding.Ticker == ticker || holding.ISIN == identifier || holding.Ticker == identifier {
			quantity += holding.Quantity
		}
	}
	if quantity == 0 {
		return c.String(http.StatusNotFound, "No holdings found for the specified asset")
	}
	latestMap, _ := s.Prices.LatestCloses([]string{ticker})
	latest := latestMap[ticker]
	if latest == 0 {
		return c.String(http.StatusInternalServerError, "Error retrieving latest price")
	}
	previous := latest
	if value, err := s.Prices.LastCloseBefore(ticker, time.Now().Add(-24*time.Hour).Unix()); err == nil {
		previous = value
	}
	currentValue := latest * quantity
	dayChange := (latest - previous) * quantity
	return c.JSON(http.StatusOK, map[string]float64{
		"current_value": currentValue, "day_change": dayChange,
		"day_change_percent": percentOf(latest-previous, previous),
	})
}

func (s *Server) getAssetNews(c echo.Context) error {
	limit := queryInt(c, "limit", 10)
	offset := queryOffset(c)
	identifier := c.QueryParam("ticker")
	ticker := s.resolveTicker(identifier)
	tickers := []string{ticker}
	holdings, _ := s.DB.GetHoldingsByTicker(ticker)
	for _, holding := range holdings {
		if holding.ETF {
			tickers = append(tickers, s.underlyingTickers(holding)...)
			break
		}
	}
	news, err := s.DB.NewsForTickers(tickers, limit, offset)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving news")
	}
	return c.JSON(http.StatusOK, news)
}

func (s *Server) getAssetSentiment(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}
	ticker := s.resolveTicker(identifier)
	tickers := []string{ticker}
	holdings, _ := s.DB.GetHoldingsByTicker(ticker)
	for _, holding := range holdings {
		if holding.ETF {
			tickers = append(tickers, s.underlyingTickers(holding)...)
			break
		}
	}
	average, count, err := s.DB.AverageNewsSentiment(tickers, startOfToday(), startOfToday()+86400)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving sentiment")
	}
	return c.JSON(http.StatusOK, map[string]any{"ticker": ticker, "average_sentiment": average, "news_count": count})
}

func (s *Server) getAssetSentiments(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}
	ticker := s.resolveTicker(identifier)
	tickers := []string{ticker}
	holdings, _ := s.DB.GetHoldingsByTicker(ticker)
	for _, holding := range holdings {
		if holding.ETF {
			tickers = append(tickers, s.underlyingTickers(holding)...)
			break
		}
	}
	news, err := s.DB.NewsForTickers(tickers, 100, 0)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving sentiments")
	}
	type entry struct {
		PublishedAt int64   `json:"published_at"`
		Sentiment   float64 `json:"sentiment"`
	}
	result := make([]entry, 0, len(news))
	for _, item := range news {
		result = append(result, entry{PublishedAt: store.PublishedUnix(item.PublishedAt), Sentiment: item.Sentiment})
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) getAssetDailySentiment(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	date := c.QueryParam("date")
	if identifier == "" || date == "" {
		return c.String(http.StatusBadRequest, "ticker and date parameters are required")
	}
	ticker := s.resolveTicker(identifier)
	sentiment, err := s.DB.GetDailySentiment(ticker, date)
	if err != nil {
		sentiment = nil
	}
	holdings, _ := s.DB.GetHoldingsByTicker(ticker)
	for _, holding := range holdings {
		if !holding.ETF {
			continue
		}
		underlying := s.underlyingTickers(holding)
		if len(underlying) == 0 {
			break
		}
		total, count := 0.0, 0
		var summaries []string
		if sentiment != nil {
			total += sentiment.Sentiment
			count++
			if sentiment.Summary != "" {
				summaries = append(summaries, sentiment.Summary)
			}
		}
		for _, component := range underlying {
			if row, err := s.DB.GetDailySentiment(component, date); err == nil && row != nil {
				total += row.Sentiment
				count++
				if row.Summary != "" {
					summaries = append(summaries, row.Summary)
				}
			}
		}
		if count > 0 {
			if sentiment == nil {
				sentiment = &store.DailySentiment{Ticker: ticker, Date: date}
			}
			sentiment.Sentiment = total / float64(count)
			if len(summaries) > 0 {
				sentiment.Summary = "Combined ETF sentiment: " + strings.Join(summaries, " | ")
			}
		}
		break
	}
	if sentiment == nil {
		return c.JSON(http.StatusOK, nil)
	}
	return c.JSON(http.StatusOK, sentiment)
}

type assetStats struct {
	Ticker       string  `json:"ticker"`
	YoYReturn    float64 `json:"yoy_return"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	AvgDrawdown  float64 `json:"avg_drawdown"`
	SortinoRatio float64 `json:"sortino_ratio"`
	TER          float64 `json:"ter"`
	CurrentPrice float64 `json:"current_price"`
	Quantity     float64 `json:"quantity"`
	CurrentValue float64 `json:"current_value"`
	CostBasis    float64 `json:"cost_basis"`
	GainLoss     float64 `json:"gain_loss"`
	GainLossPct  float64 `json:"gain_loss_pct"`
}

func (s *Server) getAssetStats(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		identifier = c.QueryParam("isin")
	}
	if identifier == "" {
		return c.String(http.StatusBadRequest, "ticker or isin parameter is required")
	}
	ticker := s.resolveTicker(identifier)
	now := time.Now().UTC()
	closes, dates, err := s.Prices.SeriesWithDates(ticker, now.AddDate(-1, 0, 0).Unix(), now.Unix())
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}
	if len(closes) == 0 {
		if _, err := s.assetCandles(ticker, "1d"); err != nil {
			return c.String(http.StatusNotFound, "No price history found")
		}
		closes, dates, _ = s.Prices.SeriesWithDates(ticker, now.AddDate(-1, 0, 0).Unix(), now.Unix())
	}
	if len(closes) == 0 {
		return c.String(http.StatusNotFound, "No price history found")
	}
	maxDD, avgDD := analytics.Drawdowns(closes)
	returns := analytics.DailyReturns(closes)

	holdings, _ := s.DB.GetHoldingsByUser(s.userID(c))
	quantity, cost, ter := 0.0, 0.0, 0.0
	for _, holding := range holdings {
		if holding.Ticker == ticker || holding.ISIN == identifier {
			quantity += holding.Quantity
			cost += holding.PurchasePrice * holding.Quantity
			ter = holding.TER
		}
	}
	currentPrice := closes[len(closes)-1]
	value := currentPrice * quantity
	gain := value - cost

	return c.JSON(http.StatusOK, assetStats{
		Ticker: ticker, YoYReturn: analytics.Round(analytics.YoYReturn(closes, dates, now), 2),
		MaxDrawdown: analytics.Round(maxDD, 2), AvgDrawdown: analytics.Round(avgDD, 2),
		SortinoRatio: analytics.Round(analytics.Sortino(returns, 0), 2), TER: ter,
		CurrentPrice: analytics.Round(currentPrice, 2), Quantity: quantity,
		CurrentValue: analytics.Round(value, 2), CostBasis: analytics.Round(cost, 2),
		GainLoss: analytics.Round(gain, 2), GainLossPct: analytics.Round(percentOf(gain, cost), 2),
	})
}

func (s *Server) getAssetDetails(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}
	latest, err := s.DB.LatestAssetDetails(ticker)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving asset details")
	}
	if latest == nil {
		// Fundamentals are stored under the ISIN for stocks, so a ticker lookup falls
		// back to the ISIN recorded on the holding.
		if holdings, holdingErr := s.DB.GetHoldingsByTicker(ticker); holdingErr == nil {
			for _, holding := range holdings {
				if holding.ISIN == "" {
					continue
				}
				if latest, err = s.DB.LatestAssetDetails(holding.ISIN); err == nil && latest != nil {
					break
				}
			}
		}
	}
	if latest == nil {
		return c.JSON(http.StatusOK, nil)
	}
	return c.JSON(http.StatusOK, latest)
}

func (s *Server) getAssetDetailsHistory(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}
	details, err := s.DB.AssetDetailsHistory(ticker)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving asset details")
	}
	return c.JSON(http.StatusOK, details)
}
