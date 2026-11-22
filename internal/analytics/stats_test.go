package analytics

import (
	"math"
	"testing"
	"time"

	"portfolio/internal/store"
)

func almostEqual(a, b float64, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestDrawdowns(t *testing.T) {
	maxDD, avgDD := Drawdowns([]float64{100, 120, 90, 110, 80, 100})
	if !almostEqual(maxDD, 33.3333, 0.001) {
		t.Errorf("expected max drawdown 33.33, got %v", maxDD)
	}
	if avgDD <= 0 {
		t.Errorf("expected a positive average drawdown, got %v", avgDD)
	}
}

func TestDrawdownsShortSeries(t *testing.T) {
	if maxDD, avgDD := Drawdowns([]float64{100}); maxDD != 0 || avgDD != 0 {
		t.Error("a single value has no drawdown")
	}
}

func TestSortinoIgnoresUpside(t *testing.T) {
	upsideOnly := []float64{0.01, 0.02, 0.03}
	if value := Sortino(upsideOnly, 0); value != 0 {
		t.Errorf("no downside risk must give 0, got %v", value)
	}
	mixed := []float64{0.01, -0.02, 0.03, -0.01}
	if value := Sortino(mixed, 0); value <= 0 {
		t.Errorf("expected a positive ratio, got %v", value)
	}
}

func TestAlphaBetaIdenticalSeries(t *testing.T) {
	returns := []float64{0.01, -0.02, 0.03, 0.005}
	alpha, beta := AlphaBeta(returns, returns)
	if !almostEqual(alpha, 0, 1e-9) || !almostEqual(beta, 1, 1e-9) {
		t.Errorf("identical series must give alpha 0 beta 1, got %v %v", alpha, beta)
	}
}

func TestWinRate(t *testing.T) {
	if value := WinRate([]float64{0.1, -0.1, 0.2, 0.05}); !almostEqual(value, 75, 0.001) {
		t.Errorf("expected 75, got %v", value)
	}
}

func TestYoYReturn(t *testing.T) {
	now := time.Now().UTC()
	timestamps := []int64{now.AddDate(-1, 0, -1).Unix(), now.Unix()}
	value := YoYReturn([]float64{100, 120}, timestamps, now)
	if !almostEqual(value, 20, 0.001) {
		t.Errorf("expected 20 percent, got %v", value)
	}
}

func TestMonthlyExtremes(t *testing.T) {
	january := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	february := time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC).Unix()
	best, bestLabel, worst, worstLabel := MonthlyExtremes(
		[]int64{january, january + 86400, february, february + 86400},
		[]float64{100, 110, 120, 108},
	)
	if !almostEqual(best, 10, 0.001) || bestLabel != "2025-01" {
		t.Errorf("unexpected best month %v %s", best, bestLabel)
	}
	if !almostEqual(worst, -10, 0.001) || worstLabel != "2025-02" {
		t.Errorf("unexpected worst month %v %s", worst, worstLabel)
	}
}

func TestBacktestWeightedPortfolio(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 9)
	dates := make([]int64, 10)
	for i := range dates {
		dates[i] = start.AddDate(0, 0, i).Unix()
	}
	benchmark := Series{Dates: dates, Closes: []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 110}}
	prices := map[string]Series{
		"A": {Dates: dates, Closes: []float64{100, 102, 104, 106, 108, 110, 112, 114, 116, 120}},
		"B": {Dates: dates, Closes: []float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100}},
	}
	holdings := []store.Holding{
		{Ticker: "A", Name: "A", Quantity: 10, PurchasePrice: 100},
		{Ticker: "B", Name: "B", Quantity: 10, PurchasePrice: 100},
	}
	result, err := Backtest(Input{Start: start, End: end, Benchmark: benchmark, Holdings: holdings, Prices: prices})
	if err != nil {
		t.Fatalf("backtest failed: %v", err)
	}
	if len(result.PortfolioValues) != 10 {
		t.Fatalf("expected 10 points, got %d", len(result.PortfolioValues))
	}
	if result.PortfolioValues[0] != 100 {
		t.Errorf("series must start at 100, got %v", result.PortfolioValues[0])
	}
	if result.HoldingReturns["B"].TotalReturn != 0 {
		t.Errorf("flat holding must return 0, got %v", result.HoldingReturns["B"].TotalReturn)
	}
	if result.HoldingReturns["A"].TotalReturn != 20 {
		t.Errorf("expected 20 percent for A, got %v", result.HoldingReturns["A"].TotalReturn)
	}
}

func BenchmarkBacktest(b *testing.B) {
	const days = 2500
	start := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	dates := make([]int64, days)
	closes := make([]float64, days)
	for i := 0; i < days; i++ {
		dates[i] = start.AddDate(0, 0, i).Unix()
		closes[i] = 100 + float64(i)*0.05
	}
	benchmark := Series{Dates: dates, Closes: closes}
	prices := map[string]Series{}
	var holdings []store.Holding
	for i := 0; i < 8; i++ {
		ticker := string(rune('A' + i))
		series := make([]float64, days)
		for d := range series {
			series[d] = 100 + float64(d)*(0.03+float64(i)*0.004)
		}
		prices[ticker] = Series{Dates: dates, Closes: series}
		holdings = append(holdings, store.Holding{Ticker: ticker, Name: ticker, Quantity: 10, PurchasePrice: 100})
	}
	input := Input{Start: start, End: start.AddDate(0, 0, days-1), Benchmark: benchmark, Holdings: holdings, Prices: prices}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Backtest(input); err != nil {
			b.Fatal(err)
		}
	}
}
