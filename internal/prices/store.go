package prices

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/marcboeker/go-duckdb/v2"
)

type Bar struct {
	Ticker string
	Date   int64
	Open   float64
	Close  float64
	High   float64
	Low    float64
	Volume int64
}

type Candle struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
}

type Store struct {
	db            *sql.DB
	path          string
	mu            sync.Mutex
	maxPoints     int
	candleCache   map[string]candleCacheEntry
	candleCacheMu sync.Mutex
}

type candleCacheEntry struct {
	key     string
	candles []Candle
	stored  time.Time
}

const (
	walProbeEnv = "PRICE_WAL_PROBE"
	upsertChunk = 5000
	upsertBatch = 500
	candleTTL   = 30 * time.Second
)

func Init(path string, legacy *sql.DB, maxPoints int) (*Store, error) {
	replayProbe(path)

	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(2)
	conn.SetMaxIdleConns(2)
	conn.SetConnMaxLifetime(time.Hour)
	execIgnore(conn, `SET threads = 2`)
	execIgnore(conn, `SET preserve_insertion_order = false`)
	execIgnore(conn, `SET memory_limit = '2GB'`)

	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS prices (
		ticker VARCHAR NOT NULL,
		date BIGINT NOT NULL,
		open DOUBLE NOT NULL,
		high DOUBLE NOT NULL,
		low DOUBLE NOT NULL,
		close DOUBLE NOT NULL,
		volume BIGINT NOT NULL,
		PRIMARY KEY (ticker, date))`)
	if err != nil {
		conn.Close()
		return nil, err
	}

	store := &Store{db: conn, path: path, maxPoints: maxPoints, candleCache: map[string]candleCacheEntry{}}
	if legacy != nil {
		if err := store.migrate(legacy); err != nil {
			return nil, err
		}
	}
	execIgnore(conn, `SET memory_limit = '512MB'`)

	var total, tickers int64
	if err := conn.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT ticker) FROM prices`).Scan(&total, &tickers); err == nil {
		log.Printf("prices: %d bars across %d tickers", total, tickers)
	}
	return store, nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	if _, err := s.db.Exec(`CHECKPOINT`); err != nil {
		log.Printf("prices: checkpoint on close failed: %v", err)
	}
	return s.db.Close()
}

func (s *Store) migrate(legacy *sql.DB) error {
	var rows int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM prices`).Scan(&rows); err != nil || rows > 0 {
		return err
	}
	var legacyRows int64
	if err := legacy.QueryRow(`SELECT COUNT(*) FROM prices`).Scan(&legacyRows); err != nil || legacyRows == 0 {
		return nil
	}
	log.Printf("prices: migrating %d legacy rows", legacyRows)

	src, err := legacy.Query(`SELECT ticker, date, open, high, low, close, volume FROM prices ORDER BY ticker, date`)
	if err != nil {
		return err
	}
	defer src.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	copied := 0
	err = conn.Raw(func(anyConn any) error {
		driverConn, ok := anyConn.(driver.Conn)
		if !ok {
			return fmt.Errorf("prices: unexpected connection type %T", anyConn)
		}
		appender, err := duckdb.NewAppenderFromConn(driverConn, "", "prices")
		if err != nil {
			return err
		}
		for src.Next() {
			var b Bar
			if err := src.Scan(&b.Ticker, &b.Date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume); err != nil {
				continue
			}
			if err := appender.AppendRow(b.Ticker, b.Date, b.Open, b.High, b.Low, b.Close, b.Volume); err != nil {
				appender.Close()
				return err
			}
			copied++
		}
		return appender.Close()
	})
	if err != nil {
		return err
	}
	log.Printf("prices: migrated %d rows", copied)
	return nil
}

func (s *Store) Upsert(bars []Bar) error {
	if len(bars) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for start := 0; start < len(bars); start += upsertChunk {
		end := min(start+upsertChunk, len(bars))
		if err := s.upsertChunk(bars[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) upsertChunk(bars []Bar) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for start := 0; start < len(bars); start += upsertBatch {
		end := min(start+upsertBatch, len(bars))
		placeholders := strings.TrimSuffix(strings.Repeat("(?,?,?,?,?,?,?),", end-start), ",")
		args := make([]any, 0, (end-start)*7)
		for _, b := range bars[start:end] {
			args = append(args, b.Ticker, b.Date, b.Open, b.Close, b.High, b.Low, b.Volume)
		}
		query := `INSERT INTO prices (ticker, date, open, close, high, low, volume) VALUES ` +
			placeholders + ` ON CONFLICT (ticker, date) DO NOTHING`
		if _, err := tx.Exec(query, args...); err != nil {
			tx.Rollback()
			return fmt.Errorf("prices: upsert rows %d-%d: %w", start, end, err)
		}
	}
	return tx.Commit()
}

func (s *Store) LastTimestamp(ticker string) (int64, error) {
	var last sql.NullInt64
	now := time.Now().UTC().Unix()
	err := s.db.QueryRow(`SELECT MAX(date) FROM prices WHERE ticker = ? AND date > 0 AND date <= ?`, ticker, now).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if !last.Valid || last.Int64 <= 0 || last.Int64 > now {
		return 0, nil
	}
	return last.Int64, nil
}

// Checkpoint folds the write ahead log into the database file so the log does not
// grow without bound on a small disk.
func (s *Store) Checkpoint() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`CHECKPOINT`)
	return err
}

func (s *Store) Tickers() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT ticker FROM prices WHERE ticker <> 'WAL_PROBE'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tickers []string
	for rows.Next() {
		var ticker string
		if err := rows.Scan(&ticker); err != nil {
			return nil, err
		}
		tickers = append(tickers, ticker)
	}
	return tickers, rows.Err()
}

func (s *Store) Candles(ticker string, start, end int64, interval string) ([]Candle, error) {
	bucket, ok := bucketFor(interval)
	if !ok {
		return nil, fmt.Errorf("prices: unsupported interval %q", interval)
	}
	key := fmt.Sprintf("%s|%d|%d|%s", ticker, start, end, interval)
	if cached, ok := s.cachedCandles(key); ok {
		return cached, nil
	}
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT %s AS bucket_ts, arg_min(open, date), max(high), min(low), arg_max(close, date),
		       CAST(COALESCE(sum(volume), 0) AS BIGINT)
		FROM prices WHERE ticker = ? AND date >= ? AND date <= ? GROUP BY 1 ORDER BY 1`, bucket.expr), ticker, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candles := make([]Candle, 0, 512)
	for rows.Next() {
		var c Candle
		if err := rows.Scan(&c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, err
		}
		candles = append(candles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	candles = Downsample(candles, s.maxPoints)
	s.storeCandles(key, candles)
	return candles, nil
}

func (s *Store) CandlesMulti(tickers []string, start, end int64, interval string) (map[string][]Candle, error) {
	if len(tickers) == 0 {
		return map[string][]Candle{}, nil
	}
	bucket, ok := bucketFor(interval)
	if !ok {
		return nil, fmt.Errorf("prices: unsupported interval %q", interval)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tickers)), ",")
	args := make([]any, 0, len(tickers)+2)
	for _, ticker := range tickers {
		args = append(args, ticker)
	}
	args = append(args, start, end)

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT ticker, %s AS bucket_ts, arg_min(open, date), max(high), min(low), arg_max(close, date),
		       CAST(COALESCE(sum(volume), 0) AS BIGINT)
		FROM prices WHERE ticker IN (%s) AND date >= ? AND date <= ? GROUP BY 1, 2 ORDER BY 2`,
		bucket.expr, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]Candle, len(tickers))
	for rows.Next() {
		var ticker string
		var c Candle
		if err := rows.Scan(&ticker, &c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, err
		}
		result[ticker] = append(result[ticker], c)
	}
	return result, rows.Err()
}

func (s *Store) LatestCloses(tickers []string) (map[string]float64, error) {
	result := make(map[string]float64, len(tickers))
	if len(tickers) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tickers)), ",")
	args := make([]any, 0, len(tickers))
	for _, ticker := range tickers {
		args = append(args, ticker)
	}
	rows, err := s.db.Query(fmt.Sprintf(`SELECT ticker, arg_max(close, date) FROM prices WHERE ticker IN (%s) GROUP BY 1`, placeholders), args...)
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

func (s *Store) LastCloseBefore(ticker string, ts int64) (float64, error) {
	var closePrice sql.NullFloat64
	if err := s.db.QueryRow(`SELECT arg_max(close, date) FROM prices WHERE ticker = ? AND date <= ?`, ticker, ts).Scan(&closePrice); err != nil {
		return 0, err
	}
	if !closePrice.Valid {
		return 0, sql.ErrNoRows
	}
	return closePrice.Float64, nil
}

func (s *Store) CloseSeries(ticker string, start, end int64) ([]float64, error) {
	rows, err := s.db.Query(`SELECT close FROM prices WHERE ticker = ? AND date >= ? AND date <= ? ORDER BY date`, ticker, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var series []float64
	for rows.Next() {
		var closePrice float64
		if err := rows.Scan(&closePrice); err != nil {
			return nil, err
		}
		series = append(series, closePrice)
	}
	return series, rows.Err()
}

func (s *Store) SeriesWithDates(ticker string, start, end int64) ([]float64, []int64, error) {
	rows, err := s.db.Query(`SELECT date, close FROM prices WHERE ticker = ? AND date >= ? AND date <= ? ORDER BY date`, ticker, start, end)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var closes []float64
	var dates []int64
	for rows.Next() {
		var date int64
		var closePrice float64
		if err := rows.Scan(&date, &closePrice); err != nil {
			return nil, nil, err
		}
		dates = append(dates, date)
		closes = append(closes, closePrice)
	}
	return closes, dates, rows.Err()
}

func (s *Store) HasDataBefore(ticker string, ts int64) (bool, error) {
	var count int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM prices WHERE ticker = ? AND date < ?`, ticker, ts).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) CompactTicker(ticker string, cutoff int64, bucket bucket) error {
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
	if _, err := tx.Exec(fmt.Sprintf(`
		CREATE OR REPLACE TEMP TABLE %s AS
		SELECT %s AS bucket_ts, arg_min(open, date) AS open, max(high) AS high, min(low) AS low,
		       arg_max(close, date) AS close, CAST(COALESCE(sum(volume), 0) AS BIGINT) AS volume
		FROM prices WHERE ticker = ? AND date < ? GROUP BY 1`, tempTable, bucket.expr), ticker, cutoff); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM prices WHERE ticker = ? AND date < ?`, ticker, cutoff); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO prices (ticker, date, open, high, low, close, volume)
		SELECT ?, bucket_ts, open, high, low, close, volume FROM %s`, tempTable), ticker); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`DROP TABLE %s`, tempTable)); err != nil {
		tx.Rollback()
		return err
	}
	s.candleCacheMu.Lock()
	s.candleCache = map[string]candleCacheEntry{}
	s.candleCacheMu.Unlock()
	return tx.Commit()
}

func (s *Store) countBefore(ticker string, cutoff int64) (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM prices WHERE ticker = ? AND date < ?`, ticker, cutoff).Scan(&count)
	return count, err
}

func (s *Store) cachedCandles(key string) ([]Candle, bool) {
	s.candleCacheMu.Lock()
	defer s.candleCacheMu.Unlock()
	entry, ok := s.candleCache[key]
	if !ok || time.Since(entry.stored) > candleTTL {
		return nil, false
	}
	return entry.candles, true
}

func (s *Store) storeCandles(key string, candles []Candle) {
	s.candleCacheMu.Lock()
	defer s.candleCacheMu.Unlock()
	if len(s.candleCache) > 512 {
		s.candleCache = map[string]candleCacheEntry{}
	}
	s.candleCache[key] = candleCacheEntry{key: key, candles: candles, stored: time.Now()}
}

type bucket struct {
	seconds int64
	expr    string
}

func bucketFor(interval string) (bucket, bool) {
	switch interval {
	case "5m":
		return epochBucket(300), true
	case "15m":
		return epochBucket(900), true
	case "1h":
		return epochBucket(3600), true
	case "4h":
		return epochBucket(14400), true
	case "1d":
		return epochBucket(86400), true
	case "1w":
		return bucket{604800, "CAST(epoch(date_trunc('week', to_timestamp(date) AT TIME ZONE 'UTC')) AS BIGINT)"}, true
	case "1M":
		return bucket{2592000, "CAST(epoch(date_trunc('month', to_timestamp(date) AT TIME ZONE 'UTC')) AS BIGINT)"}, true
	default:
		return bucket{}, false
	}
}

func epochBucket(seconds int64) bucket {
	return bucket{seconds, fmt.Sprintf("((date // %d) * %d)", seconds, seconds)}
}

// intervalLookback keeps each chart window small enough to stay useful on a phone screen
// while limiting how many intraday bars the server has to aggregate.
func IntervalLookback(interval string, lookback5mDays int) int64 {
	switch interval {
	case "5m", "15m":
		return int64(lookback5mDays) * 86400
	case "1h":
		return 120 * 86400
	case "4h":
		return 365 * 86400
	case "1d":
		return 3650 * 86400
	case "1w":
		return 730 * 86400
	default:
		return 0
	}
}

func Supported(interval string) bool {
	_, ok := bucketFor(interval)
	return ok
}

func replayProbe(path string) {
	walPath := path + ".wal"
	if _, err := os.Stat(walPath); err != nil || os.Getenv(walProbeEnv) == "1" {
		return
	}
	log.Printf("prices: probing write ahead log %s", walPath)
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), walProbeEnv+"=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		dest := fmt.Sprintf("%s.corrupt-%s", walPath, time.Now().UTC().Format("20060102T150405Z"))
		if renameErr := os.Rename(walPath, dest); renameErr != nil {
			log.Printf("prices: could not move %s aside: %v", walPath, renameErr)
			return
		}
		log.Printf("prices: unreplayable log (%v) moved to %s", err, dest)
	}
}

// RunProbe replays the log in a throwaway process so a DuckDB fault cannot kill the server.
func RunProbe(path string) int {
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return 2
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	execIgnore(conn, `SET threads = 1`)
	execIgnore(conn, `SET memory_limit = '512MB'`)
	var rows int64
	if err := conn.QueryRow(`SELECT COUNT(*) FROM prices`).Scan(&rows); err != nil {
		return 3
	}
	tx, err := conn.Begin()
	if err != nil {
		return 4
	}
	if _, err := tx.Exec(`INSERT INTO prices (ticker, date, open, close, high, low, volume)
		VALUES ('WAL_PROBE', 1, 0, 0, 0, 0, 0) ON CONFLICT (ticker, date) DO NOTHING`); err != nil {
		tx.Rollback()
		return 5
	}
	if err := tx.Commit(); err != nil {
		return 6
	}
	execIgnore(conn, `DELETE FROM prices WHERE ticker = 'WAL_PROBE'`)
	if _, err := conn.Exec(`CHECKPOINT`); err != nil {
		return 7
	}
	log.Printf("prices: log replay is clean (%d bars)", rows)
	return 0
}

func execIgnore(conn *sql.DB, statement string) {
	if _, err := conn.Exec(statement); err != nil {
		log.Printf("prices: %s failed: %v", statement, err)
	}
}

func ParseSetting(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
