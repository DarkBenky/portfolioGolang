package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*DB, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL&_cache_size=-16000&_mmap_size=67108864&_temp_store=MEMORY"
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(4)
	conn.SetConnMaxLifetime(30 * time.Minute)
	conn.SetConnMaxIdleTime(5 * time.Minute)
	db := &DB{DB: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (d *DB) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY, user_name TEXT NOT NULL, email TEXT NOT NULL UNIQUE, password TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS holdings (
			id_holding TEXT PRIMARY KEY, name TEXT NOT NULL, ticker TEXT NOT NULL, isin TEXT, exchange TEXT,
			etf BOOLEAN DEFAULT 0, quantity REAL NOT NULL, purchase_price REAL NOT NULL, ter REAL, policy TEXT,
			user_id TEXT NOT NULL, currency TEXT, FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS assets (
			id_asset TEXT PRIMARY KEY, name TEXT NOT NULL, ticker TEXT NOT NULL, isin TEXT, exchange TEXT,
			sector TEXT, region TEXT, id_holding TEXT NOT NULL, currency TEXT,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS news (
			id_news TEXT PRIMARY KEY, title TEXT NOT NULL, link TEXT, published_at TEXT, summary TEXT,
			text TEXT UNIQUE, author TEXT, sentiment REAL, ticker TEXT, id_asset TEXT, id_holding TEXT,
			FOREIGN KEY (id_asset) REFERENCES assets(id_asset) ON DELETE CASCADE,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS regions (name TEXT, percentage REAL, id_holding TEXT)`,
		`CREATE TABLE IF NOT EXISTS sectors (name TEXT, id_holding TEXT, percentage REAL)`,
		`CREATE TABLE IF NOT EXISTS prices (
			ticker TEXT NOT NULL, date INTEGER NOT NULL, open REAL NOT NULL, close REAL NOT NULL,
			high REAL NOT NULL, low REAL NOT NULL, volume INTEGER NOT NULL,
			PRIMARY KEY (ticker, date)) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS daily_sentiment (
			id_sentiment TEXT PRIMARY KEY, ticker TEXT NOT NULL, date TEXT NOT NULL, summary TEXT NOT NULL,
			sentiment REAL NOT NULL, UNIQUE(ticker, date))`,
		`CREATE TABLE IF NOT EXISTS portfolio_daily_sentiment (
			id_sentiment TEXT PRIMARY KEY, user_id TEXT NOT NULL, date TEXT NOT NULL, summary TEXT NOT NULL,
			sentiment REAL NOT NULL, UNIQUE(user_id, date))`,
		`CREATE TABLE IF NOT EXISTS running_summary (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, window_days INTEGER NOT NULL DEFAULT 30, date TEXT NOT NULL,
			summary TEXT NOT NULL, sentiment REAL NOT NULL, sector_predictions TEXT, theme_predictions TEXT,
			UNIQUE(user_id, window_days, date))`,
		`CREATE TABLE IF NOT EXISTS asset_details (
			id_asset TEXT PRIMARY KEY, ticker TEXT NOT NULL, isin TEXT, market_cap TEXT, market_cap_eur TEXT,
			country TEXT, sector TEXT, eps TEXT, pb_ratio TEXT, pe_ratio TEXT, dividend_yield TEXT,
			revenue TEXT, net_income TEXT, profit_margin TEXT, hash TEXT, date TEXT)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT NOT NULL, created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL,
			search_results TEXT, status TEXT DEFAULT '', created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS rag_chunks (
			id TEXT PRIMARY KEY, source TEXT NOT NULL, ticker TEXT, user_id TEXT, date TEXT,
			content TEXT NOT NULL, embedding TEXT NOT NULL DEFAULT '', content_hash TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS web_searches (
			query_hash TEXT PRIMARY KEY, query TEXT NOT NULL, user_id TEXT, results TEXT NOT NULL,
			created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS llm_cache (
			cache_key TEXT PRIMARY KEY, model TEXT NOT NULL, response TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0,
			cost_usd REAL NOT NULL DEFAULT 0, created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS llm_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL, kind TEXT NOT NULL, model TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL, completion_tokens INTEGER NOT NULL, cost_usd REAL NOT NULL,
			created_at INTEGER NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := d.Exec(statement); err != nil {
			return fmt.Errorf("store: schema: %w", err)
		}
	}

	// Columns added after the original schema; failures mean they already exist.
	d.Exec(`ALTER TABLE chat_messages ADD COLUMN search_results TEXT`)
	d.Exec(`ALTER TABLE chat_messages ADD COLUMN status TEXT DEFAULT ''`)

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_conversations_user_id ON conversations(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation_id ON chat_messages(conversation_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_rag_chunks_ticker ON rag_chunks(ticker)`,
		`CREATE INDEX IF NOT EXISTS idx_rag_chunks_user_id ON rag_chunks(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_rag_chunks_date ON rag_chunks(date)`,
		`CREATE INDEX IF NOT EXISTS idx_web_searches_created_at ON web_searches(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_holdings_user_id ON holdings(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_id_holding ON assets(id_holding)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_ticker ON assets(ticker)`,
		`CREATE INDEX IF NOT EXISTS idx_news_id_asset ON news(id_asset)`,
		`CREATE INDEX IF NOT EXISTS idx_news_id_holding ON news(id_holding)`,
		`CREATE INDEX IF NOT EXISTS idx_news_ticker ON news(ticker)`,
		`CREATE INDEX IF NOT EXISTS idx_news_published ON news(published_at)`,
		`CREATE INDEX IF NOT EXISTS idx_regions_id_holding ON regions(id_holding)`,
		`CREATE INDEX IF NOT EXISTS idx_sectors_id_holding ON sectors(id_holding)`,
		`CREATE INDEX IF NOT EXISTS idx_daily_sentiment_ticker_date ON daily_sentiment(ticker, date)`,
		`CREATE INDEX IF NOT EXISTS idx_portfolio_daily_sentiment_user_id ON portfolio_daily_sentiment(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_running_summary_user_window ON running_summary(user_id, window_days)`,
		`CREATE INDEX IF NOT EXISTS idx_asset_details_ticker ON asset_details(ticker)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_user_day ON llm_usage(user_id, created_at)`,
	}
	for _, statement := range indexes {
		if _, err := d.Exec(statement); err != nil {
			return fmt.Errorf("store: index: %w", err)
		}
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
