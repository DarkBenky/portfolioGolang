package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/marcboeker/go-duckdb/v2"
)

const priceDBPath = "./prices.duckdb"

type PriceCandle struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
}

type priceBucket struct {
	seconds int64
	expr    string
}

func (b priceBucket) startFor(ts int64) int64 {
	switch {
	case b.seconds <= 86400:
		return (ts / b.seconds) * b.seconds
	case b.seconds == 604800:
		return ((ts-345600)/604800)*604800 + 345600
	default:
		t := time.Unix(ts, 0).UTC()
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
	}
}

type PriceStore struct {
	db *sql.DB
	mu sync.Mutex
}

var priceStore *PriceStore

func epochBucketExpr(seconds int64) string {
	return fmt.Sprintf("((date // %d) * %d)", seconds, seconds)
}

func weekBucketExpr() string {
	return "CAST(epoch(date_trunc('week', to_timestamp(date) AT TIME ZONE 'UTC')) AS BIGINT)"
}

func monthBucketExpr() string {
	return "CAST(epoch(date_trunc('month', to_timestamp(date) AT TIME ZONE 'UTC')) AS BIGINT)"
}

// priceIntervalSpec maps an API interval to its aggregation bucket and lookback window in seconds.
func priceIntervalSpec(interval string) (priceBucket, int64) {
	switch interval {
	case "5m":
		return priceBucket{seconds: 300, expr: epochBucketExpr(300)}, 24 * 3600
	case "15m":
		return priceBucket{seconds: 900, expr: epochBucketExpr(900)}, 7 * 24 * 3600
	case "1h":
		return priceBucket{seconds: 3600, expr: epochBucketExpr(3600)}, 30 * 24 * 3600
	case "4h":
		return priceBucket{seconds: 14400, expr: epochBucketExpr(14400)}, 90 * 24 * 3600
	case "1d":
		return priceBucket{seconds: 86400, expr: epochBucketExpr(86400)}, 365 * 24 * 3600
	case "1w":
		return priceBucket{seconds: 604800, expr: weekBucketExpr()}, 730 * 24 * 3600
	case "1M":
		return priceBucket{seconds: 2592000, expr: monthBucketExpr()}, 0
	default:
		return priceBucket{seconds: 3600, expr: epochBucketExpr(3600)}, 30 * 24 * 3600
	}
}

func duckdbSetting(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	for _, r := range value {
		valid := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '.'
		if !valid {
			log.Printf("prices: ignoring invalid %s=%q", name, value)
			return fallback
		}
	}
	return value
}

func initPriceStore() error {
	conn, err := sql.Open("duckdb", priceDBPath)
	if err != nil {
		return err
	}
	conn.SetMaxOpenConns(2)
	conn.SetMaxIdleConns(2)
	conn.SetConnMaxLifetime(time.Hour)

	if _, err := conn.Exec(`SET threads = 2`); err != nil {
		log.Printf("prices: could not set thread count: %v", err)
	}
	if _, err := conn.Exec(`SET preserve_insertion_order = false`); err != nil {
		log.Printf("prices: could not disable insertion order preservation: %v", err)
	}
	migrationMemory := duckdbSetting("PRICE_MIGRATION_MEMORY", "2GB")
	steadyMemory := duckdbSetting("PRICE_MEMORY_LIMIT", "512MB")
	if _, err := conn.Exec(fmt.Sprintf("SET memory_limit = '%s'", migrationMemory)); err != nil {
		log.Printf("prices: could not set migration memory limit: %v", err)
	}

	schema := `
		CREATE TABLE IF NOT EXISTS prices (
			ticker VARCHAR NOT NULL,
			date BIGINT NOT NULL,
			open DOUBLE NOT NULL,
			high DOUBLE NOT NULL,
			low DOUBLE NOT NULL,
			close DOUBLE NOT NULL,
			volume BIGINT NOT NULL,
			PRIMARY KEY (ticker, date)
		)
	`
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return err
	}

	priceStore = &PriceStore{db: conn}
	if err := priceStore.migrateFromSQLite(); err != nil {
		return err
	}

	var total int64
	var tickers int64
	if err := conn.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT ticker) FROM prices`).Scan(&total, &tickers); err == nil {
		log.Printf("Price store ready (%s): %d bars across %d tickers", priceDBPath, total, tickers)
	}
	if _, err := conn.Exec(fmt.Sprintf("SET memory_limit = '%s'", steadyMemory)); err != nil {
		log.Printf("prices: could not set steady state memory limit: %v", err)
	}
	return nil
}

func (s *PriceStore) migrateFromSQLite() error {
	var rows int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM prices`).Scan(&rows); err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}

	var legacyRows int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM prices`).Scan(&legacyRows); err != nil {
		log.Printf("prices: no legacy SQLite price data: %v", err)
		return nil
	}
	if legacyRows == 0 {
		return nil
	}

	log.Printf("prices: migrating %d rows from SQLite to DuckDB...", legacyRows)

	src, err := db.Query(`SELECT ticker, date, open, high, low, close, volume FROM prices ORDER BY ticker, date`)
	if err != nil {
		return err
	}
	defer src.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	sqlConn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer sqlConn.Close()

	copied := 0
	appendErr := sqlConn.Raw(func(dc any) error {
		driverConn, ok := dc.(driver.Conn)
		if !ok {
			return fmt.Errorf("unexpected duckdb connection type %T", dc)
		}
		appender, err := duckdb.NewAppenderFromConn(driverConn, "", "prices")
		if err != nil {
			return err
		}
		for src.Next() {
			var p Price
			if err := src.Scan(&p.Ticker, &p.Date, &p.Open, &p.High, &p.Low, &p.Close, &p.Volume); err != nil {
				continue
			}
			if err := appender.AppendRow(p.Ticker, p.Date, p.Open, p.High, p.Low, p.Close, p.Volume); err != nil {
				appender.Close()
				return err
			}
			copied++
			if copied%100000 == 0 {
				log.Printf("prices: migrated %d/%d rows", copied, legacyRows)
			}
		}
		return appender.Close()
	})
	if appendErr != nil {
		return appendErr
	}

	log.Printf("prices: migration finished, %d rows copied", copied)
	return nil
}

func (s *PriceStore) upsertPrices(prices []Price) error {
	if len(prices) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO prices (ticker, date, open, close, high, low, volume) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (ticker, date) DO NOTHING`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, price := range prices {
		if _, err := stmt.Exec(price.Ticker, price.Date, price.Open, price.Close, price.High, price.Low, price.Volume); err != nil {
			log.Printf("Error adding price for %s on %d: %v", price.Ticker, price.Date, err)
		}
	}
	stmt.Close()
	return tx.Commit()
}

func (s *PriceStore) lastTimestamp(ticker string) (int64, error) {
	var last sql.NullInt64
	now := time.Now().UTC().Unix()
	err := s.db.QueryRow(`SELECT MAX(date) FROM prices WHERE ticker = ? AND date > 0 AND date <= ?`, ticker, now).Scan(&last)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if !last.Valid || last.Int64 <= 0 || last.Int64 > now {
		return 0, nil
	}
	return last.Int64, nil
}

func (s *PriceStore) candles(ticker string, start, end int64, bucket priceBucket) ([]PriceCandle, error) {
	query := fmt.Sprintf(`
		SELECT %s AS bucket_ts,
		       arg_min(open, date),
		       max(high),
		       min(low),
		       arg_max(close, date),
		       CAST(COALESCE(sum(volume), 0) AS BIGINT)
		FROM prices
		WHERE ticker = ? AND date >= ? AND date <= ?
		GROUP BY 1
		ORDER BY 1
	`, bucket.expr)

	rows, err := s.db.Query(query, ticker, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]PriceCandle, 0)
	for rows.Next() {
		var c PriceCandle
		if err := rows.Scan(&c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *PriceStore) candlesMulti(tickers []string, start, end int64, bucket priceBucket) (map[string][]PriceCandle, error) {
	result := make(map[string][]PriceCandle, len(tickers))
	if len(tickers) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tickers)), ",")
	args := make([]interface{}, 0, len(tickers)+2)
	for _, t := range tickers {
		args = append(args, t)
	}
	args = append(args, start, end)

	query := fmt.Sprintf(`
		SELECT ticker, %s AS bucket_ts,
		       arg_min(open, date),
		       max(high),
		       min(low),
		       arg_max(close, date),
		       CAST(COALESCE(sum(volume), 0) AS BIGINT)
		FROM prices
		WHERE ticker IN (%s) AND date >= ? AND date <= ?
		GROUP BY 1, 2
		ORDER BY 2
	`, bucket.expr, placeholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ticker string
		var c PriceCandle
		if err := rows.Scan(&ticker, &c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, err
		}
		result[ticker] = append(result[ticker], c)
	}
	return result, rows.Err()
}

func (s *PriceStore) latestCloses(tickers []string) (map[string]float64, error) {
	result := make(map[string]float64, len(tickers))
	if len(tickers) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tickers)), ",")
	args := make([]interface{}, 0, len(tickers))
	for _, t := range tickers {
		args = append(args, t)
	}

	query := fmt.Sprintf(`
		SELECT ticker, arg_max(close, date)
		FROM prices
		WHERE ticker IN (%s)
		GROUP BY 1
	`, placeholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ticker string
		var closePrice float64
		if err := rows.Scan(&ticker, &closePrice); err != nil {
			return nil, err
		}
		result[ticker] = closePrice
	}
	return result, rows.Err()
}

func (s *PriceStore) lastCloseBefore(ticker string, ts int64) (float64, error) {
	var closePrice sql.NullFloat64
	err := s.db.QueryRow(`SELECT arg_max(close, date) FROM prices WHERE ticker = ? AND date <= ?`, ticker, ts).Scan(&closePrice)
	if err != nil {
		return 0, err
	}
	if !closePrice.Valid {
		return 0, sql.ErrNoRows
	}
	return closePrice.Float64, nil
}

func (s *PriceStore) tickers() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT ticker FROM prices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tickers []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tickers = append(tickers, t)
	}
	return tickers, rows.Err()
}

func closeSeries(ticker string, start, end int64) ([]float64, error) {
	rows, err := priceStore.db.Query(`SELECT close FROM prices WHERE ticker = ? AND date >= ? AND date <= ? ORDER BY date`, ticker, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prices []float64
	for rows.Next() {
		var price float64
		if err := rows.Scan(&price); err != nil {
			return nil, err
		}
		prices = append(prices, price)
	}
	return prices, rows.Err()
}

func closeSeriesWithDates(ticker string, start, end int64) ([]float64, []int64, error) {
	rows, err := priceStore.db.Query(`SELECT date, close FROM prices WHERE ticker = ? AND date >= ? AND date <= ? ORDER BY date`, ticker, start, end)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var prices []float64
	var dates []int64
	for rows.Next() {
		var date int64
		var price float64
		if err := rows.Scan(&date, &price); err != nil {
			return nil, nil, err
		}
		dates = append(dates, date)
		prices = append(prices, price)
	}
	return prices, dates, rows.Err()
}

func latestClose(ticker string) (float64, error) {
	closes, err := priceStore.latestCloses([]string{ticker})
	if err != nil {
		return 0, err
	}
	if price, ok := closes[ticker]; ok {
		return price, nil
	}
	return 0, sql.ErrNoRows
}

func latestCloseBefore(ticker string, ts int64) (float64, error) {
	return priceStore.lastCloseBefore(ticker, ts)
}

var backtestCacheMu sync.Mutex
var backtestCache = map[string]backtestCacheEntry{}

type backtestCacheEntry struct {
	series []Price
	stored time.Time
	start  int64
	end    int64
}

const backtestCacheTTL = 15 * time.Minute

// backtestPriceSeries returns daily bars for a ticker, cached for 15 minutes.
func backtestPriceSeries(ticker string, start, end int64) ([]Price, error) {
	backtestCacheMu.Lock()
	if entry, ok := backtestCache[ticker]; ok && time.Since(entry.stored) < backtestCacheTTL && entry.start <= start && entry.end >= end {
		backtestCacheMu.Unlock()
		return entry.series, nil
	}
	backtestCacheMu.Unlock()

	candles, err := priceStore.candles(ticker, start, end, priceBucket{seconds: 86400, expr: epochBucketExpr(86400)})
	if err != nil {
		return nil, err
	}
	if len(candles) == 0 {
		return nil, nil
	}

	series := make([]Price, len(candles))
	for i, c := range candles {
		series[i] = Price{Ticker: ticker, Date: c.Timestamp, Open: c.Open, Close: c.Close, High: c.High, Low: c.Low, Volume: c.Volume}
	}

	backtestCacheMu.Lock()
	if len(backtestCache) > 256 {
		backtestCache = map[string]backtestCacheEntry{}
	}
	backtestCache[ticker] = backtestCacheEntry{series: series, stored: time.Now(), start: start, end: end}
	backtestCacheMu.Unlock()

	return series, nil
}

// priceIntervalSupported reports whether an API interval maps to a stored bar size.
func priceIntervalSupported(interval string) bool {
	switch interval {
	case "5m", "15m", "1h", "4h", "1d", "1w", "1M":
		return true
	default:
		return false
	}
}

// priceCandlesFromStore returns stored candles for a symbol and interval, or nil when the store has
// nothing for it (the caller then falls back to the live data source).
func priceCandlesFromStore(identifier string, interval string) ([]PriceCandle, error) {
	if priceStore == nil || !priceIntervalSupported(interval) {
		return nil, nil
	}

	ticker := identifier
	if resolved, err := resolveTickerOrISIN(identifier); err == nil && resolved != "" {
		ticker = resolved
	}

	bucket, _ := priceIntervalSpec(interval)
	now := time.Now().UTC()
	lookback := tradingViewLookback(interval)
	var start int64
	if lookback == 0 {
		start = time.Time{}.Unix()
	} else {
		start = now.Add(-time.Duration(lookback) * time.Second).Unix()
	}

	return priceStore.candles(ticker, start, now.Unix(), bucket)
}

// The live data source serves these windows per interval, so stored data mirrors them to keep the
// trading view charts the same length.
func tradingViewLookback(interval string) int64 {
	switch interval {
	case "5m", "15m":
		return 60 * 24 * 3600
	case "1h":
		return 730 * 24 * 3600
	case "1d":
		return 3650 * 24 * 3600
	default:
		return 0
	}
}

// fetchAndStorePriceSeries pulls the full history from the Python data source for a ticker that has
// no stored bars yet, persists it and returns the daily series for the requested range.
func fetchAndStorePriceSeries(ticker string, start, end int64) ([]Price, error) {
	prices, historicErr := getOldHistoricPriceData(ticker)
	if historicErr != nil {
		log.Printf("prices: historic backfill for %s failed: %v", ticker, historicErr)
	}
	if len(prices) == 0 {
		livePrices, liveErr := getLivePriceSeries(ticker)
		if liveErr != nil {
			log.Printf("prices: live backfill for %s failed: %v", ticker, liveErr)
		} else {
			prices = livePrices
		}
	}
	if len(prices) == 0 {
		if historicErr != nil {
			return nil, historicErr
		}
		log.Printf("prices: data source returned no bars for %s", ticker)
		return nil, nil
	}
	if err := priceStore.upsertPrices(prices); err != nil {
		log.Printf("prices: could not store backfilled history for %s: %v", ticker, err)
	} else {
		log.Printf("prices: backfilled %d bars for %s from the data source", len(prices), ticker)
	}
	return backtestPriceSeries(ticker, start, end)
}

// getLivePriceSeries reads the hourly bars of the last 730 days from the live price endpoint that the
// periodic price job already uses, so backfilling does not depend on the historic route alone.
func getLivePriceSeries(ticker string) ([]Price, error) {
	target := fmt.Sprintf("%s/get_price?ticker=%s&last_updates_unix_timestamp=0&interval=1h", BASE_URL, url.QueryEscape(ticker))
	resp, err := pythonHistoricClient.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("live price source returned %s", resp.Status)
	}

	var payload []struct {
		Timestamp int64   `json:"timestamp"`
		Open      float64 `json:"open"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Close     float64 `json:"close"`
		Volume    float64 `json:"volume"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	prices := make([]Price, 0, len(payload))
	for _, candle := range payload {
		prices = append(prices, Price{
			Ticker: ticker,
			Date:   candle.Timestamp,
			Open:   candle.Open,
			High:   candle.High,
			Low:    candle.Low,
			Close:  candle.Close,
			Volume: int64(candle.Volume),
		})
	}
	return prices, nil
}

func dailyCloses(tickers []string, start, end int64) (map[string][]PriceCandle, error) {
	bucket := priceBucket{seconds: 86400, expr: epochBucketExpr(86400)}
	return priceStore.candlesMulti(tickers, start, end, bucket)
}

// compactTicker folds every bar older than cutoff into buckets of bucketSecs, keeping open/high/low/close/sum(volume).
func (s *PriceStore) compactTicker(ticker string, cutoff int64, bucket priceBucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var existing int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM prices WHERE ticker = ? AND date < ?`, ticker, cutoff).Scan(&existing); err != nil {
		return err
	}
	if existing == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	const tempTable = "compact_right_agg_tmp"

	create := fmt.Sprintf(`
		CREATE OR REPLACE TEMP TABLE %s AS
		SELECT %s AS bucket_ts,
		       arg_min(open, date) AS open,
		       max(high) AS high,
		       min(low) AS low,
		       arg_max(close, date) AS close,
		       CAST(COALESCE(sum(volume), 0) AS BIGINT) AS volume
		FROM prices
		WHERE ticker = ? AND date < ?
		GROUP BY 1
	`, tempTable, bucket.expr)
	if _, err := tx.Exec(create, ticker, cutoff); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM prices WHERE ticker = ? AND date < ?`, ticker, cutoff); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO prices (ticker, date, open, high, low, close, volume) SELECT ?, bucket_ts, open, high, low, close, volume FROM %s`, tempTable), ticker); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`DROP TABLE %s`, tempTable)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
