package inmem

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func generateRandomUser(id int) User {
	return User{
		Id:       fmt.Sprintf("user_%d", id),
		Email:    fmt.Sprintf("user%d@test.com", id),
		Password: fmt.Sprintf("password_%d", id),
	}
}

func generateRandomHolding(id int, userID string) Holding {
	return Holding{
		IdHolding:     fmt.Sprintf("holding_%d", id),
		Name:          fmt.Sprintf("Holding %d", id),
		Ticker:        fmt.Sprintf("TKR%d", id),
		ISIN:          fmt.Sprintf("ISIN%d", id),
		Exchange:      "NYSE",
		Policy:        "FIFO",
		Quantity:      float64(rand.Intn(1000) + 1),
		PurchasePrice: float64(rand.Intn(500) + 10),
		TER:           float64(rand.Intn(100)) / 100.0,
		Etf:           rand.Intn(2) == 1,
	}
}

func generateRandomAsset(id int, holdingID string) Asset {
	return Asset{
		IdAsset:  fmt.Sprintf("asset_%d", id),
		Name:     fmt.Sprintf("Asset %d", id),
		Ticker:   fmt.Sprintf("TKR%d", id%100),
		ISIN:     fmt.Sprintf("ISIN%d", id),
		Exchange: "NYSE",
		Sector:   fmt.Sprintf("Sector%d", id%10),
		Region:   fmt.Sprintf("Region%d", id%5),
	}
}

func generateRandomSector(id int, holdingID string) Sector {
	return Sector{
		Name:       fmt.Sprintf("Sector%d", id),
		Percentage: float64(rand.Intn(100)),
		IdHolding:  holdingID,
	}
}

func generateRandomRegion(id int, holdingID string) Region {
	return Region{
		Name:       fmt.Sprintf("Region%d", id),
		Percentage: float64(rand.Intn(100)),
		IdHolding:  holdingID,
	}
}

func generateRandomNews(id int, ticker string) News {
	return News{
		IdNews:      fmt.Sprintf("news_%d", id),
		Title:       fmt.Sprintf("News Title %d", id),
		Link:        fmt.Sprintf("http://news.com/%d", id),
		PublishedAt: time.Now().Format(time.RFC3339),
		Summary:     fmt.Sprintf("Summary %d", id),
		Text:        fmt.Sprintf("Full text %d", id),
		Author:      fmt.Sprintf("Author%d", id%10),
		Ticker:      ticker,
		Sentiment:   float64(rand.Intn(200)-100) / 100.0,
	}
}

func generateRandomPrice(id int, ticker string, date string) Prices {
	open := float64(rand.Intn(1000) + 100)
	return Prices{
		IdPrice: fmt.Sprintf("price_%d", id),
		Ticker:  ticker,
		Date:    date,
		Open:    open,
		Close:   open + float64(rand.Intn(20)-10),
		High:    open + float64(rand.Intn(30)),
		Low:     open - float64(rand.Intn(30)),
		Volume:  int64(rand.Intn(1000000) + 10000),
	}
}

func generateRandomDailySentiment(id int, ticker string, date string) DailySentiment {
	return DailySentiment{
		IdSentiment: fmt.Sprintf("sentiment_%d", id),
		Ticker:      ticker,
		Date:        date,
		Summary:     fmt.Sprintf("Daily summary %d", id),
		Sentiment:   float64(rand.Intn(200)-100) / 100.0,
	}
}

func generateRandomPortfolioSentiment(id int, userID string, date string) PortfolioDailySentiment {
	return PortfolioDailySentiment{
		IdSentiment: fmt.Sprintf("portfolio_sentiment_%d", id),
		UserID:      userID,
		Date:        date,
		Summary:     fmt.Sprintf("Portfolio summary %d", id),
		Sentiment:   float64(rand.Intn(200)-100) / 100.0,
	}
}

func generateRandomAssetDetails(id int, ticker string) AssetDetails {
	return AssetDetails{
		Ticker:        ticker,
		ISIN:          fmt.Sprintf("ISIN%d", id),
		MarketCap:     fmt.Sprintf("%dB", rand.Intn(1000)),
		MarketCapEur:  fmt.Sprintf("%dB", rand.Intn(1000)),
		Country:       fmt.Sprintf("Country%d", id%20),
		Sector:        fmt.Sprintf("Sector%d", id%10),
		Eps:           fmt.Sprintf("%.2f", float64(rand.Intn(100))/10.0),
		PbRatio:       fmt.Sprintf("%.2f", float64(rand.Intn(50))/10.0),
		PeRatio:       fmt.Sprintf("%.2f", float64(rand.Intn(300))/10.0),
		DividendYield: fmt.Sprintf("%.2f", float64(rand.Intn(100))/10.0),
		Revenue:       fmt.Sprintf("%dM", rand.Intn(10000)),
		NetIncome:     fmt.Sprintf("%dM", rand.Intn(5000)),
		ProfitMargin:  fmt.Sprintf("%.2f", float64(rand.Intn(500))/10.0),
		Hash:          fmt.Sprintf("hash_%d", id),
		Date:          time.Now().Format("20060102"),
	}
}

func TestConcurrentOperations(t *testing.T) {
	db := createInMemDB(time.Hour, "./test_snapshot.gob")

	numGoroutines := 20
	operationsPerGoroutine := 100

	var wg sync.WaitGroup
	errorsChan := make(chan error, numGoroutines*operationsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < operationsPerGoroutine; i++ {
				operation := rand.Intn(15)

				switch operation {
				case 0:
					user := generateRandomUser(goroutineID*1000 + i)
					db.AddUser(user)

				case 1:
					userID := fmt.Sprintf("user_%d", rand.Intn(numGoroutines*1000))
					_, _ = db.GetUser(userID)

				case 2:
					_ = db.GetAllUsers()

				case 3:
					holding := generateRandomHolding(goroutineID*1000+i, fmt.Sprintf("user_%d", goroutineID))
					db.AddHolding(holding)

				case 4:
					holdingID2 := fmt.Sprintf("holding_%d", rand.Intn(numGoroutines*1000))
					_, _ = db.GetHolding(holdingID2)

				case 5:
					_ = db.GetAllHoldings()

				case 6:
					holdingID := fmt.Sprintf("holding_%d", goroutineID*100)
					asset := generateRandomAsset(goroutineID*1000+i, holdingID)
					db.AddAsset(asset)

				case 7:
					ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
					_ = db.GetAssetsByTicker(ticker)

				case 8:
					holdingID := fmt.Sprintf("holding_%d", goroutineID*100)
					sector := generateRandomSector(i, holdingID)
					db.AddSector(sector)

				case 9:
					holdingID := fmt.Sprintf("holding_%d", goroutineID*100)
					region := generateRandomRegion(i, holdingID)
					db.AddRegion(region)

				case 10:
					ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
					news := generateRandomNews(goroutineID*1000+i, ticker)
					db.AddNews(news)

				case 11:
					ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
					date := fmt.Sprintf("2025%02d%02d", rand.Intn(12)+1, rand.Intn(28)+1)
					price := generateRandomPrice(goroutineID*1000+i, ticker, date)
					db.AddPrice(price)

				case 12:
					ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
					date := fmt.Sprintf("2025%02d%02d", rand.Intn(12)+1, rand.Intn(28)+1)
					sentiment := generateRandomDailySentiment(goroutineID*1000+i, ticker, date)
					db.AddDailySentiment(sentiment)

				case 13:
					userID := fmt.Sprintf("user_%d", goroutineID)
					date := fmt.Sprintf("2025%02d%02d", rand.Intn(12)+1, rand.Intn(28)+1)
					sentiment := generateRandomPortfolioSentiment(goroutineID*1000+i, userID, date)
					db.AddPortfolioDailySentiment(sentiment)

				case 14:
					ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
					details := generateRandomAssetDetails(goroutineID*1000+i, ticker)
					db.AddAssetDetails(details)
				}

				if rand.Intn(100) < 10 {
					time.Sleep(time.Microsecond * time.Duration(rand.Intn(100)))
				}
			}
		}(g)
	}

	wg.Wait()
	close(errorsChan)

	errors := []error{}
	for err := range errorsChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Errorf("Encountered %d errors during concurrent operations", len(errors))
		for i, err := range errors {
			if i < 10 {
				t.Logf("Error %d: %v", i+1, err)
			}
		}
	}

	users := db.GetAllUsers()
	holdings := db.GetAllHoldings()
	assets := db.GetAllAssets()

	t.Logf("Final counts - Users: %d, Holdings: %d, Assets: %d", len(users), len(holdings), len(assets))
}

func TestConcurrentReadWriteMix(t *testing.T) {
	db := createInMemDB(time.Hour, "./test_snapshot_mix.gob")

	numUsers := 50
	for i := 0; i < numUsers; i++ {
		db.AddUser(generateRandomUser(i))
	}

	numHoldings := 100
	for i := 0; i < numHoldings; i++ {
		db.AddHolding(generateRandomHolding(i, fmt.Sprintf("user_%d", i%numUsers)))
	}

	numReaders := 30
	numWriters := 10
	duration := 5 * time.Second

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
					operation := rand.Intn(10)
					switch operation {
					case 0:
						_ = db.GetAllUsers()
					case 1:
						_ = db.GetAllHoldings()
					case 2:
						_ = db.GetAllAssets()
					case 3:
						userID := fmt.Sprintf("user_%d", rand.Intn(numUsers))
						_, _ = db.GetUser(userID)
					case 4:
						holdingID := fmt.Sprintf("holding_%d", rand.Intn(numHoldings))
						_, _ = db.GetHolding(holdingID)
					case 5:
						ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
						_ = db.GetAssetsByTicker(ticker)
					case 6:
						holdingID := fmt.Sprintf("holding_%d", rand.Intn(numHoldings))
						_ = db.GetRegionsByHolding(holdingID)
					case 7:
						holdingID := fmt.Sprintf("holding_%d", rand.Intn(numHoldings))
						_ = db.GetSectorsByHolding(holdingID)
					case 8:
						ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
						_ = db.GetNewsByTicker(ticker)
					case 9:
						ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
						_ = db.GetPricesByTicker(ticker)
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
			baseID := writerID * 10000
			for {
				select {
				case <-stopChan:
					opsMutex.Lock()
					writeOps += int64(localWrites)
					opsMutex.Unlock()
					return
				default:
					operation := rand.Intn(10)
					switch operation {
					case 0:
						user := generateRandomUser(baseID + localWrites)
						db.AddUser(user)
					case 1:
						holding := generateRandomHolding(baseID+localWrites, fmt.Sprintf("user_%d", rand.Intn(numUsers)))
						db.AddHolding(holding)
					case 2:
						holdingID := fmt.Sprintf("holding_%d", rand.Intn(numHoldings))
						asset := generateRandomAsset(baseID+localWrites, holdingID)
						db.AddAsset(asset)
					case 3:
						holdingID := fmt.Sprintf("holding_%d", rand.Intn(numHoldings))
						db.AddSector(generateRandomSector(baseID+localWrites, holdingID))
					case 4:
						holdingID := fmt.Sprintf("holding_%d", rand.Intn(numHoldings))
						db.AddRegion(generateRandomRegion(baseID+localWrites, holdingID))
					case 5:
						ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
						db.AddNews(generateRandomNews(baseID+localWrites, ticker))
					case 6:
						ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
						date := fmt.Sprintf("2025%02d%02d", rand.Intn(12)+1, rand.Intn(28)+1)
						db.AddPrice(generateRandomPrice(baseID+localWrites, ticker, date))
					case 7:
						_ = fmt.Sprintf("holding_%d", rand.Intn(numHoldings))
						db.UpdateHolding(generateRandomHolding(rand.Intn(numHoldings), fmt.Sprintf("user_%d", rand.Intn(numUsers))))
					case 8:
						ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
						date := fmt.Sprintf("2025%02d%02d", rand.Intn(12)+1, rand.Intn(28)+1)
						db.AddDailySentiment(generateRandomDailySentiment(baseID+localWrites, ticker, date))
					case 9:
						ticker := fmt.Sprintf("TKR%d", rand.Intn(100))
						db.AddAssetDetails(generateRandomAssetDetails(baseID+localWrites, ticker))
					}
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

	finalUsers := db.GetAllUsers()
	finalHoldings := db.GetAllHoldings()
	finalAssets := db.GetAllAssets()

	t.Logf("Final State:")
	t.Logf("  Users: %d", len(finalUsers))
	t.Logf("  Holdings: %d", len(finalHoldings))
	t.Logf("  Assets: %d", len(finalAssets))
}

func TestSnapshotConcurrency(t *testing.T) {
	db := createInMemDB(time.Second, "./test_snapshot_concurrent.gob")

	numOperations := 1000
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			db.AddUser(generateRandomUser(i))
			db.AddHolding(generateRandomHolding(i, fmt.Sprintf("user_%d", i%100)))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(2 * time.Second)
		err := db.SaveSnapshot()
		if err != nil {
			t.Errorf("Snapshot save failed: %v", err)
		}
	}()

	wg.Wait()

	users := db.GetAllUsers()
	holdings := db.GetAllHoldings()

	t.Logf("Snapshot test completed - Users: %d, Holdings: %d", len(users), len(holdings))
}

func TestDeleteOperationsConcurrency(t *testing.T) {
	db := createInMemDB(time.Hour, "./test_delete.gob")

	numHoldings := 100
	for i := 0; i < numHoldings; i++ {
		holding := generateRandomHolding(i, "user_1")
		db.AddHolding(holding)

		for j := 0; j < 5; j++ {
			db.AddAsset(generateRandomAsset(i*100+j, holding.IdHolding))
			db.AddSector(generateRandomSector(j, holding.IdHolding))
			db.AddRegion(generateRandomRegion(j, holding.IdHolding))
		}
	}

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				holdingID := fmt.Sprintf("holding_%d", goroutineID*10+j)
				db.DeleteHolding(holdingID)
				db.DeleteAssetsByHolding(holdingID)
				db.DeleteSectorsByHolding(holdingID)
				db.DeleteRegionsByHolding(holdingID)
			}
		}(i)
	}

	wg.Wait()

	remainingHoldings := db.GetAllHoldings()
	remainingAssets := db.GetAllAssets()
	remainingSectors := db.GetAllSectors()
	remainingRegions := db.GetAllRegions()

	t.Logf("Delete test completed:")
	t.Logf("  Remaining Holdings: %d", len(remainingHoldings))
	t.Logf("  Remaining Assets: %d", len(remainingAssets))
	t.Logf("  Remaining Sectors: %d", len(remainingSectors))
	t.Logf("  Remaining Regions: %d", len(remainingRegions))
}

func BenchmarkConcurrentWrites(b *testing.B) {
	db := createInMemDB(time.Hour, "./bench_snapshot.gob")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := 0
		for pb.Next() {
			db.AddUser(generateRandomUser(id))
			id++
		}
	})
}

func BenchmarkConcurrentReads(b *testing.B) {
	db := createInMemDB(time.Hour, "./bench_snapshot.gob")

	for i := 0; i < 1000; i++ {
		db.AddUser(generateRandomUser(i))
		db.AddHolding(generateRandomHolding(i, fmt.Sprintf("user_%d", i%100)))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = db.GetAllUsers()
		}
	})
}

func BenchmarkMixedOperations(b *testing.B) {
	db := createInMemDB(time.Hour, "./bench_mixed.gob")

	for i := 0; i < 100; i++ {
		db.AddUser(generateRandomUser(i))
		db.AddHolding(generateRandomHolding(i, fmt.Sprintf("user_%d", i%10)))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := 0
		for pb.Next() {
			if rand.Intn(2) == 0 {
				_ = db.GetAllUsers()
			} else {
				db.AddUser(generateRandomUser(10000 + id))
				id++
			}
		}
	})
}
