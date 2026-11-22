package prices

import (
	"fmt"
	"log"
	"time"
)

// Compact folds aged bars into coarser buckets. Intraday bars dominate disk usage on
// the small VM, so this keeps recent days at full resolution and older data daily.
func (s *Store) Compact(retention5mDays, retention1hDays int) error {
	now := time.Now().UTC().Unix()
	cutoffHourly := (now - int64(retention5mDays)*86400) / 3600 * 3600
	cutoffDaily := (now - int64(retention1hDays)*86400) / 86400 * 86400

	tickers, err := s.Tickers()
	if err != nil {
		return err
	}
	failures := 0
	for _, ticker := range tickers {
		// One unreadable series must not block compaction for every other ticker.
		if err := s.CompactTicker(ticker, cutoffHourly, epochBucket(3600)); err != nil {
			log.Printf("prices: compaction %s hourly: %v", ticker, err)
			failures++
			continue
		}
		if err := s.CompactTicker(ticker, cutoffDaily, epochBucket(86400)); err != nil {
			log.Printf("prices: compaction %s daily: %v", ticker, err)
			failures++
		}
	}
	log.Printf("prices: compaction finished for %d tickers (%d failures)", len(tickers), failures)
	if failures == len(tickers) {
		return fmt.Errorf("compaction failed for every ticker")
	}
	return nil
}
