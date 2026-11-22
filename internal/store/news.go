package store

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func (d *DB) AddNews(news News) error {
	_, err := d.Exec(`INSERT OR IGNORE INTO news (id_news, title, link, published_at, summary, text, author,
		sentiment, ticker, id_asset, id_holding) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		news.IDNews, news.Title, news.Link, news.PublishedAt, news.Summary, news.Text, news.Author,
		news.Sentiment, news.Ticker, nullable(news.IDAsset), nullable(news.IDHolding))
	return err
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (d *DB) NewsExists(title string, summary string, text string) (bool, error) {
	var count int
	var err error
	if text != "" {
		err = d.QueryRow(`SELECT COUNT(1) FROM news WHERE title = ? OR summary = ? OR text = ?`, title, summary, text).Scan(&count)
	} else {
		err = d.QueryRow(`SELECT COUNT(1) FROM news WHERE title = ? OR summary = ?`, title, summary).Scan(&count)
	}
	return count > 0, err
}

const newsColumns = `id_news, COALESCE(title,''), COALESCE(link,''), COALESCE(published_at,''),
	COALESCE(summary,''), COALESCE(text,''), COALESCE(author,''), COALESCE(ticker,''), COALESCE(sentiment,0)`

func scanNews(scanner interface{ Scan(...any) error }) (News, error) {
	var news News
	err := scanner.Scan(&news.IDNews, &news.Title, &news.Link, &news.PublishedAt, &news.Summary,
		&news.Text, &news.Author, &news.Ticker, &news.Sentiment)
	return news, err
}

func (d *DB) NewsForTicker(ticker string, limit int) ([]News, error) {
	rows, err := d.Query(`SELECT `+newsColumns+` FROM news WHERE ticker = ? ORDER BY CAST(published_at AS INTEGER) DESC LIMIT ?`, ticker, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectNews(rows)
}

func (d *DB) NewsForTickerToday(ticker string, date string) ([]News, error) {
	rows, err := d.Query(`SELECT `+newsColumns+` FROM news WHERE ticker = ?
		AND date(datetime(CAST(published_at AS INTEGER), 'unixepoch')) = ? ORDER BY CAST(published_at AS INTEGER) DESC`, ticker, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectNews(rows)
}

func (d *DB) RecentNews(limit int, offset int) ([]News, error) {
	rows, err := d.Query(`SELECT `+newsColumns+` FROM news ORDER BY CAST(published_at AS INTEGER) DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectNews(rows)
}

func (d *DB) RecentNewsForTickers(tickers []string, limitPerTicker int) (map[string][]News, error) {
	result := make(map[string][]News, len(tickers))
	for _, ticker := range tickers {
		news, err := d.NewsForTicker(ticker, limitPerTicker)
		if err != nil {
			return nil, err
		}
		result[ticker] = news
	}
	return result, nil
}

func collectNews(rows *sql.Rows) ([]News, error) {
	var news []News
	for rows.Next() {
		item, err := scanNews(rows)
		if err != nil {
			return nil, err
		}
		news = append(news, item)
	}
	return news, rows.Err()
}

func (d *DB) UpsertDailySentiment(sentiment DailySentiment) error {
	_, err := d.Exec(`INSERT INTO daily_sentiment (id_sentiment, ticker, date, summary, sentiment)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(ticker, date) DO UPDATE SET
		summary = excluded.summary, sentiment = excluded.sentiment`,
		sentiment.IDSentiment, sentiment.Ticker, sentiment.Date, sentiment.Summary, sentiment.Sentiment)
	return err
}

func (d *DB) GetDailySentiment(ticker string, date string) (*DailySentiment, error) {
	var sentiment DailySentiment
	err := d.QueryRow(`SELECT id_sentiment, ticker, date, summary, sentiment FROM daily_sentiment
		WHERE ticker = ? AND date = ?`, ticker, date).
		Scan(&sentiment.IDSentiment, &sentiment.Ticker, &sentiment.Date, &sentiment.Summary, &sentiment.Sentiment)
	if err != nil {
		return nil, err
	}
	return &sentiment, nil
}

func (d *DB) DailySentimentHistory(ticker string, limit int) ([]DailySentiment, error) {
	rows, err := d.Query(`SELECT id_sentiment, ticker, date, summary, sentiment FROM daily_sentiment
		WHERE ticker = ? ORDER BY date DESC LIMIT ?`, ticker, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []DailySentiment
	for rows.Next() {
		var sentiment DailySentiment
		if err := rows.Scan(&sentiment.IDSentiment, &sentiment.Ticker, &sentiment.Date, &sentiment.Summary, &sentiment.Sentiment); err == nil {
			history = append(history, sentiment)
		}
	}
	return history, rows.Err()
}

func (d *DB) UpsertPortfolioSentiment(sentiment PortfolioDailySentiment) error {
	_, err := d.Exec(`INSERT INTO portfolio_daily_sentiment (id_sentiment, user_id, date, summary, sentiment)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(user_id, date) DO UPDATE SET
		summary = excluded.summary, sentiment = excluded.sentiment`,
		sentiment.IDSentiment, sentiment.UserID, sentiment.Date, sentiment.Summary, sentiment.Sentiment)
	return err
}

func (d *DB) GetPortfolioSentiment(userID string, date string) (*PortfolioDailySentiment, error) {
	var sentiment PortfolioDailySentiment
	err := d.QueryRow(`SELECT id_sentiment, user_id, date, summary, sentiment FROM portfolio_daily_sentiment
		WHERE user_id = ? AND date = ?`, userID, date).
		Scan(&sentiment.IDSentiment, &sentiment.UserID, &sentiment.Date, &sentiment.Summary, &sentiment.Sentiment)
	if err != nil {
		return nil, err
	}
	return &sentiment, nil
}

func (d *DB) PortfolioSentimentHistory(userID string, limit int) ([]PortfolioDailySentiment, error) {
	rows, err := d.Query(`SELECT id_sentiment, user_id, date, summary, sentiment FROM portfolio_daily_sentiment
		WHERE user_id = ? ORDER BY date DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []PortfolioDailySentiment
	for rows.Next() {
		var sentiment PortfolioDailySentiment
		if err := rows.Scan(&sentiment.IDSentiment, &sentiment.UserID, &sentiment.Date, &sentiment.Summary, &sentiment.Sentiment); err == nil {
			history = append(history, sentiment)
		}
	}
	return history, rows.Err()
}

func (d *DB) UpsertRunningSummary(summary RunningSummary) error {
	_, err := d.Exec(`INSERT INTO running_summary (id, user_id, window_days, date, summary, sentiment,
		sector_predictions, theme_predictions) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, window_days, date) DO UPDATE SET summary = excluded.summary,
		sentiment = excluded.sentiment, sector_predictions = excluded.sector_predictions,
		theme_predictions = excluded.theme_predictions`,
		summary.ID, summary.UserID, summary.WindowDays, summary.Date, summary.Summary, summary.Sentiment,
		summary.SectorPredictions, summary.ThemePredictions)
	return err
}

func (d *DB) LatestRunningSummary(userID string, windowDays int) (*RunningSummary, error) {
	var summary RunningSummary
	err := d.QueryRow(`SELECT id, user_id, window_days, date, summary, sentiment,
		COALESCE(sector_predictions,''), COALESCE(theme_predictions,'') FROM running_summary
		WHERE user_id = ? AND window_days = ? ORDER BY date DESC LIMIT 1`, userID, windowDays).
		Scan(&summary.ID, &summary.UserID, &summary.WindowDays, &summary.Date, &summary.Summary,
			&summary.Sentiment, &summary.SectorPredictions, &summary.ThemePredictions)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (d *DB) RunningSummaryHistory(userID string, windowDays int, limit int) ([]RunningSummary, error) {
	rows, err := d.Query(`SELECT id, user_id, window_days, date, summary, sentiment,
		COALESCE(sector_predictions,''), COALESCE(theme_predictions,'') FROM running_summary
		WHERE user_id = ? AND window_days = ? ORDER BY date DESC LIMIT ?`, userID, windowDays, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []RunningSummary
	for rows.Next() {
		var summary RunningSummary
		if err := rows.Scan(&summary.ID, &summary.UserID, &summary.WindowDays, &summary.Date, &summary.Summary,
			&summary.Sentiment, &summary.SectorPredictions, &summary.ThemePredictions); err == nil {
			history = append(history, summary)
		}
	}
	return history, rows.Err()
}

func (d *DB) CachedWebSearch(query string, maxAgeSeconds int64) ([]SearchResult, bool) {
	var payload string
	var createdAt int64
	err := d.QueryRow(`SELECT results, created_at FROM web_searches WHERE query_hash = ?`, query).Scan(&payload, &createdAt)
	if err != nil || time.Now().Unix()-createdAt > maxAgeSeconds {
		return nil, false
	}
	var results []SearchResult
	if err := json.Unmarshal([]byte(payload), &results); err != nil {
		return nil, false
	}
	return results, true
}

func (d *DB) SaveWebSearch(query string, userID string, results []SearchResult) error {
	payload, err := json.Marshal(results)
	if err != nil {
		return err
	}
	_, err = d.Exec(`INSERT INTO web_searches (query_hash, query, user_id, results, created_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(query_hash) DO UPDATE SET results = excluded.results,
		created_at = excluded.created_at`, query, query, userID, string(payload), Now())
	return err
}

func (d *DB) ConversationCount(userID string) (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM conversations WHERE user_id = ?`, userID).Scan(&count)
	return count, err
}

func PublishedUnix(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func (d *DB) NewsForTickers(tickers []string, limit int, offset int) ([]News, error) {
	if len(tickers) == 0 {
		return []News{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tickers)), ",")
	args := make([]any, 0, len(tickers)+2)
	for _, ticker := range tickers {
		args = append(args, ticker)
	}
	args = append(args, limit, offset)
	rows, err := d.Query(`SELECT `+newsColumns+` FROM news WHERE ticker IN (`+placeholders+`)
		ORDER BY CAST(published_at AS INTEGER) DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectNews(rows)
}

func (d *DB) NewsBetween(ticker string, startDate string, endDate string) ([]News, error) {
	rows, err := d.Query(`SELECT `+newsColumns+` FROM news WHERE (ticker = ? OR id_holding IN
		(SELECT id_holding FROM holdings WHERE ticker = ?))
		AND date(datetime(CAST(published_at AS INTEGER), 'unixepoch')) >= ?
		AND date(datetime(CAST(published_at AS INTEGER), 'unixepoch')) <= ?
		ORDER BY CAST(published_at AS INTEGER) DESC LIMIT 20`, ticker, ticker, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectNews(rows)
}

func (d *DB) AverageNewsSentiment(tickers []string, start int64, end int64) (float64, int, error) {
	if len(tickers) == 0 {
		return 0, 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tickers)), ",")
	args := make([]any, 0, len(tickers)+2)
	for _, ticker := range tickers {
		args = append(args, ticker)
	}
	args = append(args, start, end)
	var average sql.NullFloat64
	var count int
	err := d.QueryRow(`SELECT AVG(sentiment), COUNT(*) FROM news WHERE ticker IN (`+placeholders+`)
		AND CAST(published_at AS INTEGER) >= ? AND CAST(published_at AS INTEGER) < ?`,
		args...).Scan(&average, &count)
	if err != nil {
		return 0, 0, err
	}
	return average.Float64, count, nil
}

func (d *DB) DailySentimentBetween(ticker string, startDate string, endDate string) ([]DailySentiment, error) {
	rows, err := d.Query(`SELECT id_sentiment, ticker, date, summary, sentiment FROM daily_sentiment
		WHERE ticker = ? AND date >= ? AND date <= ? ORDER BY date`, ticker, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []DailySentiment
	for rows.Next() {
		var sentiment DailySentiment
		if err := rows.Scan(&sentiment.IDSentiment, &sentiment.Ticker, &sentiment.Date,
			&sentiment.Summary, &sentiment.Sentiment); err == nil {
			history = append(history, sentiment)
		}
	}
	return history, rows.Err()
}
