package analytics

import (
	"fmt"
	"math"
	"sort"
	"time"

	"portfolio/internal/store"
)

type Series struct {
	Dates  []int64
	Closes []float64
}

func (s Series) Len() int { return len(s.Dates) }

// Range returns the slice covering [start, end] with binary search, so large
// histories are not scanned linearly for every request.
func (s Series) Range(start, end int64) Series {
	from := sort.Search(len(s.Dates), func(i int) bool { return s.Dates[i] >= start })
	to := sort.Search(len(s.Dates), func(i int) bool { return s.Dates[i] > end })
	if from >= to {
		return Series{}
	}
	return Series{Dates: s.Dates[from:to], Closes: s.Closes[from:to]}
}

func (s Series) ValueAt(ts int64) float64 {
	if len(s.Dates) == 0 {
		return 0
	}
	index := sort.Search(len(s.Dates), func(i int) bool { return s.Dates[i] > ts })
	if index == 0 {
		return 0
	}
	return s.Closes[index-1]
}

type HoldingReturn struct {
	Ticker             string  `json:"ticker"`
	Name               string  `json:"name"`
	ISIN               string  `json:"isin"`
	TotalReturn        float64 `json:"total_return"`
	VsBenchmark        float64 `json:"vs_benchmark"`
	CurrentPrice       float64 `json:"current_price"`
	Weight             float64 `json:"weight"`
	Return1M           float64 `json:"return_1m"`
	Return3M           float64 `json:"return_3m"`
	DrawdownFromPeak   float64 `json:"drawdown_from_peak"`
	MeanReversionScore float64 `json:"mean_reversion_score"`
	Signal             string  `json:"signal"`
}

type Result struct {
	PortfolioValues     []float64                `json:"portfolio_values"`
	BenchmarkValues     []float64                `json:"benchmark_values"`
	Timestamps          []int64                  `json:"timestamps"`
	CAGRPortfolio       float64                  `json:"cagr_portfolio"`
	CAGRBenchmark       float64                  `json:"cagr_benchmark"`
	MaxDDPortfolio      float64                  `json:"max_dd_portfolio"`
	MaxDDBenchmark      float64                  `json:"max_dd_benchmark"`
	SharpePortfolio     float64                  `json:"sharpe_portfolio"`
	SharpeBenchmark     float64                  `json:"sharpe_benchmark"`
	SortinoPortfolio    float64                  `json:"sortino_portfolio"`
	SortinoBenchmark    float64                  `json:"sortino_benchmark"`
	VolatilityPortfolio float64                  `json:"volatility_portfolio"`
	VolatilityBenchmark float64                  `json:"volatility_benchmark"`
	CalmarPortfolio     float64                  `json:"calmar_portfolio"`
	CalmarBenchmark     float64                  `json:"calmar_benchmark"`
	Alpha               float64                  `json:"alpha"`
	Beta                float64                  `json:"beta"`
	WinRate             float64                  `json:"win_rate"`
	BestMonth           float64                  `json:"best_month"`
	WorstMonth          float64                  `json:"worst_month"`
	BestMonthLabel      string                   `json:"best_month_label"`
	WorstMonthLabel     string                   `json:"worst_month_label"`
	HoldingReturns      map[string]HoldingReturn `json:"holding_returns"`
}

type Input struct {
	Start     time.Time
	End       time.Time
	Benchmark Series
	Holdings  []store.Holding
	Prices    map[string]Series
}

func Backtest(input Input) (*Result, error) {
	startUnix, endUnix := input.Start.Unix(), input.End.Unix()
	benchmark := input.Benchmark.Range(startUnix, endUnix)
	if benchmark.Len() == 0 {
		return nil, fmt.Errorf("no benchmark data in date range")
	}

	type weightedHolding struct {
		holding    store.Holding
		series     Series
		startPrice float64
		weight     float64
	}
	var weighted []weightedHolding
	totalInitial := 0.0
	for _, holding := range input.Holdings {
		series, ok := input.Prices[holding.Ticker]
		if !ok {
			continue
		}
		window := series.Range(startUnix, endUnix)
		if window.Len() == 0 {
			continue
		}
		if window.Closes[0] > 0 {
			weighted = append(weighted, weightedHolding{holding: holding, series: window, startPrice: window.Closes[0]})
			totalInitial += window.Closes[0] * holding.Quantity
		}
	}
	if len(weighted) == 0 || totalInitial == 0 {
		return nil, fmt.Errorf("no holdings found for user")
	}
	for i := range weighted {
		weighted[i].weight = weighted[i].startPrice * weighted[i].holding.Quantity / totalInitial
	}

	portfolioValues := make([]float64, benchmark.Len())
	benchmarkValues := make([]float64, benchmark.Len())
	for i, timestamp := range benchmark.Dates {
		value := 0.0
		for _, entry := range weighted {
			price := entry.series.ValueAt(timestamp)
			if price > 0 {
				value += entry.weight * (price / entry.startPrice)
			}
		}
		portfolioValues[i] = value
		benchmarkValues[i] = benchmark.Closes[i]
	}

	if portfolioValues[0] == 0 || benchmarkValues[0] == 0 {
		return nil, fmt.Errorf("insufficient price data at range start")
	}
	for i := range portfolioValues {
		portfolioValues[i] = portfolioValues[i] / portfolioValues[0] * 100
		benchmarkValues[i] = benchmarkValues[i] / benchmarkValues[0] * 100
	}

	portfolioReturns := DailyReturns(portfolioValues)
	benchmarkReturns := DailyReturns(benchmarkValues)
	years := math.Max(input.End.Sub(input.Start).Hours()/(24*365.25), 1.0/365.25)
	lastPortfolio := portfolioValues[len(portfolioValues)-1]
	lastBenchmark := benchmarkValues[len(benchmarkValues)-1]
	cagrPortfolio := (math.Pow(lastPortfolio/100, 1/years) - 1) * 100
	cagrBenchmark := (math.Pow(lastBenchmark/100, 1/years) - 1) * 100

	maxDDPortfolio, _ := Drawdowns(portfolioValues)
	maxDDBenchmark, _ := Drawdowns(benchmarkValues)
	alpha, beta := AlphaBeta(portfolioReturns, benchmarkReturns)
	bestMonth, bestLabel, worstMonth, worstLabel := MonthlyExtremes(benchmark.Dates, portfolioValues)

	holdingReturns := make(map[string]HoldingReturn, len(weighted))
	totalEndValue := 0.0
	endPrices := make(map[string]float64, len(weighted))
	for _, entry := range weighted {
		endPrice := entry.series.Closes[len(entry.series.Closes)-1]
		endPrices[entry.holding.Ticker] = endPrice
		totalEndValue += endPrice * entry.holding.Quantity
	}
	benchmarkTotalReturn := lastBenchmark - 100

	for _, entry := range weighted {
		window := entry.series
		endPrice := endPrices[entry.holding.Ticker]
		if entry.startPrice == 0 || endPrice == 0 {
			continue
		}
		totalReturn := (endPrice - entry.startPrice) / entry.startPrice * 100
		weight := 0.0
		if totalEndValue > 0 {
			weight = endPrice * entry.holding.Quantity / totalEndValue * 100
		}
		n := window.Len()
		return1M, return3M, drawdownFromPeak := 0.0, 0.0, 0.0
		if n > 0 {
			peak := window.Closes[0]
			for _, closePrice := range window.Closes {
				if closePrice > peak {
					peak = closePrice
				}
			}
			if peak > 0 {
				drawdownFromPeak = (endPrice - peak) / peak * 100
			}
			if price := window.Closes[max(n-21, 0)]; price > 0 {
				return1M = (endPrice - price) / price * 100
			}
			if price := window.Closes[max(n-63, 0)]; price > 0 {
				return3M = (endPrice - price) / price * 100
			}
		}
		meanReversion := 0.0
		if n > 21 {
			averageDaily := (endPrice - entry.startPrice) / entry.startPrice / float64(n) * 100
			meanReversion = averageDaily*21 - return1M
		}
		signal := "NEUTRAL"
		switch {
		case totalReturn > 0 && meanReversion > 3 && drawdownFromPeak < -5:
			signal = "BUY_DIP"
		case return1M > 3 && return3M > 5 && totalReturn-benchmarkTotalReturn > 0:
			signal = "STRONG"
		case totalReturn < 0 && return1M < 0 && return3M < 0:
			signal = "WEAK"
		}
		holdingReturns[entry.holding.Ticker] = HoldingReturn{
			Ticker:             entry.holding.Ticker,
			Name:               entry.holding.Name,
			ISIN:               entry.holding.ISIN,
			TotalReturn:        Round(totalReturn, 2),
			VsBenchmark:        Round(totalReturn-benchmarkTotalReturn, 2),
			CurrentPrice:       Round(endPrice, 2),
			Weight:             Round(weight, 2),
			Return1M:           Round(return1M, 2),
			Return3M:           Round(return3M, 2),
			DrawdownFromPeak:   Round(drawdownFromPeak, 2),
			MeanReversionScore: Round(meanReversion, 2),
			Signal:             signal,
		}
	}

	return &Result{
		PortfolioValues:     portfolioValues,
		BenchmarkValues:     benchmarkValues,
		Timestamps:          benchmark.Dates,
		CAGRPortfolio:       Round(cagrPortfolio, 2),
		CAGRBenchmark:       Round(cagrBenchmark, 2),
		MaxDDPortfolio:      Round(maxDDPortfolio, 2),
		MaxDDBenchmark:      Round(maxDDBenchmark, 2),
		SharpePortfolio:     Round(Sharpe(portfolioReturns, 0), 2),
		SharpeBenchmark:     Round(Sharpe(benchmarkReturns, 0), 2),
		SortinoPortfolio:    Round(Sortino(portfolioReturns, 0), 2),
		SortinoBenchmark:    Round(Sortino(benchmarkReturns, 0), 2),
		VolatilityPortfolio: Round(AnnualizedVolatility(portfolioReturns), 2),
		VolatilityBenchmark: Round(AnnualizedVolatility(benchmarkReturns), 2),
		CalmarPortfolio:     Round(Calmar(cagrPortfolio, maxDDPortfolio), 2),
		CalmarBenchmark:     Round(Calmar(cagrBenchmark, maxDDBenchmark), 2),
		Alpha:               Round(alpha, 2),
		Beta:                Round(beta, 2),
		WinRate:             Round(WinRate(portfolioReturns), 2),
		BestMonth:           Round(bestMonth, 2),
		WorstMonth:          Round(worstMonth, 2),
		BestMonthLabel:      bestLabel,
		WorstMonthLabel:     worstLabel,
		HoldingReturns:      holdingReturns,
	}, nil
}
