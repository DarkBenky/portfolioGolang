package prices

import (
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
	for _, ticker := range tickers {
		if err := s.CompactTicker(ticker, cutoffHourly, epochBucket(3600)); err != nil {
			return err
		}
		if err := s.CompactTicker(ticker, cutoffDaily, epochBucket(86400)); err != nil {
			return err
		}
	}
	log.Printf("prices: compaction finished for %d tickers", len(tickers))
	return nil
}
