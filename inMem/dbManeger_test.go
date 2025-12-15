package inmem

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func generateRandomPrice(ticker string, date uint32) Price {
	open := float32(rand.Intn(1000) + 100)
	return Price{
		Ticker: ticker,
		Date:   date,
		Open:   open,
		Close:  open + float32(rand.Intn(20)-10),
		High:   open + float32(rand.Intn(30)),
		Low:    open - float32(rand.Intn(30)),
		Volume: int64(rand.Intn(1000000) + 10000),
	}
}

func TestBasicOperations(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_snapshot.gob", false)

	ticker := "AAPL"
	date := uint32(1700000000)

	price := Price{
		Ticker: ticker,
		Date:   date,
		Open:   150.0,
		Close:  155.0,
		High:   160.0,
		Low:    145.0,
		Volume: 1000000,
	}

	db.AddPrice(price)

	retrieved, found := db.GetPrice(ticker, date)
	if !found {
		t.Fatal("Price not found after adding")
	}

	if retrieved.Close != 155.0 {
		t.Errorf("Expected close price 155.0, got %f", retrieved.Close)
	}

	latest, found := db.GetLatestPrice(ticker)
	if !found {
		t.Fatal("Latest price not found")
	}

	if latest.Date != date {
		t.Errorf("Expected latest date %d, got %d", date, latest.Date)
	}
}

func TestPriceUpdate(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_snapshot.gob", false)

	ticker := "AAPL"
	date := uint32(1700000000)

	price1 := Price{
		Ticker: ticker,
		Date:   date,
		Open:   150.0,
		Close:  155.0,
		High:   160.0,
		Low:    145.0,
		Volume: 1000000,
	}

	db.AddPrice(price1)

	price2 := Price{
		Ticker: ticker,
		Date:   date,
		Open:   150.0,
		Close:  160.0,
		High:   165.0,
		Low:    145.0,
		Volume: 2000000,
	}

	db.AddPrice(price2)

	retrieved, found := db.GetPrice(ticker, date)
	if !found {
		t.Fatal("Price not found after update")
	}

	if retrieved.Close != 160.0 {
		t.Errorf("Expected updated close price 160.0, got %f", retrieved.Close)
	}

	if retrieved.Volume != 2000000 {
		t.Errorf("Expected updated volume 2000000, got %d", retrieved.Volume)
	}
}

func TestRangeQuery(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_snapshot.gob", false)

	ticker := "AAPL"
	baseDate := uint32(1700000000)

	for i := 0; i < 10; i++ {
		price := generateRandomPrice(ticker, baseDate+uint32(i*60))
		db.AddPrice(price)
	}

	prices := db.GetPricesByTickerRange(ticker, baseDate, baseDate+300)

	if len(prices) != 6 {
		t.Errorf("Expected 6 prices in range, got %d", len(prices))
	}

	for i := 0; i < len(prices)-1; i++ {
		if prices[i].Date >= prices[i+1].Date {
			t.Error("Prices not sorted by date")
		}
	}
}

func TestConcurrentWrites(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_concurrent.gob", false)

	numGoroutines := 10
	pricesPerGoroutine := 100
	tickers := []string{"AAPL", "GOOGL", "MSFT", "TSLA", "AMZN"}

	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < pricesPerGoroutine; i++ {
				ticker := tickers[rand.Intn(len(tickers))]
				date := uint32(1700000000 + goroutineID*10000 + i*60)
				price := generateRandomPrice(ticker, date)
				db.AddPrice(price)
			}
		}(g)
	}

	wg.Wait()

	stats := db.GetStats()
	t.Logf("Concurrent write test completed: %v", stats)
}

func TestConcurrentReadWrite(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_concurrent_rw.gob", false)

	tickers := []string{"AAPL", "GOOGL", "MSFT", "TSLA", "AMZN"}
	baseDate := uint32(1700000000)

	for _, ticker := range tickers {
		for i := 0; i < 50; i++ {
			price := generateRandomPrice(ticker, baseDate+uint32(i*60))
			db.AddPrice(price)
		}
	}

	numReaders := 20
	numWriters := 5
	duration := 3 * time.Second

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	readOps := int64(0)
	writeOps := int64(0)
	var opsMutex sync.Mutex

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			localReads := 0
			for {
				select {
				case <-stopChan:
					opsMutex.Lock()
					readOps += int64(localReads)
					opsMutex.Unlock()
					return
				default:
					ticker := tickers[rand.Intn(len(tickers))]
					operation := rand.Intn(4)

					switch operation {
					case 0:
						db.GetLatestPrice(ticker)
					case 1:
						db.GetPrice(ticker, baseDate+uint32(rand.Intn(50)*60))
					case 2:
						db.GetPricesByTicker(ticker)
					case 3:
						start := baseDate + uint32(rand.Intn(30)*60)
						end := start + uint32(rand.Intn(20)*60)
						db.GetPricesByTickerRange(ticker, start, end)
					}
					localReads++
				}
			}
		}(i)
	}

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			localWrites := 0
			for {
				select {
				case <-stopChan:
					opsMutex.Lock()
					writeOps += int64(localWrites)
					opsMutex.Unlock()
					return
				default:
					ticker := tickers[rand.Intn(len(tickers))]
					date := baseDate + uint32(rand.Intn(100)*60)
					price := generateRandomPrice(ticker, date)
					db.AddPrice(price)
					localWrites++
				}
			}
		}(i)
	}

	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	t.Logf("Performance Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Read Operations: %d (%.0f ops/sec)", readOps, float64(readOps)/duration.Seconds())
	t.Logf("  Write Operations: %d (%.0f ops/sec)", writeOps, float64(writeOps)/duration.Seconds())
	t.Logf("  Total Operations: %d (%.0f ops/sec)", readOps+writeOps, float64(readOps+writeOps)/duration.Seconds())

	stats := db.GetStats()
	t.Logf("Final State: %v", stats)
}

func TestSnapshotSaveLoad(t *testing.T) {
	snapshotPath := "./test_snapshot_saveload.gob"

	db1 := NewPriceDB(time.Hour, snapshotPath, false)

	tickers := []string{"AAPL", "GOOGL", "MSFT"}
	baseDate := uint32(1700000000)

	for _, ticker := range tickers {
		for i := 0; i < 100; i++ {
			price := generateRandomPrice(ticker, baseDate+uint32(i*60))
			db1.AddPrice(price)
		}
	}

	err := db1.SaveSnapshot()
	if err != nil {
		t.Fatalf("Failed to save snapshot: %v", err)
	}

	db2 := NewPriceDB(time.Hour, snapshotPath, true)

	stats1 := db1.GetStats()
	stats2 := db2.GetStats()

	if stats1["total_prices"] != stats2["total_prices"] {
		t.Errorf("Price count mismatch: original %d, loaded %d",
			stats1["total_prices"], stats2["total_prices"])
	}

	if stats1["total_tickers"] != stats2["total_tickers"] {
		t.Errorf("Ticker count mismatch: original %d, loaded %d",
			stats1["total_tickers"], stats2["total_tickers"])
	}

	for _, ticker := range tickers {
		latest1, found1 := db1.GetLatestPrice(ticker)
		latest2, found2 := db2.GetLatestPrice(ticker)

		if found1 != found2 {
			t.Errorf("Latest price existence mismatch for %s", ticker)
		}

		if found1 && latest1.Close != latest2.Close {
			t.Errorf("Latest price mismatch for %s: %f vs %f",
				ticker, latest1.Close, latest2.Close)
		}
	}

	t.Logf("Snapshot save/load test passed with %v", stats1)
}

func TestConcurrentSnapshotWithWrites(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_snapshot_concurrent_writes.gob", false)

	tickers := []string{"AAPL", "GOOGL", "MSFT", "TSLA"}
	baseDate := uint32(1700000000)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			ticker := tickers[rand.Intn(len(tickers))]
			price := generateRandomPrice(ticker, baseDate+uint32(i*60))
			db.AddPrice(price)
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		for i := 0; i < 3; i++ {
			err := db.SaveSnapshot()
			if err != nil {
				t.Errorf("Snapshot save failed: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	wg.Wait()

	stats := db.GetStats()
	t.Logf("Concurrent snapshot test completed: %v", stats)
}

func TestGetAllTickers(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_get_tickers.gob", false)

	expectedTickers := []string{"AAPL", "GOOGL", "MSFT", "TSLA", "AMZN"}
	baseDate := uint32(1700000000)

	for _, ticker := range expectedTickers {
		for i := 0; i < 10; i++ {
			price := generateRandomPrice(ticker, baseDate+uint32(i*60))
			db.AddPrice(price)
		}
	}

	tickers := db.GetAllTickers()

	if len(tickers) != len(expectedTickers) {
		t.Errorf("Expected %d tickers, got %d", len(expectedTickers), len(tickers))
	}

	tickerMap := make(map[string]bool)
	for _, ticker := range tickers {
		tickerMap[ticker] = true
	}

	for _, expected := range expectedTickers {
		if !tickerMap[expected] {
			t.Errorf("Expected ticker %s not found", expected)
		}
	}
}

func TestGetLastPriceTimestamp(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_last_timestamp.gob", false)

	ticker := "AAPL"
	dates := []uint32{1700000000, 1700000060, 1700000120, 1700000180}

	for _, date := range dates {
		price := generateRandomPrice(ticker, date)
		db.AddPrice(price)
	}

	lastTimestamp, err := db.GetLastPriceTimestamp(ticker)
	if err != nil {
		t.Fatalf("Error getting last timestamp: %v", err)
	}

	expectedLast := dates[len(dates)-1]
	if lastTimestamp != expectedLast {
		t.Errorf("Expected last timestamp %d, got %d", expectedLast, lastTimestamp)
	}

	nonExistentTimestamp, err := db.GetLastPriceTimestamp("NONEXISTENT")
	if err != nil {
		t.Fatalf("Error getting non-existent timestamp: %v", err)
	}

	if nonExistentTimestamp != 0 {
		t.Errorf("Expected 0 for non-existent ticker, got %d", nonExistentTimestamp)
	}
}

func BenchmarkConcurrentWrites(b *testing.B) {
	db := NewPriceDB(time.Hour, "./bench_writes.gob", false)
	tickers := []string{"AAPL", "GOOGL", "MSFT", "TSLA", "AMZN"}
	baseDate := uint32(1700000000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ticker := tickers[i%len(tickers)]
			price := generateRandomPrice(ticker, baseDate+uint32(i*60))
			db.AddPrice(price)
			i++
		}
	})
}

func BenchmarkConcurrentReads(b *testing.B) {
	db := NewPriceDB(time.Hour, "./bench_reads.gob", false)
	tickers := []string{"AAPL", "GOOGL", "MSFT", "TSLA", "AMZN"}
	baseDate := uint32(1700000000)

	for _, ticker := range tickers {
		for i := 0; i < 1000; i++ {
			price := generateRandomPrice(ticker, baseDate+uint32(i*60))
			db.AddPrice(price)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ticker := tickers[i%len(tickers)]
			db.GetLatestPrice(ticker)
			i++
		}
	})
}

func BenchmarkRangeQuery(b *testing.B) {
	db := NewPriceDB(time.Hour, "./bench_range.gob", false)
	ticker := "AAPL"
	baseDate := uint32(1700000000)

	for i := 0; i < 10000; i++ {
		price := generateRandomPrice(ticker, baseDate+uint32(i*60))
		db.AddPrice(price)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := baseDate + uint32(rand.Intn(9000)*60)
		end := start + uint32(1000*60)
		db.GetPricesByTickerRange(ticker, start, end)
	}
}

func TestMemoryEfficiency(t *testing.T) {
	db := NewPriceDB(time.Hour, "./test_memory.gob", false)

	tickers := make([]string, 100)
	for i := 0; i < 100; i++ {
		tickers[i] = fmt.Sprintf("TICKER%d", i)
	}

	baseDate := uint32(1700000000)
	pricesPerTicker := 10000

	start := time.Now()
	for _, ticker := range tickers {
		for i := 0; i < pricesPerTicker; i++ {
			price := generateRandomPrice(ticker, baseDate+uint32(i*60))
			db.AddPrice(price)
		}
	}
	elapsed := time.Since(start)

	stats := db.GetStats()
	totalPrices := stats["total_prices"].(int)
	expectedPrices := len(tickers) * pricesPerTicker

	if totalPrices != expectedPrices {
		t.Errorf("Expected %d prices, got %d", expectedPrices, totalPrices)
	}

	t.Logf("Added %d prices in %v (%.0f prices/sec)",
		totalPrices, elapsed, float64(totalPrices)/elapsed.Seconds())
	t.Logf("Stats: %v", stats)
}
