package prices

import "testing"

func candles(closes ...float64) []Candle {
	result := make([]Candle, len(closes))
	for i, closePrice := range closes {
		result[i] = Candle{Timestamp: int64(i * 60), Open: closePrice, High: closePrice + 1, Low: closePrice - 1,
			Close: closePrice, Volume: 10}
	}
	return result
}

func TestDownsampleKeepsExtremes(t *testing.T) {
	input := candles(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	output := Downsample(input, 3)
	if len(output) != 3 {
		t.Fatalf("expected 3 points, got %d", len(output))
	}
	if output[0].Open != 1 {
		t.Errorf("first open must be preserved, got %v", output[0].Open)
	}
	if output[len(output)-1].Close != 10 {
		t.Errorf("last close must be preserved, got %v", output[len(output)-1].Close)
	}
	highest := 0.0
	for _, candle := range output {
		if candle.High > highest {
			highest = candle.High
		}
	}
	if highest != 11 {
		t.Errorf("expected high 11, got %v", highest)
	}
	total := int64(0)
	for _, candle := range output {
		total += candle.Volume
	}
	if total != 100 {
		t.Errorf("volume must be summed, got %d", total)
	}
}

func TestDownsampleBelowLimit(t *testing.T) {
	input := candles(1, 2, 3)
	if output := Downsample(input, 10); len(output) != 3 {
		t.Errorf("expected unchanged series, got %d points", len(output))
	}
}

func TestIntervalLookback(t *testing.T) {
	cases := map[string]int64{
		"5m": 14 * 86400,
		"1h": 120 * 86400,
		"1d": 3650 * 86400,
		"1M": 0,
	}
	for interval, expected := range cases {
		if actual := IntervalLookback(interval, 14); actual != expected {
			t.Errorf("%s: expected %d, got %d", interval, expected, actual)
		}
	}
}

func TestBucketForRejectsUnknown(t *testing.T) {
	if _, ok := bucketFor("2m"); ok {
		t.Error("2m must not be supported")
	}
	if _, ok := bucketFor("1M"); !ok {
		t.Error("1M must be supported")
	}
}

func BenchmarkDownsample(b *testing.B) {
	input := make([]Candle, 10000)
	for i := range input {
		input[i] = Candle{Timestamp: int64(i), Open: 100, High: 101, Low: 99, Close: 100, Volume: 5}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if len(Downsample(input, 500)) != 500 {
			b.Fatal("unexpected downsample result")
		}
	}
}
