package api

import (
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"

	"portfolio/internal/analytics"
	"portfolio/internal/store"
)

type drawdownPoint struct {
	Time     int64   `json:"time"`
	Drawdown float64 `json:"drawdown"`
}

type monthReturn struct {
	Month  string  `json:"month"`
	Return float64 `json:"return"`
}

type exposureRow struct {
	Ticker       string  `json:"ticker"`
	Name         string  `json:"name"`
	Weight       float64 `json:"weight"`
	Return       float64 `json:"return"`
	Contribution float64 `json:"contribution"`
}

type riskResponse struct {
	Period          string          `json:"period"`
	Start           string          `json:"start"`
	End             string          `json:"end"`
	Observations    int             `json:"observations"`
	TotalReturn     float64         `json:"total_return"`
	Cagr            float64         `json:"cagr"`
	Volatility      float64         `json:"volatility"`
	Sharpe          float64         `json:"sharpe"`
	Sortino         float64         `json:"sortino"`
	Calmar          float64         `json:"calmar"`
	MaxDrawdown     float64         `json:"max_drawdown"`
	CurrentDrawdown float64         `json:"current_drawdown"`
	WinRate         float64         `json:"win_rate"`
	BestDay         float64         `json:"best_day"`
	WorstDay        float64         `json:"worst_day"`
	Var95           float64         `json:"var_95"`
	Beta            float64         `json:"beta"`
	Alpha           float64         `json:"alpha"`
	Correlation     float64         `json:"correlation"`
	Benchmark       string          `json:"benchmark"`
	BenchmarkReturn float64         `json:"benchmark_return"`
	DrawdownSeries  []drawdownPoint `json:"drawdown_series"`
	Monthly         []monthReturn   `json:"monthly"`
	Exposure        []exposureRow   `json:"exposure"`
}

// getPortfolioRisk returns the risk profile of the whole portfolio. It shares the daily
// value series with the stats endpoint so both views always agree.
func (s *Server) getPortfolioRisk(c echo.Context) error {
	holdings, err := s.DB.GetHoldingsByUser(s.userID(c))
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, riskResponse{DrawdownSeries: []drawdownPoint{}, Monthly: []monthReturn{}, Exposure: []exposureRow{}})
	}

	period := defaultQuery(c, "period", "1y")
	days := lookbackDays(period)
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -days)
	timestamps, values := s.portfolioDailyValues(holdings, start)
	if len(values) < 2 {
		return c.JSON(http.StatusOK, riskResponse{Period: period})
	}

	returns := analytics.DailyReturns(values)
	years := math.Max(float64(timestamps[len(timestamps)-1]-timestamps[0])/86400/365.25, 1.0/365.25)
	totalReturn := (values[len(values)-1]/values[0] - 1) * 100
	cagr := (math.Pow(values[len(values)-1]/values[0], 1/years) - 1) * 100
	maxDD, _ := analytics.Drawdowns(values)

	drawdownTimes, drawdowns := analytics.DrawdownSeries(timestamps, values)
	series := make([]drawdownPoint, 0, len(drawdownTimes))
	for i := range drawdownTimes {
		series = append(series, drawdownPoint{Time: drawdownTimes[i], Drawdown: analytics.Round(drawdowns[i], 2)})
	}

	monthly := make([]monthReturn, 0, 36)
	for _, month := range analytics.MonthlyReturns(timestamps, values) {
		monthly = append(monthly, monthReturn{Month: month.Label, Return: analytics.Round(month.Return, 2)})
	}

	benchmark := defaultQuery(c, "benchmark", "SPY")
	beta, alpha, correlation, benchmarkReturn := s.benchmarkRisk(benchmark, timestamps, returns)

	response := riskResponse{
		Period:          period,
		Start:           time.Unix(timestamps[0], 0).UTC().Format("2006-01-02"),
		End:             time.Unix(timestamps[len(timestamps)-1], 0).UTC().Format("2006-01-02"),
		Observations:    len(values),
		TotalReturn:     analytics.Round(totalReturn, 2),
		Cagr:            analytics.Round(cagr, 2),
		Volatility:      analytics.Round(analytics.AnnualizedVolatility(returns), 2),
		Sharpe:          analytics.Round(analytics.Sharpe(returns, 0), 2),
		Sortino:         analytics.Round(analytics.Sortino(returns, 0), 2),
		Calmar:          analytics.Round(analytics.Calmar(cagr, maxDD), 2),
		MaxDrawdown:     analytics.Round(maxDD, 2),
		CurrentDrawdown: analytics.Round(drawdowns[len(drawdowns)-1], 2),
		WinRate:         analytics.Round(analytics.WinRate(returns), 1),
		BestDay:         analytics.Round(analytics.Percentile(returns, 1)*100, 2),
		WorstDay:        analytics.Round(analytics.Percentile(returns, 0)*100, 2),
		Var95:           analytics.Round(analytics.Percentile(returns, 0.05)*100, 2),
		Beta:            analytics.Round(beta, 2),
		Alpha:           analytics.Round(alpha, 2),
		Correlation:     analytics.Round(correlation, 2),
		Benchmark:       benchmark,
		BenchmarkReturn: analytics.Round(benchmarkReturn, 2),
		DrawdownSeries:  series,
		Monthly:         monthly,
		Exposure:        s.exposureRows(holdings, start, now),
	}
	return c.JSON(http.StatusOK, response)
}

func lookbackDays(period string) int {
	switch period {
	case "1m":
		return 31
	case "3m":
		return 92
	case "6m":
		return 183
	case "2y":
		return 731
	case "3y":
		return 1096
	case "5y":
		return 1827
	case "max":
		return 3650
	default:
		return 366
	}
}

// benchmarkRisk aligns the benchmark closes with the portfolio days before comparing
// returns, so a missing benchmark day cannot silently shift the whole series.
func (s *Server) benchmarkRisk(benchmark string, timestamps []int64, portfolioReturns []float64) (float64, float64, float64, float64) {
	series, err := s.Prices.Candles(benchmark, timestamps[0], timestamps[len(timestamps)-1], "1d")
	if err != nil || len(series) == 0 {
		return 0, 0, 0, 0
	}
	byDay := make(map[int64]float64, len(series))
	for _, candle := range series {
		byDay[candle.Timestamp] = candle.Close
	}
	var benchValues []float64
	for _, day := range timestamps {
		if closePrice, ok := byDay[day]; ok && closePrice > 0 {
			benchValues = append(benchValues, closePrice)
		}
	}
	if len(benchValues) < 3 {
		return 0, 0, 0, 0
	}
	benchReturns := analytics.DailyReturns(benchValues)
	// Trim to the common tail so both return series cover the same window.
	portfolioReturns = portfolioReturns[len(portfolioReturns)-min(len(portfolioReturns), len(benchReturns)):]
	benchReturns = benchReturns[len(benchReturns)-min(len(portfolioReturns), len(benchReturns)):]
	alpha, beta := analytics.AlphaBeta(portfolioReturns, benchReturns)
	benchmarkReturn := (benchValues[len(benchValues)-1]/benchValues[0] - 1) * 100
	return beta, alpha, analytics.Correlation(portfolioReturns, benchReturns) * 100, benchmarkReturn
}

// exposureRows reports how much each position drives the period return.
func (s *Server) exposureRows(holdings []store.Holding, start time.Time, end time.Time) []exposureRow {
	tickers := make([]string, 0, len(holdings))
	quantities := map[string]float64{}
	names := map[string]string{}
	for _, holding := range holdings {
		if _, seen := quantities[holding.Ticker]; !seen {
			tickers = append(tickers, holding.Ticker)
			names[holding.Ticker] = holding.Name
		}
		quantities[holding.Ticker] += holding.Quantity
	}
	latest, _ := s.Prices.LatestCloses(tickers)
	total := 0.0
	for _, ticker := range tickers {
		price := latest[ticker]
		if price == 0 {
			continue
		}
		total += price * quantities[ticker]
	}
	rows := make([]exposureRow, 0, len(tickers))
	if total == 0 {
		return rows
	}
	for _, ticker := range tickers {
		price := latest[ticker]
		if price == 0 {
			continue
		}
		weight := price * quantities[ticker] / total * 100
		periodReturn := 0.0
		if candles, err := s.Prices.Candles(ticker, start.Unix(), end.Unix(), "1d"); err == nil && len(candles) > 1 && candles[0].Close > 0 {
			periodReturn = (candles[len(candles)-1].Close/candles[0].Close - 1) * 100
		}
		rows = append(rows, exposureRow{Ticker: ticker, Name: names[ticker],
			Weight: analytics.Round(weight, 2), Return: analytics.Round(periodReturn, 2),
			Contribution: analytics.Round(weight*periodReturn/100, 2)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Weight > rows[j].Weight })
	return rows
}

// portfolioDailyValues builds the quantity weighted value of the portfolio per day.
func (s *Server) portfolioDailyValues(holdings []store.Holding, start time.Time) ([]int64, []float64) {
	quantities := map[string]float64{}
	var tickers []string
	for _, holding := range holdings {
		if _, seen := quantities[holding.Ticker]; !seen {
			tickers = append(tickers, holding.Ticker)
		}
		quantities[holding.Ticker] += holding.Quantity
	}
	series, err := s.Prices.CandlesMulti(tickers, start.Unix(), time.Now().UTC().Unix(), "1d")
	if err != nil {
		return nil, nil
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
	var timestamps []int64
	var values []float64
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
			timestamps = append(timestamps, day)
			values = append(values, value)
		}
	}
	return timestamps, values
}
