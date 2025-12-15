package inmem

import (
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Price struct {
	Ticker string
	Date   uint32
	Open   float32
	Close  float32
	High   float32
	Low    float32
	Volume int64
}

type TickerData struct {
	mutex   sync.RWMutex
	Dates   []uint32
	Opens   []float32
	Closes  []float32
	Highs   []float32
	Lows    []float32
	Volumes []int64
}

type PriceDB struct {
	tickers        map[string]*TickerData
	tickersMutex   sync.RWMutex
	snapshotMutex  sync.RWMutex
	snapshotPeriod time.Duration
	snapShotPath   string
}

type Snapshot struct {
	Tickers   map[string]*TickerData
	Timestamp time.Time
}

const DEBUG = true

func NewPriceDB(snapshotPeriod time.Duration, snapShotPath string, loadSnapshot bool) *PriceDB {
	db := &PriceDB{
		tickers:        make(map[string]*TickerData),
		snapshotPeriod: snapshotPeriod,
		snapShotPath:   snapShotPath,
	}

	if loadSnapshot {
		err := db.LoadSnapshot()
		if err != nil {
			log.Printf("Failed to load snapshot: %v, starting with empty database", err)
		}
	}

	db.StartSnapshotRoutine()
	return db
}

func NewPriceDBFromSQL(snapshotPeriod time.Duration, snapShotPath string, sqlDBPath string) (*PriceDB, error) {
	db := &PriceDB{
		tickers:        make(map[string]*TickerData),
		snapshotPeriod: snapshotPeriod,
		snapShotPath:   snapShotPath,
	}

	err := db.LoadFromSQL(sqlDBPath)
	if err != nil {
		return nil, err
	}

	db.StartSnapshotRoutine()
	return db, nil
}

func (db *PriceDB) StartSnapshotRoutine() {
	ticker := time.NewTicker(db.snapshotPeriod)
	go func() {
		for range ticker.C {
			err := db.SaveSnapshot()
			if err != nil {
				log.Printf("Failed to save snapshot: %v", err)
			}
		}
	}()
}

func (db *PriceDB) getOrCreateTicker(ticker string) *TickerData {
	db.tickersMutex.RLock()
	td, exists := db.tickers[ticker]
	db.tickersMutex.RUnlock()

	if exists {
		return td
	}

	db.tickersMutex.Lock()
	defer db.tickersMutex.Unlock()

	td, exists = db.tickers[ticker]
	if exists {
		return td
	}

	td = &TickerData{
		Dates:   make([]uint32, 0),
		Opens:   make([]float32, 0),
		Closes:  make([]float32, 0),
		Highs:   make([]float32, 0),
		Lows:    make([]float32, 0),
		Volumes: make([]int64, 0),
	}
	db.tickers[ticker] = td
	return td
}

func (db *PriceDB) AddPrice(price Price) {
	td := db.getOrCreateTicker(price.Ticker)

	td.mutex.Lock()
	defer td.mutex.Unlock()

	idx := sort.Search(len(td.Dates), func(i int) bool {
		return td.Dates[i] >= price.Date
	})

	if idx < len(td.Dates) && td.Dates[idx] == price.Date {
		td.Opens[idx] = price.Open
		td.Closes[idx] = price.Close
		td.Highs[idx] = price.High
		td.Lows[idx] = price.Low
		td.Volumes[idx] = price.Volume
		return
	}

	td.Dates = append(td.Dates, 0)
	td.Opens = append(td.Opens, 0)
	td.Closes = append(td.Closes, 0)
	td.Highs = append(td.Highs, 0)
	td.Lows = append(td.Lows, 0)
	td.Volumes = append(td.Volumes, 0)

	copy(td.Dates[idx+1:], td.Dates[idx:])
	copy(td.Opens[idx+1:], td.Opens[idx:])
	copy(td.Closes[idx+1:], td.Closes[idx:])
	copy(td.Highs[idx+1:], td.Highs[idx:])
	copy(td.Lows[idx+1:], td.Lows[idx:])
	copy(td.Volumes[idx+1:], td.Volumes[idx:])

	td.Dates[idx] = price.Date
	td.Opens[idx] = price.Open
	td.Closes[idx] = price.Close
	td.Highs[idx] = price.High
	td.Lows[idx] = price.Low
	td.Volumes[idx] = price.Volume
}

func (db *PriceDB) AddPrices(prices []Price) {
	for _, price := range prices {
		db.AddPrice(price)
	}
}

func (db *PriceDB) GetPrice(ticker string, date uint32) (Price, bool) {
	db.tickersMutex.RLock()
	td, exists := db.tickers[ticker]
	db.tickersMutex.RUnlock()

	if !exists {
		return Price{}, false
	}

	td.mutex.RLock()
	defer td.mutex.RUnlock()

	idx := sort.Search(len(td.Dates), func(i int) bool {
		return td.Dates[i] >= date
	})

	if idx < len(td.Dates) && td.Dates[idx] == date {
		return Price{
			Ticker: ticker,
			Date:   td.Dates[idx],
			Open:   td.Opens[idx],
			Close:  td.Closes[idx],
			High:   td.Highs[idx],
			Low:    td.Lows[idx],
			Volume: td.Volumes[idx],
		}, true
	}

	return Price{}, false
}

func (db *PriceDB) GetLatestPrice(ticker string) (Price, bool) {
	db.tickersMutex.RLock()
	td, exists := db.tickers[ticker]
	db.tickersMutex.RUnlock()

	if !exists {
		return Price{}, false
	}

	td.mutex.RLock()
	defer td.mutex.RUnlock()

	if len(td.Dates) == 0 {
		return Price{}, false
	}

	idx := len(td.Dates) - 1
	return Price{
		Ticker: ticker,
		Date:   td.Dates[idx],
		Open:   td.Opens[idx],
		Close:  td.Closes[idx],
		High:   td.Highs[idx],
		Low:    td.Lows[idx],
		Volume: td.Volumes[idx],
	}, true
}

func (db *PriceDB) GetPricesByTickerRange(ticker string, startDate, endDate uint32) []Price {
	db.tickersMutex.RLock()
	td, exists := db.tickers[ticker]
	db.tickersMutex.RUnlock()

	if !exists {
		return []Price{}
	}

	td.mutex.RLock()
	defer td.mutex.RUnlock()

	startIdx := sort.Search(len(td.Dates), func(i int) bool {
		return td.Dates[i] >= startDate
	})

	endIdx := sort.Search(len(td.Dates), func(i int) bool {
		return td.Dates[i] > endDate
	})

	prices := make([]Price, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		prices = append(prices, Price{
			Ticker: ticker,
			Date:   td.Dates[i],
			Open:   td.Opens[i],
			Close:  td.Closes[i],
			High:   td.Highs[i],
			Low:    td.Lows[i],
			Volume: td.Volumes[i],
		})
	}

	return prices
}

func (db *PriceDB) GetPricesByTicker(ticker string) []Price {
	db.tickersMutex.RLock()
	td, exists := db.tickers[ticker]
	db.tickersMutex.RUnlock()

	if !exists {
		return []Price{}
	}

	td.mutex.RLock()
	defer td.mutex.RUnlock()

	prices := make([]Price, len(td.Dates))
	for i := range td.Dates {
		prices[i] = Price{
			Ticker: ticker,
			Date:   td.Dates[i],
			Open:   td.Opens[i],
			Close:  td.Closes[i],
			High:   td.Highs[i],
			Low:    td.Lows[i],
			Volume: td.Volumes[i],
		}
	}

	return prices
}

func (db *PriceDB) GetAllTickers() []string {
	db.tickersMutex.RLock()
	defer db.tickersMutex.RUnlock()

	tickers := make([]string, 0, len(db.tickers))
	for ticker := range db.tickers {
		tickers = append(tickers, ticker)
	}

	return tickers
}

func (db *PriceDB) GetLastPriceTimestamp(ticker string) (uint32, error) {
	db.tickersMutex.RLock()
	td, exists := db.tickers[ticker]
	db.tickersMutex.RUnlock()

	if !exists {
		return 0, nil
	}

	td.mutex.RLock()
	defer td.mutex.RUnlock()

	if len(td.Dates) == 0 {
		return 0, nil
	}

	return td.Dates[len(td.Dates)-1], nil
}

func (db *PriceDB) SaveSnapshot() error {
	db.snapshotMutex.Lock()
	defer db.snapshotMutex.Unlock()

	db.tickersMutex.RLock()

	snapshot := Snapshot{
		Tickers:   make(map[string]*TickerData),
		Timestamp: time.Now(),
	}

	for ticker, td := range db.tickers {
		td.mutex.RLock()
		tdCopy := &TickerData{
			Dates:   make([]uint32, len(td.Dates)),
			Opens:   make([]float32, len(td.Opens)),
			Closes:  make([]float32, len(td.Closes)),
			Highs:   make([]float32, len(td.Highs)),
			Lows:    make([]float32, len(td.Lows)),
			Volumes: make([]int64, len(td.Volumes)),
		}
		copy(tdCopy.Dates, td.Dates)
		copy(tdCopy.Opens, td.Opens)
		copy(tdCopy.Closes, td.Closes)
		copy(tdCopy.Highs, td.Highs)
		copy(tdCopy.Lows, td.Lows)
		copy(tdCopy.Volumes, td.Volumes)
		td.mutex.RUnlock()

		snapshot.Tickers[ticker] = tdCopy
	}

	db.tickersMutex.RUnlock()

	file, err := os.Create(db.snapShotPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	err = encoder.Encode(snapshot)
	if err != nil {
		return err
	}

	if DEBUG {
		jsonFile, err := os.Create(db.snapShotPath + ".json")
		if err != nil {
			return err
		}
		defer jsonFile.Close()

		jsonEncoder := json.NewEncoder(jsonFile)
		jsonEncoder.SetIndent("", "  ")
		err = jsonEncoder.Encode(snapshot)
		if err != nil {
			return err
		}
	}

	log.Printf("Snapshot saved successfully at %s with %d tickers", db.snapShotPath, len(snapshot.Tickers))
	return nil
}

func (db *PriceDB) LoadSnapshot() error {
	file, err := os.Open(db.snapShotPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var snapshot Snapshot
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&snapshot)
	if err != nil {
		return err
	}

	db.snapshotMutex.Lock()
	defer db.snapshotMutex.Unlock()

	db.tickersMutex.Lock()
	defer db.tickersMutex.Unlock()

	db.tickers = snapshot.Tickers

	totalPrices := 0
	for _, td := range db.tickers {
		totalPrices += len(td.Dates)
	}

	log.Printf("Snapshot loaded successfully from %s (timestamp: %s, %d tickers, %d total prices)",
		db.snapShotPath, snapshot.Timestamp, len(db.tickers), totalPrices)
	return nil
}

func (db *PriceDB) LoadFromSQL(sqlDBPath string) error {
	sqlDB, err := sql.Open("sqlite3", sqlDBPath)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	log.Printf("Loading prices from SQL database: %s", sqlDBPath)

	rows, err := sqlDB.Query(`
		SELECT ticker, CAST(date AS INTEGER), open, close, high, low, volume
		FROM prices
		ORDER BY ticker, CAST(date AS INTEGER) ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var ticker string
		var date int64
		var open, close, high, low float64
		var volume int64

		err := rows.Scan(&ticker, &date, &open, &close, &high, &low, &volume)
		if err != nil {
			log.Printf("Error scanning price: %v", err)
			continue
		}

		price := Price{
			Ticker: ticker,
			Date:   uint32(date),
			Open:   float32(open),
			Close:  float32(close),
			High:   float32(high),
			Low:    float32(low),
			Volume: volume,
		}

		db.AddPrice(price)
		count++
	}

	log.Printf("Loaded %d prices from SQL database", count)
	return nil
}

func (db *PriceDB) GetStats() map[string]interface{} {
	db.tickersMutex.RLock()
	defer db.tickersMutex.RUnlock()

	totalPrices := 0
	for _, td := range db.tickers {
		td.mutex.RLock()
		totalPrices += len(td.Dates)
		td.mutex.RUnlock()
	}

	return map[string]interface{}{
		"total_tickers": len(db.tickers),
		"total_prices":  totalPrices,
	}
}
