package api

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"portfolio/internal/analytics"
	"portfolio/internal/store"
)

type holdingDetail struct {
	IDHolding     string        `json:"id_holding"`
	Name          string        `json:"name"`
	Ticker        string        `json:"ticker"`
	ISIN          string        `json:"isin"`
	Exchange      string        `json:"exchange"`
	Policy        string        `json:"policy"`
	Currency      string        `json:"currency"`
	Quantity      float64       `json:"quantity"`
	PurchasePrice float64       `json:"purchase_price"`
	TER           float64       `json:"ter"`
	ETF           bool          `json:"etf"`
	Sectors       []nameWeight  `json:"sectors"`
	Regions       []nameWeight  `json:"regions"`
	Assets        []store.Asset `json:"assets,omitempty"`
}

type nameWeight struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
}

func (s *Server) getHoldings(c echo.Context) error {
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	result := make([]holdingDetail, 0, len(holdings))
	for _, holding := range holdings {
		detail := holdingDetail{
			IDHolding: holding.IDHolding, Name: holding.Name, Ticker: holding.Ticker, ISIN: holding.ISIN,
			Exchange: holding.Exchange, Policy: holding.Policy, Currency: holding.Currency,
			Quantity: holding.Quantity, PurchasePrice: holding.PurchasePrice, TER: holding.TER,
			ETF: holding.ETF, Sectors: []nameWeight{}, Regions: []nameWeight{},
		}
		sectors, regions, breakdownErr := s.DB.GetBreakdownForHolding(holding.IDHolding)
		if breakdownErr == nil {
			for _, sector := range sectors {
				detail.Sectors = append(detail.Sectors, nameWeight{Name: sector.Name, Percentage: sector.Percentage})
			}
			for _, region := range regions {
				detail.Regions = append(detail.Regions, nameWeight{Name: region.Name, Percentage: region.Percentage})
			}
		}
		assets, assetErr := s.DB.GetAssetsForHolding(holding.IDHolding)
		if assetErr == nil && len(assets) > 0 {
			detail.Assets = assets
		}
		result = append(result, detail)
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) addHolding(c echo.Context) error {
	userID := s.userID(c)
	holding := s.holdingFromForm(c, store.NewID(), userID)
	if err := s.DB.AddHolding(holding); err != nil {
		return c.String(http.StatusInternalServerError, "Error adding holding to database")
	}
	s.afterHoldingChange(holding, false, false)
	return c.String(http.StatusOK, "Holding added successfully")
}

func (s *Server) modifyHolding(c echo.Context) error {
	userID := s.userID(c)
	holdingID := c.FormValue("HoldingID")
	if holdingID == "" {
		return c.String(http.StatusBadRequest, "HoldingID is required")
	}
	previous, err := s.DB.GetHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	wasETF := false
	previousISIN := ""
	for _, holding := range previous {
		if holding.IDHolding == holdingID {
			wasETF = holding.ETF
			previousISIN = holding.ISIN
		}
	}
	holding := s.holdingFromForm(c, holdingID, userID)
	if err := s.DB.ModifyHolding(holding); err != nil {
		return c.String(http.StatusInternalServerError, "Error modifying holding in database")
	}
	s.afterHoldingChange(holding, wasETF, previousISIN != holding.ISIN)
	return c.String(http.StatusOK, "Holding modified successfully")
}

func (s *Server) removeHolding(c echo.Context) error {
	holdingID := c.FormValue("HoldingID")
	if holdingID == "" {
		return c.String(http.StatusBadRequest, "HoldingID is required")
	}
	if err := s.DB.RemoveHolding(holdingID, s.userID(c)); err != nil {
		return c.String(http.StatusInternalServerError, "Error removing holding from database")
	}
	return c.String(http.StatusOK, "Holding removed successfully")
}

func (s *Server) holdingFromForm(c echo.Context, id string, userID string) store.Holding {
	quantity, _ := strconv.ParseFloat(c.FormValue("Quantity"), 64)
	purchasePrice, _ := strconv.ParseFloat(c.FormValue("PurchasePrice"), 64)
	ter, _ := strconv.ParseFloat(c.FormValue("TER"), 64)
	return store.Holding{
		IDHolding: id, Name: c.FormValue("Name"), Ticker: strings.ToUpper(strings.TrimSpace(c.FormValue("Ticker"))),
		ISIN: c.FormValue("ISIN"), Exchange: c.FormValue("Exchange"), ETF: c.FormValue("ETF") == "true",
		Quantity: quantity, PurchasePrice: purchasePrice, TER: ter, Policy: c.FormValue("Policy"),
		UserID: userID, Currency: c.FormValue("Currency"),
	}
}

// afterHoldingChange kicks off the background refreshes a new or edited holding needs.
func (s *Server) afterHoldingChange(holding store.Holding, wasETF bool, isinChanged bool) {
	if !s.Background {
		return
	}
	go func() {
		if err := s.Jobs.EnsureHistory(holding.Ticker); err != nil {
			log.Printf("api: history %s: %v", holding.Ticker, err)
		}
		if err := s.Jobs.FetchPrices(holding.Ticker); err != nil {
			log.Printf("api: prices %s: %v", holding.Ticker, err)
		}
		if err := s.Jobs.FetchNews(holding.Ticker, holding.Ticker, 10); err != nil {
			log.Printf("api: news %s: %v", holding.Ticker, err)
		}
	}()
	if !holding.ETF {
		if wasETF {
			go func() {
				_ = s.DB.DeleteAssetsForHolding(holding.IDHolding)
				_ = s.DB.DeleteBreakdownForHolding(holding.IDHolding)
			}()
		}
		return
	}
	go func() {
		if wasETF || isinChanged {
			_ = s.DB.DeleteAssetsForHolding(holding.IDHolding)
			_ = s.DB.DeleteBreakdownForHolding(holding.IDHolding)
		}
		if err := s.Jobs.RefreshETF(holding); err != nil {
			log.Printf("api: etf %s: %v", holding.Ticker, err)
		}
	}()
}

// toCurrency converts a value from a holding currency into the requested one. An empty
// target keeps the raw sum, which is what the API returned before conversion existed.
func (s *Server) toCurrency(value float64, from string, to string) float64 {
	if to == "" || from == "" || strings.EqualFold(from, to) {
		return value
	}
	rate, err := s.Market.Rate(from, to)
	if err != nil || rate <= 0 {
		return value
	}
	return value * rate
}

func (s *Server) getPortfolioValue(c echo.Context) error {
	target := c.QueryParam("currency")
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]float64{"total_value": 0})
	}
	quantities := map[string]float64{}
	costs := map[string]float64{}
	currencies := map[string]string{}
	var tickers []string
	for _, holding := range holdings {
		if _, seen := quantities[holding.Ticker]; !seen {
			tickers = append(tickers, holding.Ticker)
			currencies[holding.Ticker] = holding.Currency
		}
		quantities[holding.Ticker] += holding.Quantity
		costs[holding.Ticker] = holding.PurchasePrice
	}
	latest, err := s.Prices.LatestCloses(tickers)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error fetching prices")
	}
	total := 0.0
	for ticker, quantity := range quantities {
		price, ok := latest[ticker]
		if !ok {
			price = costs[ticker]
		}
		total += s.toCurrency(price*quantity, currencies[ticker], target)
	}
	return c.JSON(http.StatusOK, map[string]float64{"total_value": total})
}

type portfolioCandle struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
	Sentiment float64 `json:"sentiment,omitempty"`
}

func (s *Server) getPortfolioHistory(c echo.Context) error {
	interval := c.QueryParam("interval")
	if !pricesIntervalSupported(interval) {
		interval = "1d"
	}
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, []portfolioCandle{})
	}
	quantities := map[string]float64{}
	var tickers []string
	for _, holding := range holdings {
		if _, seen := quantities[holding.Ticker]; !seen {
			tickers = append(tickers, holding.Ticker)
		}
		quantities[holding.Ticker] += holding.Quantity
	}

	now := time.Now().UTC()
	lookback := analyticsLookback(interval)
	start := int64(0)
	if lookback > 0 {
		start = now.Add(-time.Duration(lookback) * time.Second).Unix()
	}
	series, err := s.Prices.CandlesMulti(tickers, start, now.Unix(), interval)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}

	type bar struct {
		open, high, low, close float64
		volume                 int64
	}
	buckets := map[int64]map[string]bar{}
	for ticker, candles := range series {
		for _, candle := range candles {
			if buckets[candle.Timestamp] == nil {
				buckets[candle.Timestamp] = map[string]bar{}
			}
			buckets[candle.Timestamp][ticker] = bar{candle.Open, candle.High, candle.Low, candle.Close, candle.Volume}
		}
	}
	timestamps := make([]int64, 0, len(buckets))
	for timestamp := range buckets {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	lastKnown := map[string]bar{}
	result := make([]portfolioCandle, 0, len(timestamps))
	for _, timestamp := range timestamps {
		var open, high, low, closePrice float64
		var volume int64
		count := 0
		for ticker, quantity := range quantities {
			current, ok := buckets[timestamp][ticker]
			if ok {
				lastKnown[ticker] = current
			} else {
				current, ok = lastKnown[ticker]
				if !ok {
					continue
				}
			}
			open += current.open * quantity
			high += current.high * quantity
			low += current.low * quantity
			closePrice += current.close * quantity
			volume += current.volume
			count++
		}
		if count > 0 && closePrice > 0 {
			result = append(result, portfolioCandle{Timestamp: timestamp, Open: open, High: high,
				Low: low, Close: closePrice, Volume: volume})
		}
	}

	attachSentiment(s, s.userID(c), interval, result)
	return c.JSON(http.StatusOK, downsamplePortfolio(result, s.Cfg.ChartMaxPoints))
}

// downsamplePortfolio applies the same point budget as single ticker candles, so the
// dashboard payload stays small even for accounts with many holdings.
func downsamplePortfolio(candles []portfolioCandle, maxPoints int) []portfolioCandle {
	if maxPoints <= 1 || len(candles) <= maxPoints {
		return candles
	}
	factor := (len(candles) + maxPoints - 1) / maxPoints
	result := make([]portfolioCandle, 0, (len(candles)+factor-1)/factor)
	for start := 0; start < len(candles); start += factor {
		end := min(start+factor, len(candles))
		bucket := candles[start]
		bucket.Volume = 0
		sentimentSum, sentimentCount := 0.0, 0
		for i := start; i < end; i++ {
			candle := candles[i]
			if candle.High > bucket.High {
				bucket.High = candle.High
			}
			if candle.Low < bucket.Low {
				bucket.Low = candle.Low
			}
			bucket.Close = candle.Close
			bucket.Volume += candle.Volume
			if candle.Sentiment != 0 {
				sentimentSum += candle.Sentiment
				sentimentCount++
			}
		}
		if sentimentCount > 0 {
			bucket.Sentiment = sentimentSum / float64(sentimentCount)
		}
		result = append(result, bucket)
	}
	return result
}

// attachSentiment merges the stored daily sentiment into the chart candles. Putting it
// on the same timestamps removes the need for a second series with different times,
// which used to punch gaps into the candle chart.
func attachSentiment(s *Server, userID string, interval string, candles []portfolioCandle) {
	if len(candles) == 0 {
		return
	}
	holdings, err := s.DB.GetHoldingsByUser(userID)
	if err != nil || len(holdings) == 0 {
		return
	}
	total := 0.0
	for _, holding := range holdings {
		total += holding.Quantity * holding.PurchasePrice
	}
	if total == 0 {
		return
	}
	weights := map[string]float64{}
	for _, holding := range holdings {
		weights[holding.Ticker] += holding.Quantity * holding.PurchasePrice / total
	}
	start := time.Unix(candles[0].Timestamp, 0).UTC().Format("2006-01-02")
	end := time.Unix(candles[len(candles)-1].Timestamp, 0).UTC().Format("2006-01-02")
	daily := map[int64][2]float64{}
	bucket := intervalBucketFor(interval)
	for ticker, weight := range weights {
		history, err := s.DB.DailySentimentBetween(ticker, start, end)
		if err != nil {
			continue
		}
		for _, row := range history {
			parsed, err := time.Parse("2006-01-02", row.Date)
			if err != nil {
				continue
			}
			key := bucket.start(parsed.Unix())
			entry := daily[key]
			entry[0] += row.Sentiment * weight
			entry[1] += weight
			daily[key] = entry
		}
	}
	for i := range candles {
		key := bucket.start(candles[i].Timestamp)
		if entry, ok := daily[key]; ok && entry[1] > 0 {
			candles[i].Sentiment = entry[0] / entry[1]
		}
	}
}

func (s *Server) getPortfolioChange(c echo.Context) error {
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]float64{})
	}
	target := c.QueryParam("currency")
	oneDayAgo := time.Now().UTC().Add(-24 * time.Hour).Unix()
	var current, previous, invested float64
	var tickers []string
	for _, holding := range holdings {
		tickers = append(tickers, holding.Ticker)
	}
	latest, _ := s.Prices.LatestCloses(tickers)
	for _, holding := range holdings {
		invested += s.toCurrency(holding.PurchasePrice*holding.Quantity, holding.Currency, target)
		price := holding.PurchasePrice
		if value, ok := latest[holding.Ticker]; ok {
			price = value
		}
		current += s.toCurrency(price*holding.Quantity, holding.Currency, target)
		previousPrice := price
		if value, err := s.Prices.LastCloseBefore(holding.Ticker, oneDayAgo); err == nil {
			previousPrice = value
		}
		previous += s.toCurrency(previousPrice*holding.Quantity, holding.Currency, target)
	}
	dayChange := current - previous
	totalChange := current - invested
	percent := func(value, base float64) float64 {
		if base == 0 {
			return 0
		}
		return value / base * 100
	}
	return c.JSON(http.StatusOK, map[string]float64{
		"current_value": current, "day_change": dayChange, "day_change_percent": percent(dayChange, previous),
		"total_change": totalChange, "total_change_percent": percent(totalChange, invested), "total_invested": invested,
	})
}

func (s *Server) portfolioTickers(userID string) ([]string, []store.Holding, error) {
	holdings, err := s.DB.GetHoldingsByUser(userID)
	if err != nil {
		return nil, nil, err
	}
	set := map[string]bool{}
	for _, holding := range holdings {
		set[holding.Ticker] = true
		if holding.ETF {
			for _, asset := range s.underlyingTickers(holding) {
				set[asset] = true
			}
		}
	}
	tickers := make([]string, 0, len(set))
	for ticker := range set {
		tickers = append(tickers, ticker)
	}
	sort.Strings(tickers)
	return tickers, holdings, nil
}

func (s *Server) underlyingTickers(holding store.Holding) []string {
	assets, err := s.DB.GetAssetsForHolding(holding.IDHolding)
	if err != nil {
		return nil
	}
	tickers := make([]string, 0, len(assets))
	for _, asset := range assets {
		if asset.Ticker != "" {
			tickers = append(tickers, asset.Ticker)
		}
	}
	return tickers
}

func (s *Server) getPortfolioNews(c echo.Context) error {
	limit := queryInt(c, "limit", 10)
	offset := queryOffset(c)
	tickers, holdings, err := s.portfolioTickers(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, []store.News{})
	}
	news, err := s.DB.NewsForTickers(tickers, limit, offset)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving news")
	}
	return c.JSON(http.StatusOK, news)
}

func (s *Server) getPortfolioSentiment(c echo.Context) error {
	tickers, holdings, err := s.portfolioTickers(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]any{"average_sentiment": 0, "news_count": 0})
	}
	average, count, err := s.DB.AverageNewsSentiment(tickers, startOfToday(), startOfToday()+86400)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving sentiment")
	}
	return c.JSON(http.StatusOK, map[string]any{"average_sentiment": average, "news_count": count})
}

func (s *Server) getPortfolioDailySentiment(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		return c.String(http.StatusBadRequest, "date parameter is required")
	}
	sentiment, err := s.DB.GetPortfolioSentiment(s.userID(c), date)
	if err != nil {
		return c.JSON(http.StatusOK, nil)
	}
	return c.JSON(http.StatusOK, sentiment)
}

func (s *Server) getPortfolioAllocation(c echo.Context) error {
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]any{"total_value": 0, "sectors": []nameWeight{}, "regions": []nameWeight{}, "companies": []nameWeight{}})
	}
	var tickers []string
	for _, holding := range holdings {
		tickers = append(tickers, holding.Ticker)
	}
	latest, _ := s.Prices.LatestCloses(tickers)

	totalValue := 0.0
	values := map[string]float64{}
	for _, holding := range holdings {
		price := holding.PurchasePrice
		if value, ok := latest[holding.Ticker]; ok {
			price = value
		}
		values[holding.IDHolding] = price * holding.Quantity
		totalValue += values[holding.IDHolding]
	}

	sectorTotals := map[string]float64{}
	regionTotals := map[string]float64{}
	companyTotals := map[string]float64{}
	for _, holding := range holdings {
		weight := 0.0
		if totalValue > 0 {
			weight = values[holding.IDHolding] / totalValue
		}
		sectors, regions, err := s.DB.GetBreakdownForHolding(holding.IDHolding)
		if err == nil {
			for _, sector := range sectors {
				sectorTotals[sector.Name] += sector.Percentage * weight
			}
			for _, region := range regions {
				regionTotals[region.Name] += region.Percentage * weight
			}
		}
		if !holding.ETF {
			sectorTotals["Individual Stocks"] += weight * 100
			companyTotals[holding.Name] += weight * 100
			continue
		}
		assets, err := s.DB.GetAssetsForHolding(holding.IDHolding)
		if err != nil || len(assets) == 0 {
			continue
		}
		limit := min(len(assets), 10)
		totalDecay := 0.0
		for i := 0; i < limit; i++ {
			totalDecay += math.Pow(0.9, float64(i))
		}
		covered := 0.0
		for i := 0; i < limit; i++ {
			share := math.Pow(0.9, float64(i)) / totalDecay * 100
			companyTotals[assets[i].Name] += share * weight
			covered += share
		}
		if covered < 100 {
			companyTotals["Other"] += (100 - covered) * weight
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"total_value": totalValue,
		"sectors":     toAllocation(sectorTotals),
		"regions":     toAllocation(regionTotals),
		"companies":   toAllocation(companyTotals),
	})
}

func toAllocation(values map[string]float64) []nameWeight {
	result := make([]nameWeight, 0, len(values))
	for name, percentage := range values {
		if percentage <= 0 {
			continue
		}
		result = append(result, nameWeight{Name: name, Percentage: analytics.Round(percentage, 2)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Percentage > result[j].Percentage })
	return result
}

type portfolioStats struct {
	YoYReturn        float64 `json:"yoy_return"`
	MaxDrawdown      float64 `json:"max_drawdown"`
	AvgDrawdown      float64 `json:"avg_drawdown"`
	SortinoRatio     float64 `json:"sortino_ratio"`
	AggregatedTER    float64 `json:"aggregated_ter"`
	TotalValue       float64 `json:"total_value"`
	TotalCost        float64 `json:"total_cost"`
	TotalGainLoss    float64 `json:"total_gain_loss"`
	TotalGainLossPct float64 `json:"total_gain_loss_pct"`
}

func (s *Server) getPortfolioStats(c echo.Context) error {
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, portfolioStats{})
	}
	quantities := map[string]float64{}
	byTicker := map[string]store.Holding{}
	var tickers []string
	for _, holding := range holdings {
		if _, ok := quantities[holding.Ticker]; !ok {
			tickers = append(tickers, holding.Ticker)
		}
		quantities[holding.Ticker] += holding.Quantity
		byTicker[holding.Ticker] = holding
	}

	now := time.Now().UTC()
	series, err := s.Prices.CandlesMulti(tickers, now.AddDate(-1, 0, 0).Unix(), now.Unix(), "1d")
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}

	dayValues := map[int64]map[string]float64{}
	for ticker, candles := range series {
		for _, candle := range candles {
			if dayValues[candle.Timestamp] == nil {
				dayValues[candle.Timestamp] = map[string]float64{}
			}
			dayValues[candle.Timestamp][ticker] = candle.Close
		}
	}
	days := make([]int64, 0, len(dayValues))
	for day := range dayValues {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })

	lastPrices := map[string]float64{}
	var values []float64
	var timestamps []int64
	for _, day := range days {
		for ticker, price := range dayValues[day] {
			lastPrices[ticker] = price
		}
		value, complete := 0.0, true
		for ticker, quantity := range quantities {
			price, ok := lastPrices[ticker]
			if !ok {
				complete = false
				break
			}
			value += price * quantity
		}
		if complete && value > 0 {
			values = append(values, value)
			timestamps = append(timestamps, day)
		}
	}

	returns := analytics.DailyReturns(values)
	maxDD, avgDD := analytics.Drawdowns(values)
	target := c.QueryParam("currency")
	totalValue, totalCost, weightedTER := 0.0, 0.0, 0.0
	for ticker, quantity := range quantities {
		holding := byTicker[ticker]
		price := lastPrices[ticker]
		if price == 0 {
			price = holding.PurchasePrice
		}
		value := s.toCurrency(price*quantity, holding.Currency, target)
		totalValue += value
		totalCost += s.toCurrency(holding.PurchasePrice*quantity, holding.Currency, target)
		weightedTER += holding.TER / 100 * value
	}
	aggregatedTER := 0.0
	if totalValue > 0 {
		aggregatedTER = weightedTER / totalValue
	}
	gain := totalValue - totalCost
	return c.JSON(http.StatusOK, portfolioStats{
		YoYReturn:        analytics.Round(analytics.YoYReturn(values, timestamps, now), 2),
		MaxDrawdown:      analytics.Round(maxDD, 2),
		AvgDrawdown:      analytics.Round(avgDD, 2),
		SortinoRatio:     analytics.Round(analytics.Sortino(returns, 0), 2),
		AggregatedTER:    analytics.Round(aggregatedTER, 4),
		TotalValue:       analytics.Round(totalValue, 2),
		TotalCost:        analytics.Round(totalCost, 2),
		TotalGainLoss:    analytics.Round(gain, 2),
		TotalGainLossPct: analytics.Round(percentOf(gain, totalCost), 2),
	})
}

func percentOf(value float64, base float64) float64 {
	if base == 0 {
		return 0
	}
	return value / base * 100
}

func (s *Server) getBacktest(c echo.Context) error {
	startDate := c.QueryParam("start_date")
	endDate := c.QueryParam("end_date")
	if startDate == "" || endDate == "" {
		return c.String(http.StatusBadRequest, "start_date and end_date are required")
	}
	benchmark := c.QueryParam("benchmark")
	if benchmark == "" {
		benchmark = "SPY"
	}
	var filter []string
	for _, ticker := range strings.Split(c.QueryParam("tickers"), ",") {
		if ticker = strings.TrimSpace(ticker); ticker != "" {
			filter = append(filter, ticker)
		}
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return c.String(http.StatusBadRequest, "invalid start date")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return c.String(http.StatusBadRequest, "invalid end date")
	}

	benchmarkSeries, err := s.dailySeries(benchmark, start, end)
	if err != nil || benchmarkSeries.Len() == 0 {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to create backtest: no price data for benchmark %s", benchmark))
	}

	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to fetch holdings")
	}
	if len(filter) > 0 {
		allowed := map[string]bool{}
		for _, ticker := range filter {
			allowed[ticker] = true
		}
		kept := holdings[:0]
		for _, holding := range holdings {
			if allowed[holding.Ticker] {
				kept = append(kept, holding)
			}
		}
		holdings = kept
	}
	if len(holdings) == 0 {
		return c.String(http.StatusInternalServerError, "Failed to create backtest: no holdings found for user")
	}

	priceMap := map[string]analytics.Series{}
	for _, holding := range holdings {
		if _, done := priceMap[holding.Ticker]; done {
			continue
		}
		series, err := s.dailySeries(holding.Ticker, start, end)
		if err != nil || series.Len() == 0 {
			log.Printf("api: no prices for %s: %v", holding.Ticker, err)
			continue
		}
		priceMap[holding.Ticker] = series
	}

	result, err := analytics.Backtest(analytics.Input{
		Start: start, End: end, Benchmark: benchmarkSeries, Holdings: holdings, Prices: priceMap,
	})
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to create backtest: "+err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

// dailySeries loads daily bars, fetching and storing them when the store has none.
func (s *Server) dailySeries(ticker string, start time.Time, end time.Time) (analytics.Series, error) {
	candles, err := s.Prices.Candles(ticker, start.Unix(), end.Unix(), "1d")
	if err == nil && len(candles) > 0 {
		return toSeries(candles), nil
	}
	if err := s.Jobs.BackfillHistory(ticker); err != nil {
		return analytics.Series{}, err
	}
	candles, err = s.Prices.Candles(ticker, start.Unix(), end.Unix(), "1d")
	if err != nil {
		return analytics.Series{}, err
	}
	return toSeries(candles), nil
}

func toSeries(candles []pricesCandle) analytics.Series {
	series := analytics.Series{Dates: make([]int64, len(candles)), Closes: make([]float64, len(candles))}
	for i, candle := range candles {
		series.Dates[i] = candle.Timestamp
		series.Closes[i] = candle.Close
	}
	return series
}

func (s *Server) topGainers(c echo.Context) error {
	return s.topMovers(c, true)
}

func (s *Server) topLosers(c echo.Context) error {
	return s.topMovers(c, false)
}

func (s *Server) topMovers(c echo.Context, gainers bool) error {
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour).Unix()
	newsStart := now.Add(-24 * time.Hour).Format("2006-01-02")
	results := make([]store.GainerLoser, 0, len(holdings))
	for _, holding := range holdings {
		closes, err := s.Prices.CloseSeries(holding.Ticker, start, now.Unix())
		if err != nil || len(closes) < 2 || closes[0] == 0 {
			continue
		}
		change := (closes[len(closes)-1] - closes[0]) / closes[0] * 100
		assets, _ := s.DB.GetAssetsForHolding(holding.IDHolding)
		news, _ := s.DB.NewsBetween(holding.Ticker, newsStart, now.Format("2006-01-02"))
		entry := store.GainerLoser{Holding: &holding, PriceChangePct: change, RelatedNews: news}
		if len(assets) > 0 {
			entry.Asset = &assets[0]
		}
		results = append(results, entry)
	}
	sort.Slice(results, func(i, j int) bool {
		if gainers {
			return results[i].PriceChangePct > results[j].PriceChangePct
		}
		return results[i].PriceChangePct < results[j].PriceChangePct
	})
	if len(results) > 5 {
		results = results[:5]
	}
	return c.JSON(http.StatusOK, results)
}

func (s *Server) getSentimentHistory(c echo.Context) error {
	interval := c.QueryParam("interval")
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil || len(holdings) == 0 {
		return c.JSON(http.StatusOK, []map[string]any{})
	}
	total := 0.0
	for _, holding := range holdings {
		total += holding.Quantity * holding.PurchasePrice
	}
	weights := map[string]float64{}
	for _, holding := range holdings {
		if total > 0 {
			weights[holding.Ticker] += holding.Quantity * holding.PurchasePrice / total
		}
	}
	start := time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01-02")
	end := time.Now().UTC().Format("2006-01-02")
	bucket := intervalBucketFor(interval)
	points := map[int64][2]float64{}
	for ticker, weight := range weights {
		history, err := s.DB.DailySentimentBetween(ticker, start, end)
		if err != nil {
			continue
		}
		for _, row := range history {
			parsed, err := time.Parse("2006-01-02", row.Date)
			if err != nil {
				continue
			}
			key := bucket.start(parsed.Unix())
			entry := points[key]
			entry[0] += row.Sentiment * weight
			entry[1] += weight
			points[key] = entry
		}
	}
	keys := make([]int64, 0, len(points))
	for key := range points {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	result := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		if points[key][1] == 0 {
			continue
		}
		result = append(result, map[string]any{"timestamp": key, "sentiment": points[key][0] / points[key][1]})
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) generateNewsSummary(c echo.Context) error {
	userID := s.userID(c)
	if ok, retry := s.allow("portfolio-summary:"+userID, 30*time.Minute); !ok {
		return c.JSON(http.StatusTooManyRequests, map[string]any{"error": "Please wait before generating another summary", "retry_after_min": retry/60 + 1})
	}
	date := store.Today()
	if err := s.Jobs.RegeneratePortfolioSentiment(userID, date); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate summary"})
	}
	sentiment, err := s.DB.GetPortfolioSentiment(userID, date)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load summary"})
	}
	return c.JSON(http.StatusOK, sentiment)
}

func (s *Server) generateHoldingSummary(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}
	ticker := s.resolveTicker(identifier)
	if ok, retry := s.allow("ticker-summary:"+ticker, 15*time.Minute); !ok {
		return c.JSON(http.StatusTooManyRequests, map[string]any{"error": "Please wait before generating another summary", "retry_after_min": retry/60 + 1})
	}
	if err := s.Jobs.RegenerateTickerSentiment(ticker); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate summary"})
	}
	sentiment, err := s.DB.GetDailySentiment(ticker, store.Today())
	if err != nil {
		return c.JSON(http.StatusOK, nil)
	}
	return c.JSON(http.StatusOK, sentiment)
}

func (s *Server) generateRunningSummary(c echo.Context) error {
	userID := s.userID(c)
	windowDays := queryInt(c, "window_days", 30)
	if ok, retry := s.allow(fmt.Sprintf("running:%s:%d", userID, windowDays), 30*time.Minute); !ok {
		return c.JSON(http.StatusTooManyRequests, map[string]any{"error": "Please wait before generating another running summary", "retry_after_min": retry/60 + 1})
	}
	if err := s.Jobs.UpdateRunningSummary(userID, windowDays); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate running summary"})
	}
	summary, err := s.DB.LatestRunningSummary(userID, windowDays)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load running summary"})
	}
	return c.JSON(http.StatusOK, runningSummaryResponse(*summary))
}

func (s *Server) getRunningSummary(c echo.Context) error {
	windowDays := queryInt(c, "window_days", 30)
	summary, err := s.DB.LatestRunningSummary(s.userID(c), windowDays)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "No running summary available. Generate one first."})
	}
	return c.JSON(http.StatusOK, runningSummaryResponse(*summary))
}

func (s *Server) getRunningSummaryHistory(c echo.Context) error {
	windowDays := queryInt(c, "window_days", 30)
	limit := queryInt(c, "limit", 90)
	if limit > 365 {
		limit = 365
	}
	history, err := s.DB.RunningSummaryHistory(s.userID(c), windowDays, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch running summary history"})
	}
	result := make([]map[string]any, 0, len(history))
	for _, summary := range history {
		result = append(result, runningSummaryResponse(summary))
	}
	return c.JSON(http.StatusOK, result)
}

func runningSummaryResponse(summary store.RunningSummary) map[string]any {
	var sectors, themes []map[string]any
	if summary.SectorPredictions != "" {
		_ = jsonUnmarshal([]byte(summary.SectorPredictions), &sectors)
	}
	if summary.ThemePredictions != "" {
		_ = jsonUnmarshal([]byte(summary.ThemePredictions), &themes)
	}
	return map[string]any{
		"id": summary.ID, "user_id": summary.UserID, "window_days": summary.WindowDays, "date": summary.Date,
		"summary": summary.Summary, "sentiment": summary.Sentiment,
		"sector_predictions": sectors, "theme_predictions": themes,
	}
}
