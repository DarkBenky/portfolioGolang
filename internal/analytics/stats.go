package analytics

import (
	"math"
	"time"
)

type Stat struct {
	YoYReturn    float64 `json:"yoy_return"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	AvgDrawdown  float64 `json:"avg_drawdown"`
	SortinoRatio float64 `json:"sortino_ratio"`
	Volatility   float64 `json:"volatility"`
	SharpeRatio  float64 `json:"sharpe_ratio"`
}

func Drawdowns(values []float64) (maxDD float64, avgDD float64) {
	if len(values) < 2 {
		return 0, 0
	}
	peak := values[0]
	sum, count := 0.0, 0
	for _, value := range values {
		if value > peak {
			peak = value
		}
		if peak <= 0 {
			continue
		}
		drawdown := (peak - value) / peak * 100
		if drawdown <= 0 {
			continue
		}
		sum += drawdown
		count++
		if drawdown > maxDD {
			maxDD = drawdown
		}
	}
	if count > 0 {
		avgDD = sum / float64(count)
	}
	return maxDD, avgDD
}

func DailyReturns(values []float64) []float64 {
	if len(values) < 2 {
		return nil
	}
	returns := make([]float64, 0, len(values)-1)
	for i := 1; i < len(values); i++ {
		if values[i-1] > 0 {
			returns = append(returns, (values[i]-values[i-1])/values[i-1])
		}
	}
	return returns
}

func Sortino(dailyReturns []float64, riskFree float64) float64 {
	if len(dailyReturns) < 2 {
		return 0
	}
	sum := 0.0
	for _, value := range dailyReturns {
		sum += value
	}
	average := sum / float64(len(dailyReturns))
	downsideSum, downsideCount := 0.0, 0
	for _, value := range dailyReturns {
		if value < riskFree {
			diff := value - riskFree
			downsideSum += diff * diff
			downsideCount++
		}
	}
	if downsideCount == 0 {
		return 0
	}
	deviation := math.Sqrt(downsideSum / float64(downsideCount))
	if deviation == 0 {
		return 0
	}
	return (average*252 - riskFree) / (deviation * math.Sqrt(252))
}

func Sharpe(dailyReturns []float64, riskFree float64) float64 {
	if len(dailyReturns) < 2 {
		return 0
	}
	sum := 0.0
	for _, value := range dailyReturns {
		sum += value
	}
	mean := sum / float64(len(dailyReturns))
	variance := 0.0
	for _, value := range dailyReturns {
		diff := value - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(dailyReturns)))
	if stdDev == 0 {
		return 0
	}
	return (mean*252 - riskFree) / (stdDev * math.Sqrt(252))
}

func AnnualizedVolatility(dailyReturns []float64) float64 {
	if len(dailyReturns) < 2 {
		return 0
	}
	sum := 0.0
	for _, value := range dailyReturns {
		sum += value
	}
	mean := sum / float64(len(dailyReturns))
	variance := 0.0
	for _, value := range dailyReturns {
		diff := value - mean
		variance += diff * diff
	}
	return math.Sqrt(variance/float64(len(dailyReturns))) * math.Sqrt(252) * 100
}

func Calmar(cagr float64, maxDrawdown float64) float64 {
	if maxDrawdown == 0 {
		return 0
	}
	return cagr / math.Abs(maxDrawdown)
}

func YoYReturn(prices []float64, timestamps []int64, now time.Time) float64 {
	if len(prices) < 2 {
		return 0
	}
	oneYearAgo := now.AddDate(-1, 0, 0).Unix()
	startIndex := 0
	for i, timestamp := range timestamps {
		if timestamp >= oneYearAgo {
			startIndex = i
			break
		}
	}
	// When the series only starts inside the last year there is no year old price,
	// so the first observation is the honest baseline.
	if startIndex >= len(prices)-1 {
		startIndex = 0
	}
	startPrice := prices[startIndex]
	if startPrice == 0 {
		return 0
	}
	return (prices[len(prices)-1] - startPrice) / startPrice * 100
}

func AlphaBeta(portfolioReturns []float64, benchmarkReturns []float64) (float64, float64) {
	n := min(len(portfolioReturns), len(benchmarkReturns))
	if n < 2 {
		return 0, 1
	}
	var sumP, sumB float64
	for i := 0; i < n; i++ {
		sumP += portfolioReturns[i]
		sumB += benchmarkReturns[i]
	}
	meanP, meanB := sumP/float64(n), sumB/float64(n)

	var covariance, variance float64
	for i := 0; i < n; i++ {
		diffP := portfolioReturns[i] - meanP
		diffB := benchmarkReturns[i] - meanB
		covariance += diffP * diffB
		variance += diffB * diffB
	}
	covariance /= float64(n)
	variance /= float64(n)
	if variance == 0 {
		return 0, 1
	}
	beta := covariance / variance
	alpha := (meanP - beta*meanB) * 252 * 100
	return alpha, beta
}

func WinRate(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	wins := 0
	for _, value := range returns {
		if value > 0 {
			wins++
		}
	}
	return float64(wins) / float64(len(returns)) * 100
}

func MonthlyExtremes(timestamps []int64, values []float64) (best float64, bestLabel string, worst float64, worstLabel string) {
	if len(timestamps) < 2 {
		return 0, "", 0, ""
	}
	type monthValue struct {
		label string
		first float64
		last  float64
	}
	months := map[string]*monthValue{}
	order := make([]string, 0, 48)
	for i, timestamp := range timestamps {
		label := time.Unix(timestamp, 0).UTC().Format("2006-01")
		entry, ok := months[label]
		if !ok {
			entry = &monthValue{label: label, first: values[i]}
			months[label] = entry
			order = append(order, label)
		}
		entry.last = values[i]
	}
	first := true
	for _, label := range order {
		entry := months[label]
		if entry.first == 0 {
			continue
		}
		change := (entry.last - entry.first) / entry.first * 100
		if first {
			best, worst, bestLabel, worstLabel = change, change, label, label
			first = false
			continue
		}
		if change > best {
			best, bestLabel = change, label
		}
		if change < worst {
			worst, worstLabel = change, label
		}
	}
	return best, bestLabel, worst, worstLabel
}

func Round(value float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(value*factor) / factor
}
