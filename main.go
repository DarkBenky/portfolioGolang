package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/bcrypt"
)

// hashPasswordWithSalt creates a SHA-256 hash of password+salt to stay within bcrypt's 72-byte limit
func hashPasswordWithSalt(password string) string {
	hash := sha256.Sum256([]byte(password + SALT))
	return hex.EncodeToString(hash[:])
}

const (
	SALT       = "7a726befdfc6eff42209898e532ac1a71fc7f20290d5c1f7dd6298e5ccab4ab1"
	JWT_SECRET = "5ce80dd0f1070f65168ad0593f1669a000adfbc54c9977331de0434d3c9319c9"
	BASE_URL   = "http://localhost:5123/api"
)

var db *DB
var dbMutex sync.Mutex

type User struct {
	userName string
	Email    string
	Password string
	Id       string
}

type JWTClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type Holding struct {
	IdHolding     string
	Name          string
	Ticker        string
	ISIN          string
	Exchange      string
	Policy        string
	userID        string
	currency      string
	Quantity      float64
	PurchasePrice float64
	TER           float64
	Etf           bool
}

type Region struct {
	Name       string
	Percentage float64
	IdHolding  string
}

type Sector struct {
	Name       string
	Percentage float64
	IdHolding  string
}

type DailySentiment struct {
	IdSentiment string
	Ticker      string
	Date        string
	Summary     string
	Sentiment   float64
}

type PortfolioDailySentiment struct {
	IdSentiment string
	UserID      string
	Date        string
	Summary     string
	Sentiment   float64
}

type Asset struct {
	IdAsset   string
	Name      string
	Ticker    string
	ISIN      string
	Exchange  string
	Sector    string
	Region    string
	idHolding string
	currency  string
}

type News struct {
	IdNews      string
	Title       string
	Link        string
	PublishedAt string
	Summary     string
	Text        string
	idAsset     string
	idHolding   string
	Ticker      string
	Sentiment   float64
}

type Price struct {
	IdPrice string
	Ticker  string
	Date    string
	Open    float64
	Close   float64
	High    float64
	Low     float64
	Volume  int64
}

func fetchPricesPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		log.Println("Starting periodic price fetch...")

		tickers, err := db.getUniqueTickers()
		if err != nil {
			log.Printf("Error getting unique tickers: %v", err)
			continue
		}
		log.Printf("Found %d unique tickers to fetch prices for", len(tickers))

		for _, tickerSymbol := range tickers {
			go func(ticker string) {
				log.Printf("Fetching prices for %s...", ticker)
				err := fetchPrices(ticker)
				if err != nil {
					log.Printf("Error fetching prices for %s: %v", ticker, err)
				} else {
					log.Printf("Successfully fetched prices for %s", ticker)
				}
			}(tickerSymbol)
			time.Sleep(250 * time.Millisecond)
		}

		log.Println("Completed periodic price fetch cycle")
	}
}

func fetchPrices(ticker string) error {
	lastTimestamp, err := db.getLastPriceTimestamp(ticker)
	if err != nil {
		log.Printf("Error getting last price timestamp for %s: %v", ticker, err)
		return err
	}

	now := time.Now().UTC().Unix()

	// If no previous data or invalid timestamp, start from 7 days ago
	if lastTimestamp == 0 || lastTimestamp > now {
		lastTimestamp = time.Now().UTC().Add(-7 * 24 * time.Hour).Unix()
		log.Printf("Using default start time for %s: %d", ticker, lastTimestamp)
	}

	baseURL := BASE_URL + "/get_price"
	params := url.Values{}
	params.Add("ticker", ticker)
	params.Add("last_updates_unix_timestamp", strconv.FormatInt(lastTimestamp, 10))
	params.Add("interval", "1m")

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	resp, err := http.Get(fullURL)
	if err != nil {
		log.Printf("Error fetching prices for %s: %v", ticker, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to fetch prices: %s", resp.Status)
		log.Printf("Failed to fetch prices for %s: %s", ticker, resp.Status)
		return err
	}

	var candles []struct {
		Timestamp int64   `json:"timestamp"`
		Open      float64 `json:"open"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Close     float64 `json:"close"`
		Volume    float64 `json:"volume"`
	}

	err = json.NewDecoder(resp.Body).Decode(&candles)
	if err != nil {
		log.Printf("Error decoding price data for %s: %v", ticker, err)
		return err
	}

	log.Printf("Fetched %d price candles for %s", len(candles), ticker)

	validCandles := 0
	for _, candle := range candles {
		// Skip future timestamps
		if candle.Timestamp > now {
			log.Printf("Warning: Skipping future timestamp %d for %s", candle.Timestamp, ticker)
			continue
		}

		// Skip invalid/zero timestamps
		if candle.Timestamp <= 0 {
			log.Printf("Warning: Skipping invalid timestamp %d for %s", candle.Timestamp, ticker)
			continue
		}

		price := Price{
			IdPrice: generateID(),
			Ticker:  ticker,
			Date:    strconv.FormatInt(candle.Timestamp, 10),
			Open:    candle.Open,
			High:    candle.High,
			Low:     candle.Low,
			Close:   candle.Close,
			Volume:  int64(candle.Volume),
		}

		err = db.addPrice(price)
		if err != nil {
			log.Printf("Error adding price data for %s at %s: %v", ticker, price.Date, err)
		} else {
			validCandles++
		}
	}

	log.Printf("Successfully processed %d valid price candles for %s", validCandles, ticker)
	return nil
}

func fetchNewsPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting periodic news fetch for all tickers...")

		tickers, err := db.getUniqueTickers()
		if err != nil {
			log.Printf("Error getting unique tickers: %v", err)
			continue
		}

		log.Printf("Found %d unique tickers to fetch news for", len(tickers))

		for _, tickerSymbol := range tickers {
			// Fetch news for each ticker in a goroutine
			go func(ticker string) {
				log.Printf("Fetching news for %s...", ticker)
				err := fetchNews(ticker, 10)
				if err != nil {
					log.Printf("Error fetching news for %s: %v", ticker, err)
				} else {
					log.Printf("Successfully fetched news for %s", ticker)
				}
			}(tickerSymbol)

			// Small delay to avoid overwhelming the API
			time.Sleep(2500 * time.Millisecond)
		}

		log.Println("Completed periodic news fetch cycle")
	}
}

func (database *DB) newsExists(title string, summary string, text string) (bool, error) {
	var count int
	// Check by title, summary, or text (if provided)
	if text != "" {
		err := database.QueryRow(`
			SELECT COUNT(1) FROM news WHERE title = ? OR summary = ? OR text = ?
		`, title, summary, text).Scan(&count)
		if err != nil {
			return false, err
		}
	} else {
		err := database.QueryRow(`
			SELECT COUNT(1) FROM news WHERE title = ? OR summary = ?
		`, title, summary).Scan(&count)
		if err != nil {
			return false, err
		}
	}
	return count > 0, nil
}

func newsExistsWithTitle(c echo.Context) error {
	title := c.QueryParam("title")
	summary := c.QueryParam("summary")
	text := c.QueryParam("text")
	exists, err := db.newsExists(title, summary, text)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error checking news existence")
	}
	return c.JSON(http.StatusOK, map[string]bool{"exists": exists})
}

func fetchNews(ticker string, numArticles int) error {
	baseURL := BASE_URL + "/fetch_news"
	params := url.Values{}
	params.Add("ticker", ticker)
	params.Add("num_articles", strconv.Itoa(numArticles))

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	resp, err := http.Get(fullURL)
	if err != nil {
		log.Printf("Error fetching news for %s: %v", ticker, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to fetch news: %s", resp.Status)
		log.Printf("Failed to fetch news for %s: %s", ticker, resp.Status)
		return err
	}

	var newsArticles []struct {
		Title       string  `json:"title"`
		Summary     string  `json:"summary"`
		Text        string  `json:"text"`
		URL         string  `json:"url"`
		PublishedAt int64   `json:"published_at"`
		Author      string  `json:"author"`
		ImgURL      string  `json:"img_url"`
		Sentiment   float64 `json:"sentiment"`
	}

	err = json.NewDecoder(resp.Body).Decode(&newsArticles)
	if err != nil {
		log.Printf("Error decoding news data for %s: %v", ticker, err)
		return err
	}

	log.Printf("Fetched %d news articles for %s", len(newsArticles), ticker)

	// Try to find assets with this ticker first
	assets, err := db.getAssetsByTicker(ticker)
	if err != nil {
		log.Printf("Error fetching assets for ticker %s: %v", ticker, err)
	}

	// Also try to find holdings with this ticker
	holdings, err := db.getHoldingsByTicker(ticker)
	if err != nil {
		log.Printf("Error fetching holdings for ticker %s: %v", ticker, err)
	}

	// Store news articles
	for _, article := range newsArticles {
		news := News{
			IdNews:      generateID(),
			Title:       article.Title,
			Link:        article.URL,
			PublishedAt: strconv.FormatInt(article.PublishedAt, 10),
			Summary:     article.Summary,
			Text:        article.Text,
			Ticker:      ticker,
			Sentiment:   article.Sentiment,
		}

		// Link to first asset if found
		if len(assets) > 0 {
			news.idAsset = assets[0].IdAsset
		}

		// Link to first holding if found
		if len(holdings) > 0 {
			news.idHolding = holdings[0].IdHolding
		}

		err = db.addNews(news)
		if err != nil {
			log.Printf("Error adding news article '%s': %v", article.Title, err)
		} else {
			log.Printf("Added news article: %s (asset: %s, holding: %s)", article.Title, news.idAsset, news.idHolding)
		}
	}

	log.Printf("Successfully processed news for %s", ticker)
	return nil
}

func fetchAndStoreETFData(holdingID, ticker, isin, name string) error {
	baseURL := BASE_URL + "/etf_data"
	params := url.Values{}
	params.Add("ticker", ticker)
	params.Add("isin", isin)
	params.Add("etf_name", name)

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	resp, err := http.Get(fullURL)
	if err != nil {
		log.Printf("Error fetching ETF data for %s: %v", ticker, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to fetch ETF data: %s", resp.Status)
		log.Printf("Failed to fetch ETF data for %s: %s", ticker, resp.Status)
		return err
	}

	var etfData struct {
		Holdings []struct {
			Name     string `json:"name"`
			Ticker   string `json:"ticker"`
			ISIN     string `json:"isin"`
			Exchange string `json:"exchange"`
			Sector   string `json:"sector"`
			Region   string `json:"region"`
		} `json:"holdings"`
		Sectors map[string]float64 `json:"sectors"`
		Regions map[string]float64 `json:"regions"`
	}

	err = json.NewDecoder(resp.Body).Decode(&etfData)
	if err != nil {
		log.Printf("Error decoding ETF data for %s: %v", ticker, err)
		return err
	}

	log.Printf("Fetched ETF data for holding ID %s: %d holdings, %d sectors, %d regions",
		holdingID, len(etfData.Holdings), len(etfData.Sectors), len(etfData.Regions))

	// Insert holdings as assets
	for _, holding := range etfData.Holdings {
		asset := Asset{
			IdAsset:   generateID(),
			Name:      holding.Name,
			Ticker:    holding.Ticker,
			ISIN:      holding.ISIN,
			Exchange:  holding.Exchange,
			Sector:    holding.Sector,
			Region:    holding.Region,
			idHolding: holdingID,
			currency:  "", // Could be extracted from holding if available
		}

		err = db.addAsset(asset)
		if err != nil {
			log.Printf("Error adding asset %s: %v", holding.Ticker, err)
		}
	}

	// Insert sectors with percentages
	for sectorName, percentage := range etfData.Sectors {
		sector := Sector{
			Name:       sectorName,
			Percentage: percentage,
			IdHolding:  holdingID,
		}
		err = db.addSector(sector)
		if err != nil {
			log.Printf("Error adding sector %s: %v", sectorName, err)
		}
	}

	// Insert regions with percentages
	for regionName, percentage := range etfData.Regions {
		region := Region{
			Name:       regionName,
			Percentage: percentage,
			IdHolding:  holdingID,
		}
		err = db.addRegion(region)
		if err != nil {
			log.Printf("Error adding region %s: %v", regionName, err)
		}
	}

	log.Printf("Successfully processed ETF data for holding ID %s", holdingID)
	return nil
}

func initDB(fakeData bool) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./portfolio.db")
	if err != nil {
		return nil, err
	}

	// Create Users table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			user_name TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Holdings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS holdings (
			id_holding TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			ticker TEXT NOT NULL,
			isin TEXT,
			exchange TEXT,
			etf BOOLEAN DEFAULT 0,
			quantity REAL NOT NULL,
			purchase_price REAL NOT NULL,
			ter REAL,
			policy TEXT,
			user_id TEXT NOT NULL,
			currency TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Assets table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS assets (
			id_asset TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			ticker TEXT NOT NULL,
			isin TEXT,
			exchange TEXT,
			sector TEXT,
			region TEXT,
			id_holding TEXT NOT NULL,
			currency TEXT,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create News table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS news (
			id_news TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			link TEXT,
			published_at TEXT,
			summary TEXT,
			text TEXT UNIQUE,
			sentiment REAL,
			ticker TEXT,
			id_asset TEXT,
			id_holding TEXT,
			FOREIGN KEY (id_asset) REFERENCES assets(id_asset) ON DELETE CASCADE,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Crete Regions table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS regions (
			name TEXT,
			percentage REAL,
			id_holding TEXT,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Sectors table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sectors (
			name TEXT,
			id_holding TEXT,
			percentage REAL,
			FOREIGN KEY (id_holding) REFERENCES holdings(id_holding) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create Prices table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS prices (
			id_price TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			date TEXT NOT NULL,
			open REAL NOT NULL,
			close REAL NOT NULL,
			high REAL NOT NULL,
			low REAL NOT NULL,
			volume INTEGER NOT NULL,
			UNIQUE(ticker, date)
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create daily sentiment/summary table for tickers
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS daily_sentiment (
			id_sentiment TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			date TEXT NOT NULL,
			summary TEXT NOT NULL,
			sentiment REAL NOT NULL,
			UNIQUE(ticker, date)
		)
	`)

	// Crete Daily Sentiment/Summary table for whole user portfolio
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS portfolio_daily_sentiment (
			id_sentiment TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			date TEXT NOT NULL,
			summary TEXT NOT NULL,
			sentiment REAL NOT NULL,
			UNIQUE(user_id, date),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)

	if err != nil {
		return nil, err
	}

	if fakeData {
		log.Println("Populating database with fake data...")
		err = populateFakeData(db)
		if err != nil {
			log.Printf("Warning: Failed to populate fake data: %v", err)
		} else {
			log.Println("Fake data populated successfully")
		}
	}

	return db, nil
}

func populateFakeData(database *sql.DB) error {
	testUserID := uuid.New().String()
	password := "test123"
	// Hash password with salt using SHA-256 first to stay within bcrypt's 72-byte limit
	passwordHash := hashPasswordWithSalt(password)
	hashed, err := bcrypt.GenerateFromPassword([]byte(passwordHash), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = database.Exec(`
		INSERT OR IGNORE INTO users (id, user_name, email, password)
		VALUES (?, ?, ?, ?)
	`, testUserID, "Test User", "test@example.com", string(hashed))
	if err != nil {
		return err
	}

	holdings := []struct {
		Name          string
		Ticker        string
		ISIN          string
		Exchange      string
		Etf           bool
		Quantity      float64
		PurchasePrice float64
		TER           float64
		Policy        string
		Currency      string
		Sectors       []struct {
			Name       string
			Percentage float64
		}
		Regions []struct {
			Name       string
			Percentage float64
		}
	}{
		{
			Name:          "Apple Inc.",
			Ticker:        "AAPL",
			ISIN:          "US0378331005",
			Exchange:      "NASDAQ",
			Etf:           false,
			Quantity:      10.0,
			PurchasePrice: 150.25,
			TER:           0.0,
			Policy:        "",
			Currency:      "USD",
			Sectors: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "Technology", Percentage: 100.0},
			},
			Regions: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "North America", Percentage: 100.0},
			},
		},
		{
			Name:          "Microsoft Corporation",
			Ticker:        "MSFT",
			ISIN:          "US5949181045",
			Exchange:      "NASDAQ",
			Etf:           false,
			Quantity:      5.0,
			PurchasePrice: 320.50,
			TER:           0.0,
			Policy:        "",
			Currency:      "USD",
			Sectors: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "Technology", Percentage: 100.0},
			},
			Regions: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "North America", Percentage: 100.0},
			},
		},
		{
			Name:          "Vanguard S&P 500 ETF",
			Ticker:        "VOO",
			ISIN:          "US9229087690",
			Exchange:      "NYSE",
			Etf:           true,
			Quantity:      25.0,
			PurchasePrice: 400.75,
			TER:           0.03,
			Policy:        "Accumulating",
			Currency:      "USD",
			Sectors: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "Technology", Percentage: 32.0},
				{Name: "Healthcare", Percentage: 12.5},
				{Name: "Financial Services", Percentage: 12.0},
				{Name: "Consumer Cyclical", Percentage: 10.5},
				{Name: "Communication Services", Percentage: 9.0},
				{Name: "Industrials", Percentage: 8.5},
				{Name: "Consumer Defensive", Percentage: 6.0},
				{Name: "Energy", Percentage: 4.0},
				{Name: "Utilities", Percentage: 2.5},
				{Name: "Real Estate", Percentage: 2.0},
				{Name: "Basic Materials", Percentage: 1.0},
			},
			Regions: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "North America", Percentage: 100.0},
			},
		},
		{
			Name:          "iShares MSCI World ETF",
			Ticker:        "URTH",
			ISIN:          "US4642874329",
			Exchange:      "NYSE",
			Etf:           true,
			Quantity:      15.0,
			PurchasePrice: 125.30,
			TER:           0.24,
			Policy:        "Distributing",
			Currency:      "USD",
			Sectors: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "Technology", Percentage: 24.0},
				{Name: "Financial Services", Percentage: 15.0},
				{Name: "Healthcare", Percentage: 12.0},
				{Name: "Consumer Cyclical", Percentage: 11.0},
				{Name: "Industrials", Percentage: 10.5},
				{Name: "Communication Services", Percentage: 7.5},
				{Name: "Consumer Defensive", Percentage: 7.0},
				{Name: "Energy", Percentage: 5.0},
				{Name: "Basic Materials", Percentage: 4.0},
				{Name: "Utilities", Percentage: 2.5},
				{Name: "Real Estate", Percentage: 1.5},
			},
			Regions: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "North America", Percentage: 70.0},
				{Name: "Europe", Percentage: 17.0},
				{Name: "Asia Pacific", Percentage: 10.0},
				{Name: "Other", Percentage: 3.0},
			},
		},
		{
			Name:          "Tesla Inc.",
			Ticker:        "TSLA",
			ISIN:          "US88160R1014",
			Exchange:      "NASDAQ",
			Etf:           false,
			Quantity:      8.0,
			PurchasePrice: 245.80,
			TER:           0.0,
			Policy:        "",
			Currency:      "USD",
			Sectors: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "Consumer Cyclical", Percentage: 100.0},
			},
			Regions: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "North America", Percentage: 100.0},
			},
		},
		{
			Name:          "Bitcoin",
			Ticker:        "BTC-USD",
			ISIN:          "",
			Exchange:      "CRYPTO",
			Etf:           false,
			Quantity:      0.1,
			PurchasePrice: 45000.00,
			TER:           0.0,
			Policy:        "",
			Currency:      "USD",
			Sectors: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "Cryptocurrency", Percentage: 100.0},
			},
			Regions: []struct {
				Name       string
				Percentage float64
			}{
				{Name: "Global", Percentage: 100.0},
			},
		},
	}

	for _, h := range holdings {
		holdingID := uuid.New().String()
		_, err = database.Exec(`
			INSERT INTO holdings (id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, holdingID, h.Name, h.Ticker, h.ISIN, h.Exchange, h.Etf, h.Quantity, h.PurchasePrice, h.TER, h.Policy, testUserID, h.Currency)
		if err != nil {
			log.Printf("Error inserting holding %s: %v", h.Name, err)
			continue
		}
		log.Printf("Added holding: %s (%s)", h.Name, h.Ticker)

		// Add sectors for this holding
		for _, sector := range h.Sectors {
			_, err = database.Exec(`
				INSERT INTO sectors (name, id_holding, percentage)
				VALUES (?, ?, ?)
			`, sector.Name, holdingID, sector.Percentage)
			if err != nil {
				log.Printf("Error inserting sector %s for %s: %v", sector.Name, h.Name, err)
			}
		}

		// Add regions for this holding
		for _, region := range h.Regions {
			_, err = database.Exec(`
				INSERT INTO regions (name, id_holding, percentage)
				VALUES (?, ?, ?)
			`, region.Name, holdingID, region.Percentage)
			if err != nil {
				log.Printf("Error inserting region %s for %s: %v", region.Name, h.Name, err)
			}
		}
	}

	log.Println("Test user created - Email: test@example.com, Password: test123")
	return nil
}

type DB struct {
	*sql.DB
}

func (database *DB) addUser(user User) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO users (id, user_name, email, password)
		VALUES (?, ?, ?, ?)
	`, user.Id, user.userName, user.Email, user.Password)
	return err
}

func (database *DB) getUserByEmail(email string) (*User, error) {
	var user User
	err := database.QueryRow(`
		SELECT id, user_name, email, password FROM users WHERE email = ?
	`, email).Scan(&user.Id, &user.userName, &user.Email, &user.Password)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (database *DB) verifyUser(email string, password string) (bool, error) {
	var hashedPassword string
	err := database.QueryRow(`
		SELECT password FROM users WHERE email = ?
	`, email).Scan(&hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	// Hash password with salt using SHA-256 first to match registration
	passwordHash := hashPasswordWithSalt(password)
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(passwordHash))
	if err != nil {
		return false, nil
	}

	return true, nil
}

func (database *DB) addHolding(holding Holding) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	var existingHolding Holding
	err := database.QueryRow(`
		SELECT id_holding, quantity, purchase_price 
		FROM holdings 
		WHERE user_id = ? AND ticker = ? AND exchange = ?
	`, holding.userID, holding.Ticker, holding.Exchange).Scan(
		&existingHolding.IdHolding,
		&existingHolding.Quantity,
		&existingHolding.PurchasePrice,
	)

	if err == sql.ErrNoRows {
		_, err := database.Exec(`
			INSERT INTO holdings (id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, holding.IdHolding, holding.Name, holding.Ticker, holding.ISIN, holding.Exchange, holding.Etf, holding.Quantity, holding.PurchasePrice, holding.TER, holding.Policy, holding.userID, holding.currency)
		return err
	} else if err != nil {
		return err
	}

	totalCost := (existingHolding.Quantity * existingHolding.PurchasePrice) + (holding.Quantity * holding.PurchasePrice)
	newQuantity := existingHolding.Quantity + holding.Quantity
	newAvgPrice := totalCost / newQuantity

	_, err = database.Exec(`
		UPDATE holdings 
		SET quantity = ?, purchase_price = ?, name = ?, isin = ?, ter = ?, policy = ?, currency = ?
		WHERE id_holding = ?
	`, newQuantity, newAvgPrice, holding.Name, holding.ISIN, holding.TER, holding.Policy, holding.currency, existingHolding.IdHolding)

	return err
}

func (database *DB) removeHolding(holdingID string, userID string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	result, err := database.Exec(`
		DELETE FROM holdings 
		WHERE id_holding = ? AND user_id = ?
	`, holdingID, userID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (database *DB) modifyHolding(holding Holding) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	result, err := database.Exec(`
		UPDATE holdings 
		SET name = ?, ticker = ?, isin = ?, exchange = ?, etf = ?, quantity = ?, purchase_price = ?, ter = ?, policy = ?, currency = ?
		WHERE id_holding = ? AND user_id = ?
	`, holding.Name, holding.Ticker, holding.ISIN, holding.Exchange, holding.Etf, holding.Quantity, holding.PurchasePrice, holding.TER, holding.Policy, holding.currency, holding.IdHolding, holding.userID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (database *DB) getHoldingsByUser(userID string) ([]Holding, error) {
	rows, err := database.Query(`
		SELECT id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency
		FROM holdings
		WHERE user_id = ?
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var holdings []Holding
	for rows.Next() {
		var h Holding
		err := rows.Scan(&h.IdHolding, &h.Name, &h.Ticker, &h.ISIN, &h.Exchange, &h.Etf, &h.Quantity, &h.PurchasePrice, &h.TER, &h.Policy, &h.userID, &h.currency)
		if err != nil {
			return nil, err
		}
		holdings = append(holdings, h)
	}

	return holdings, nil
}

func (database *DB) getPortfolioDailySummary(userID string, date string) (*PortfolioDailySentiment, error) {
	var summary PortfolioDailySentiment
	err := database.QueryRow(`
		SELECT id_sentiment, user_id, date, summary, sentiment
		FROM portfolio_daily_sentiment
		WHERE user_id = ? AND date = ?
	`, userID, date).Scan(&summary.IdSentiment, &summary.UserID, &summary.Date, &summary.Summary, &summary.Sentiment)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (database *DB) getHoldingDailySummary(holdingID string, date string) (*DailySentiment, error) {
	var summary DailySentiment
	err := database.QueryRow(`
		SELECT id_sentiment, ticker, date, summary, sentiment
		FROM daily_sentiment
		WHERE ticker = ? AND date = ?
	`, holdingID, date).Scan(&summary.IdSentiment, &summary.Ticker, &summary.Date, &summary.Summary, &summary.Sentiment)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (database *DB) addAsset(asset Asset) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO assets (id_asset, name, ticker, isin, exchange, sector, region, id_holding, currency)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, asset.IdAsset, asset.Name, asset.Ticker, asset.ISIN, asset.Exchange, asset.Sector, asset.Region, asset.idHolding, asset.currency)
	return err
}

func (database *DB) addSector(sector Sector) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO sectors (name, id_holding, percentage)
		VALUES (?, ?, ?)
	`, sector.Name, sector.IdHolding, sector.Percentage)
	return err
}

func (database *DB) addRegion(region Region) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO regions (name, id_holding, percentage)
		VALUES (?, ?, ?)
	`, region.Name, region.IdHolding, region.Percentage)
	return err
}

func (database *DB) addNews(news News) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO news (id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, news.IdNews, news.Title, news.Link, news.PublishedAt, news.Summary, news.Text, news.Sentiment, news.Ticker, news.idAsset, news.idHolding)
	return err
}

func (database *DB) upsertDailySentiment(sentiment DailySentiment) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO daily_sentiment (id_sentiment, ticker, date, summary, sentiment)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(ticker, date) DO UPDATE SET
			summary = excluded.summary,
			sentiment = excluded.sentiment
	`, sentiment.IdSentiment, sentiment.Ticker, sentiment.Date, sentiment.Summary, sentiment.Sentiment)
	return err
}

func (database *DB) upsertPortfolioDailySentiment(sentiment PortfolioDailySentiment) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO portfolio_daily_sentiment (id_sentiment, user_id, date, summary, sentiment)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			summary = excluded.summary,
			sentiment = excluded.sentiment
	`, sentiment.IdSentiment, sentiment.UserID, sentiment.Date, sentiment.Summary, sentiment.Sentiment)
	return err
}

func (database *DB) getNewsForTickerToday(ticker string, todayDate string) ([]News, error) {
	rows, err := database.Query(`
		SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
		FROM news
		WHERE ticker = ? AND date(datetime(published_at, 'unixepoch')) = ?
		ORDER BY published_at DESC
	`, ticker, todayDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var newsList []News
	for rows.Next() {
		var n News
		var idAsset, idHolding sql.NullString
		err := rows.Scan(&n.IdNews, &n.Title, &n.Link, &n.PublishedAt, &n.Summary, &n.Text, &n.Sentiment, &n.Ticker, &idAsset, &idHolding)
		if err != nil {
			return nil, err
		}
		if idAsset.Valid {
			n.idAsset = idAsset.String
		}
		if idHolding.Valid {
			n.idHolding = idHolding.String
		}
		newsList = append(newsList, n)
	}
	return newsList, nil
}

func (database *DB) getAllUsers() ([]User, error) {
	rows, err := database.Query(`SELECT id, user_name, email FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		err := rows.Scan(&u.Id, &u.userName, &u.Email)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (database *DB) getAssetsByTicker(ticker string) ([]Asset, error) {
	rows, err := database.Query(`
		SELECT id_asset, name, ticker, isin, exchange, sector, region, id_holding, currency
		FROM assets
		WHERE ticker = ?
	`, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []Asset
	for rows.Next() {
		var a Asset
		err := rows.Scan(&a.IdAsset, &a.Name, &a.Ticker, &a.ISIN, &a.Exchange, &a.Sector, &a.Region, &a.idHolding, &a.currency)
		if err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

func (database *DB) getUniqueTickers() ([]string, error) {
	// Get unique tickers from both holdings and assets
	rows, err := database.Query(`
		SELECT DISTINCT ticker FROM (
			SELECT ticker FROM holdings
			UNION
			SELECT ticker FROM assets
		) AS combined_tickers
		ORDER BY ticker
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var ticker string
		err := rows.Scan(&ticker)
		if err != nil {
			return nil, err
		}
		tickers = append(tickers, ticker)
	}

	return tickers, nil
}

func (database *DB) getHoldingsByTicker(ticker string) ([]Holding, error) {
	rows, err := database.Query(`
		SELECT id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency
		FROM holdings
		WHERE ticker = ?
	`, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var holdings []Holding
	for rows.Next() {
		var h Holding
		err := rows.Scan(&h.IdHolding, &h.Name, &h.Ticker, &h.ISIN, &h.Exchange, &h.Etf, &h.Quantity, &h.PurchasePrice, &h.TER, &h.Policy, &h.userID, &h.currency)
		if err != nil {
			return nil, err
		}
		holdings = append(holdings, h)
	}
	return holdings, nil
}

func (database *DB) addPrice(price Price) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Validate timestamp
	timestamp, err := strconv.ParseInt(price.Date, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %v", err)
	}

	now := time.Now().UTC().Unix()
	if timestamp <= 0 || timestamp > now {
		return fmt.Errorf("invalid timestamp: %d (now: %d)", timestamp, now)
	}

	// Use INSERT OR IGNORE to avoid duplicates (ticker, date combination must be unique)
	_, err = database.Exec(`
		INSERT OR IGNORE INTO prices (id_price, ticker, date, open, close, high, low, volume)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, price.IdPrice, price.Ticker, price.Date, price.Open, price.Close, price.High, price.Low, price.Volume)
	return err
}

func addPriceIndexes(database *sql.DB) error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_prices_ticker ON prices(ticker)`,
		`CREATE INDEX IF NOT EXISTS idx_prices_date ON prices(date)`,
		`CREATE INDEX IF NOT EXISTS idx_prices_ticker_date ON prices(ticker, date)`,
	}

	for _, indexSQL := range indexes {
		_, err := database.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("error creating index: %v", err)
		}
	}

	log.Println("Price indexes created successfully")
	return nil
}

func (database *DB) getLastPriceTimestamp(ticker string) (int64, error) {
	var lastTimestamp sql.NullInt64
	now := time.Now().UTC().Unix()

	err := database.QueryRow(`
		SELECT MAX(CAST(date AS INTEGER))
		FROM prices
		WHERE ticker = ?
		AND CAST(date AS INTEGER) > 0
		AND CAST(date AS INTEGER) <= ?
	`, ticker, now).Scan(&lastTimestamp)

	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	if !lastTimestamp.Valid || lastTimestamp.Int64 <= 0 || lastTimestamp.Int64 > now {
		return 0, nil
	}

	return lastTimestamp.Int64, nil
}

func generateID() string {
	return uuid.New().String()
}

func generateJWT(userID, email string) (string, error) {
	claims := &JWTClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWT_SECRET))
}

func addUser(c echo.Context) error {
	var req struct {
		UserName string `json:"user_name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if req.Email == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Email and password are required"})
	}

	// Hash password with salt using SHA-256 first to stay within bcrypt's 72-byte limit
	passwordHash := hashPasswordWithSalt(req.Password)
	hashed, err := bcrypt.GenerateFromPassword([]byte(passwordHash), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error hashing password"})
	}

	user := User{
		Id:       generateID(),
		userName: req.UserName,
		Email:    req.Email,
		Password: string(hashed),
	}

	err = db.addUser(user)
	if err != nil {
		log.Printf("Error adding user: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error adding user to database"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "User added successfully"})
}

func login(c echo.Context) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	valid, err := db.verifyUser(req.Email, req.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error verifying user"})
	}
	if !valid {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
	}

	// Get user details
	user, err := db.getUserByEmail(req.Email)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error retrieving user"})
	}

	// Generate JWT token
	token, err := generateJWT(user.Id, user.Email)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error generating token"})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"token":   token,
		"email":   user.Email,
		"message": "Login successful",
	})
}

func getProfile(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)

	return c.JSON(http.StatusOK, map[string]string{
		"user_id": claims.UserID,
		"email":   claims.Email,
	})
}

func fillInBetweenPricesPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting periodic news fetch for all tickers...")

		tickers, err := db.getUniqueTickers()
		if err != nil {
			log.Printf("Error getting unique tickers: %v", err)
			continue
		}

		log.Printf("Found %d unique tickers to fetch news for", len(tickers))

		for _, tickerSymbol := range tickers {
			// Fetch news for each ticker in a goroutine
			go func(ticker string) {
				log.Printf("Filling in between prices for %s...", ticker)
				err := FillInBetweenPrices(ticker)
				if err != nil {
					log.Printf("Error filling in between prices for %s: %v", ticker, err)
				} else {
					log.Printf("Successfully filled in between prices for %s", ticker)
				}
			}(tickerSymbol)
			// Small delay to avoid overwhelming the API
			time.Sleep(2500 * time.Millisecond)
		}
	}
}

func FillInBetweenPrices(Ticker string) error {
	//  get all prices for the ticker
	rows, err := db.Query(`
		SELECT date, open, close, high, low, volume
		FROM prices
		WHERE ticker = ?
		ORDER BY CAST(date AS INTEGER) ASC
	`, Ticker)
	if err != nil {
		return err
	}
	defer rows.Close()

	var prices []Price
	for rows.Next() {
		var p Price
		err := rows.Scan(&p.Date, &p.Open, &p.Close, &p.High, &p.Low, &p.Volume)
		if err != nil {
			return err
		}
		prices = append(prices, p)
	}

	// Fill in missing dates
	for index, price := range prices {
		if index == 0 {
			continue
		}
		prevPrice := prices[index-1]
		currentDateInt, _ := strconv.ParseInt(price.Date, 10, 64)
		prevDateInt, _ := strconv.ParseInt(prevPrice.Date, 10, 64)
		if currentDateInt-prevDateInt > 60 { // more than 1 minute gap
			// Fill in missing dates
			fillsNeeded := (currentDateInt-prevDateInt)/60 - 1
			for i := int64(1); i <= fillsNeeded; i++ {
				missingDate := prevDateInt + i*60
				interpolatedOpen := prevPrice.Close + (price.Open-prevPrice.Close)*float64(i)/float64(fillsNeeded+1)
				interpolatedClose := prevPrice.Close + (price.Close-prevPrice.Close)*float64(i)/float64(fillsNeeded+1)
				interpolatedHigh := prevPrice.Close + (price.High-prevPrice.Close)*float64(i)/float64(fillsNeeded+1)
				interpolatedLow := prevPrice.Close + (price.Low-prevPrice.Close)*float64(i)/float64(fillsNeeded+1)
				interpolatedVolume := int64(float64(prevPrice.Volume) + (float64(price.Volume)-float64(prevPrice.Volume))*float64(i)/float64(fillsNeeded+1))
				missingPrice := Price{
					IdPrice: generateID(),
					Ticker:  Ticker,
					Date:    strconv.FormatInt(missingDate, 10),
					Open:    interpolatedOpen,
					Close:   interpolatedClose,
					High:    interpolatedHigh,
					Low:     interpolatedLow,
					Volume:  interpolatedVolume,
				}
				err := db.addPrice(missingPrice)
				if err != nil {
					log.Printf("Error adding interpolated price for %s on %d: %v", Ticker, missingDate, err)
				}
			}
		}
	}

	return nil
}

// updateSentimentsPeriodic runs every 6 hours to generate daily summaries for tickers and portfolios
func updateSentimentsPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting periodic sentiment/summary update...")
		todayDate := time.Now().UTC().Format("2006-01-02")

		// 1. Update daily sentiment for each ticker
		tickers, err := db.getUniqueTickers()
		if err != nil {
			log.Printf("Error getting unique tickers: %v", err)
			continue
		}

		log.Printf("Updating daily sentiment for %d tickers", len(tickers))

		for _, tickerSymbol := range tickers {
			go func(tickerSym string) {
				err := updateTickerDailySentiment(tickerSym, todayDate)
				if err != nil {
					log.Printf("Error updating sentiment for %s: %v", tickerSym, err)
				} else {
					log.Printf("Successfully updated sentiment for %s", tickerSym)
				}
			}(tickerSymbol)
			time.Sleep(5 * time.Second) // Delay to avoid overwhelming Ollama
		}

		// 2. Update daily sentiment for each user's portfolio
		users, err := db.getAllUsers()
		if err != nil {
			log.Printf("Error getting users: %v", err)
			continue
		}

		log.Printf("Updating portfolio sentiment for %d users", len(users))

		for _, user := range users {
			go func(u User) {
				err := updatePortfolioDailySentiment(u.Id, todayDate)
				if err != nil {
					log.Printf("Error updating portfolio sentiment for user %s: %v", u.Id, err)
				} else {
					log.Printf("Successfully updated portfolio sentiment for user %s", u.Id)
				}
			}(user)
			time.Sleep(10 * time.Second) // Longer delay for portfolio summaries
		}

		log.Println("Completed periodic sentiment/summary update cycle")
	}
}

func updateTickerDailySentiment(tickerSymbol string, todayDate string) error {
	// Get today's news for this ticker
	newsList, err := db.getNewsForTickerToday(tickerSymbol, todayDate)
	if err != nil {
		return fmt.Errorf("error fetching news: %v", err)
	}

	if len(newsList) == 0 {
		log.Printf("No news found for %s on %s, skipping", tickerSymbol, todayDate)
		return nil
	}

	// Prepare data for API call
	var summaries []string
	var sentiments []float64
	for _, news := range newsList {
		summaries = append(summaries, news.Summary)
		sentiments = append(sentiments, news.Sentiment)
	}

	// Call Python API to generate summary
	requestBody := map[string]interface{}{
		"ticker":         tickerSymbol,
		"date":           todayDate,
		"news_list":      summaries,
		"sentiment_list": sentiments,
		"max_tokens":     2048,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %v", err)
	}

	resp, err := http.Post(BASE_URL+"/summarize_ticker", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("error calling summarize API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("summarize API returned status: %s", resp.Status)
	}

	var result struct {
		Ticker    string  `json:"ticker"`
		Date      string  `json:"date"`
		Summary   string  `json:"summary"`
		Sentiment float64 `json:"sentiment"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	// Upsert to database
	sentiment := DailySentiment{
		IdSentiment: generateID(),
		Ticker:      result.Ticker,
		Date:        result.Date,
		Summary:     result.Summary,
		Sentiment:   result.Sentiment,
	}

	return db.upsertDailySentiment(sentiment)
}

func updatePortfolioDailySentiment(userID string, todayDate string) error {
	// Get user's holdings
	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return fmt.Errorf("error fetching holdings: %v", err)
	}

	if len(holdings) == 0 {
		log.Printf("No holdings found for user %s, skipping", userID)
		return nil
	}

	// Collect news from all holdings
	var allSummaries []string
	var allSentiments []float64
	var allTickers []string

	for _, holding := range holdings {
		newsList, err := db.getNewsForTickerToday(holding.Ticker, todayDate)
		if err != nil {
			log.Printf("Error fetching news for %s: %v", holding.Ticker, err)
			continue
		}

		for _, news := range newsList {
			allSummaries = append(allSummaries, news.Summary)
			allSentiments = append(allSentiments, news.Sentiment)
			allTickers = append(allTickers, news.Ticker)
		}
	}

	if len(allSummaries) == 0 {
		log.Printf("No news found for user %s portfolio on %s, skipping", userID, todayDate)
		return nil
	}

	// Call Python API to generate portfolio summary
	requestBody := map[string]interface{}{
		"user_id":        userID,
		"date":           todayDate,
		"news_list":      allSummaries,
		"sentiment_list": allSentiments,
		"tickers_list":   allTickers,
		"max_tokens":     2048,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %v", err)
	}

	resp, err := http.Post(BASE_URL+"/summarize_portfolio", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("error calling summarize API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("summarize API returned status: %s", resp.Status)
	}

	var result struct {
		UserID    string  `json:"user_id"`
		Date      string  `json:"date"`
		Summary   string  `json:"summary"`
		Sentiment float64 `json:"sentiment"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return fmt.Errorf("error decoding response: %v", err)
	}

	// Upsert to database
	sentiment := PortfolioDailySentiment{
		IdSentiment: generateID(),
		UserID:      result.UserID,
		Date:        result.Date,
		Summary:     result.Summary,
		Sentiment:   result.Sentiment,
	}

	return db.upsertPortfolioDailySentiment(sentiment)
}

// holding actions
func AddHolding(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	Name := c.FormValue("Name")
	Ticker := c.FormValue("Ticker")
	ISIN := c.FormValue("ISIN")
	Exchange := c.FormValue("Exchange")
	ETF := c.FormValue("ETF")
	Quantity := c.FormValue("Quantity")
	PurchasePrice := c.FormValue("PurchasePrice")
	TER := c.FormValue("TER")
	Policy := c.FormValue("Policy")
	Currency := c.FormValue("Currency")

	quantity, _ := strconv.ParseFloat(Quantity, 64)
	purchasePrice, _ := strconv.ParseFloat(PurchasePrice, 64)
	ter, _ := strconv.ParseFloat(TER, 64)

	holding := Holding{
		IdHolding:     generateID(),
		Name:          Name,
		Ticker:        Ticker,
		ISIN:          ISIN,
		Exchange:      Exchange,
		Etf:           ETF == "true",
		Quantity:      quantity,
		PurchasePrice: purchasePrice,
		TER:           ter,
		Policy:        Policy,
		userID:        userID,
		currency:      Currency,
	}

	if holding.Etf {
		go func(holdingID, ticker, isin, name string) {
			log.Printf("Starting background ETF data fetch for %s...", ticker)
			err := fetchAndStoreETFData(holdingID, ticker, isin, name)
			if err != nil {
				log.Printf("Error processing ETF data for %s: %v", ticker, err)
			} else {
				log.Printf("Successfully completed background ETF data fetch for %s", ticker)
			}
		}(holding.IdHolding, holding.Ticker, holding.ISIN, holding.Name)
	}

	go func(ticker string) {
		log.Printf("Starting background news fetch for %s...", ticker)
		err := fetchNews(ticker, 10)
		if err != nil {
			log.Printf("Error fetching news for %s: %v", ticker, err)
		} else {
			log.Printf("Successfully completed background news fetch for %s", ticker)
		}
	}(holding.Ticker)

	go func(ticker string) {
		log.Printf("Starting background price fetch for %s...", ticker)
		err := fetchPrices(ticker)
		if err != nil {
			log.Printf("Error fetching prices for %s: %v", ticker, err)
		} else {
			log.Printf("Successfully completed background price fetch for %s", ticker)
		}
	}(holding.Ticker)

	err := db.addHolding(holding)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error adding holding to database")
	}

	return c.String(http.StatusOK, "Holding added successfully")
}

func RemoveHolding(c echo.Context) error {
	holdingID := c.FormValue("HoldingID")

	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	if holdingID == "" {
		return c.String(http.StatusBadRequest, "HoldingID is required")
	}

	err := db.removeHolding(holdingID, userID)
	if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "Holding not found")
	}
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error removing holding from database")
	}

	return c.String(http.StatusOK, "Holding removed successfully")
}

func ModifyHolding(c echo.Context) error {
	holdingID := c.FormValue("HoldingID")
	Name := c.FormValue("Name")
	Ticker := c.FormValue("Ticker")
	ISIN := c.FormValue("ISIN")
	Exchange := c.FormValue("Exchange")
	ETF := c.FormValue("ETF")
	Quantity := c.FormValue("Quantity")
	PurchasePrice := c.FormValue("PurchasePrice")
	TER := c.FormValue("TER")
	Policy := c.FormValue("Policy")
	Currency := c.FormValue("Currency")

	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	if holdingID == "" {
		return c.String(http.StatusBadRequest, "HoldingID is required")
	}

	quantity, _ := strconv.ParseFloat(Quantity, 64)
	purchasePrice, _ := strconv.ParseFloat(PurchasePrice, 64)
	ter, _ := strconv.ParseFloat(TER, 64)

	holding := Holding{
		IdHolding:     holdingID,
		Name:          Name,
		Ticker:        Ticker,
		ISIN:          ISIN,
		Exchange:      Exchange,
		Etf:           ETF == "true",
		Quantity:      quantity,
		PurchasePrice: purchasePrice,
		TER:           ter,
		Policy:        Policy,
		userID:        userID,
		currency:      Currency,
	}

	err := db.modifyHolding(holding)
	if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "Holding not found")
	}
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error modifying holding in database")
	}

	return c.String(http.StatusOK, "Holding modified successfully")
}

func GetHoldings(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	// Build response with sectors and regions for each holding
	type SectorData struct {
		Name       string  `json:"name"`
		Percentage float64 `json:"percentage"`
	}
	type RegionData struct {
		Name       string  `json:"name"`
		Percentage float64 `json:"percentage"`
	}
	type AssetData struct {
		IdAsset  string `json:"id_asset"`
		Name     string `json:"name"`
		Ticker   string `json:"ticker"`
		ISIN     string `json:"isin"`
		Exchange string `json:"exchange"`
		Sector   string `json:"sector"`
		Region   string `json:"region"`
	}
	type HoldingWithDetails struct {
		IdHolding     string       `json:"id_holding"`
		Name          string       `json:"name"`
		Ticker        string       `json:"ticker"`
		ISIN          string       `json:"isin"`
		Exchange      string       `json:"exchange"`
		Policy        string       `json:"policy"`
		Currency      string       `json:"currency"`
		Quantity      float64      `json:"quantity"`
		PurchasePrice float64      `json:"purchase_price"`
		TER           float64      `json:"ter"`
		Etf           bool         `json:"etf"`
		Sectors       []SectorData `json:"sectors"`
		Regions       []RegionData `json:"regions"`
		Assets        []AssetData  `json:"assets,omitempty"`
	}

	result := make([]HoldingWithDetails, 0, len(holdings))

	for _, h := range holdings {
		// Get sectors for this holding with percentages
		sectorRows, err := db.Query(`SELECT name, percentage FROM sectors WHERE id_holding = ?`, h.IdHolding)
		var sectors []SectorData
		if err == nil {
			for sectorRows.Next() {
				var name string
				var percentage float64
				if err := sectorRows.Scan(&name, &percentage); err == nil {
					sectors = append(sectors, SectorData{Name: name, Percentage: percentage})
				}
			}
			sectorRows.Close()
		}

		// Get regions for this holding with percentages
		regionRows, err := db.Query(`SELECT name, percentage FROM regions WHERE id_holding = ?`, h.IdHolding)
		var regions []RegionData
		if err == nil {
			for regionRows.Next() {
				var name string
				var percentage float64
				if err := regionRows.Scan(&name, &percentage); err == nil {
					regions = append(regions, RegionData{Name: name, Percentage: percentage})
				}
			}
			regionRows.Close()
		}

		// Get assets for ETF holdings
		var assets []AssetData
		if h.Etf {
			assetRows, err := db.Query(`
				SELECT id_asset, name, ticker, isin, exchange, sector, region 
				FROM assets 
				WHERE id_holding = ?
				LIMIT 10
			`, h.IdHolding)
			if err == nil {
				for assetRows.Next() {
					var a AssetData
					if err := assetRows.Scan(&a.IdAsset, &a.Name, &a.Ticker, &a.ISIN, &a.Exchange, &a.Sector, &a.Region); err == nil {
						assets = append(assets, a)
					}
				}
				assetRows.Close()
			}
		}

		result = append(result, HoldingWithDetails{
			IdHolding:     h.IdHolding,
			Name:          h.Name,
			Ticker:        h.Ticker,
			ISIN:          h.ISIN,
			Exchange:      h.Exchange,
			Policy:        h.Policy,
			Currency:      h.currency,
			Quantity:      h.Quantity,
			PurchasePrice: h.PurchasePrice,
			TER:           h.TER,
			Etf:           h.Etf,
			Sectors:       sectors,
			Regions:       regions,
			Assets:        assets,
		})
	}

	return c.JSON(http.StatusOK, result)
}

func GetPortfolioValue(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}
	totalValue := 0.0
	for _, holding := range holdings {
		// Fetch latest price for each holding
		var latestPrice float64
		err := db.QueryRow(`
			SELECT close FROM prices 
			WHERE ticker = ? 
			ORDER BY CAST(date AS INTEGER) DESC 
			LIMIT 1
		`, holding.Ticker).Scan(&latestPrice)
		if err != nil {
			log.Printf("Error fetching latest price for %s: %v", holding.Ticker, err)
			totalValue += holding.PurchasePrice * holding.Quantity
			continue
		}
		totalValue += latestPrice * holding.Quantity
	}
	return c.JSON(http.StatusOK, map[string]float64{
		"total_value": totalValue,
	})
}

func GetPortfolioValueHistory(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	period := c.QueryParam("period")                  // e.g., "1d", "1w", "1m", "3m", "1y"
	candleInterval := c.QueryParam("candle_interval") // e.g., "1m", "5m", "1h", "1d"
	userID := claims.UserID

	// Calculate start time based on period
	now := time.Now().UTC()
	var startTime time.Time
	switch period {
	case "1d":
		startTime = now.Add(-24 * time.Hour)
	case "1w":
		startTime = now.Add(-7 * 24 * time.Hour)
	case "1m":
		startTime = now.Add(-30 * 24 * time.Hour)
	case "3m":
		startTime = now.Add(-90 * 24 * time.Hour)
	case "1y":
		startTime = now.Add(-365 * 24 * time.Hour)
	default:
		startTime = now.Add(-7 * 24 * time.Hour) // Default to 1 week
	}

	// Determine aggregation interval in seconds
	var intervalSeconds int64
	switch candleInterval {
	case "1m":
		intervalSeconds = 60
	case "5m":
		intervalSeconds = 300
	case "15m":
		intervalSeconds = 900
	case "1h":
		intervalSeconds = 3600
	case "4h":
		intervalSeconds = 14400
	case "1d":
		intervalSeconds = 86400
	default:
		intervalSeconds = 3600 // Default to 1 hour
	}

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, []map[string]interface{}{})
	}

	// Build a map of ticker -> quantity
	tickerQuantities := make(map[string]float64)
	for _, holding := range holdings {
		tickerQuantities[holding.Ticker] += holding.Quantity
	}

	// Get all unique tickers
	tickers := make([]string, 0, len(tickerQuantities))
	for ticker := range tickerQuantities {
		tickers = append(tickers, ticker)
	}

	// Fetch all prices for user's tickers within the time range
	startTimestamp := startTime.Unix()
	endTimestamp := now.Unix()

	// Build query with placeholders for all tickers
	placeholders := ""
	args := make([]interface{}, 0, len(tickers)+2)
	for i, ticker := range tickers {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, ticker)
	}
	args = append(args, startTimestamp, endTimestamp)

	query := fmt.Sprintf(`
		SELECT ticker, date, open, high, low, close, volume
		FROM prices
		WHERE ticker IN (%s)
		AND CAST(date AS INTEGER) >= ?
		AND CAST(date AS INTEGER) <= ?
		ORDER BY CAST(date AS INTEGER) ASC
	`, placeholders)

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error querying prices: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}
	defer rows.Close()

	// Group prices by timestamp bucket and ticker
	type PriceData struct {
		Open   float64
		High   float64
		Low    float64
		Close  float64
		Volume int64
	}

	// Map: bucket timestamp -> ticker -> price data
	bucketData := make(map[int64]map[string]*PriceData)

	for rows.Next() {
		var ticker, dateStr string
		var open, high, low, closePrice float64
		var volume int64

		err := rows.Scan(&ticker, &dateStr, &open, &high, &low, &closePrice, &volume)
		if err != nil {
			log.Printf("Error scanning price row: %v", err)
			continue
		}

		timestamp, _ := strconv.ParseInt(dateStr, 10, 64)
		bucket := (timestamp / intervalSeconds) * intervalSeconds

		if bucketData[bucket] == nil {
			bucketData[bucket] = make(map[string]*PriceData)
		}

		if bucketData[bucket][ticker] == nil {
			bucketData[bucket][ticker] = &PriceData{
				Open:   open,
				High:   high,
				Low:    low,
				Close:  closePrice,
				Volume: volume,
			}
		} else {
			// Aggregate within the bucket
			pd := bucketData[bucket][ticker]
			if high > pd.High {
				pd.High = high
			}
			if low < pd.Low {
				pd.Low = low
			}
			pd.Close = closePrice // Last close in the bucket
			pd.Volume += volume
		}
	}

	// Sort buckets by timestamp
	bucketTimestamps := make([]int64, 0, len(bucketData))
	for ts := range bucketData {
		bucketTimestamps = append(bucketTimestamps, ts)
	}

	// Sort timestamps
	for i := 0; i < len(bucketTimestamps)-1; i++ {
		for j := i + 1; j < len(bucketTimestamps); j++ {
			if bucketTimestamps[i] > bucketTimestamps[j] {
				bucketTimestamps[i], bucketTimestamps[j] = bucketTimestamps[j], bucketTimestamps[i]
			}
		}
	}

	// Calculate portfolio value for each bucket
	type PortfolioCandle struct {
		Timestamp int64   `json:"timestamp"`
		Open      float64 `json:"open"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Close     float64 `json:"close"`
		Volume    int64   `json:"volume"`
	}

	// Track last known prices for each ticker (for gaps in data)
	lastKnownPrices := make(map[string]*PriceData)

	result := make([]PortfolioCandle, 0, len(bucketTimestamps))

	for _, bucket := range bucketTimestamps {
		tickerPrices := bucketData[bucket]

		var portfolioOpen, portfolioHigh, portfolioLow, portfolioClose float64
		var portfolioVolume int64

		for ticker, quantity := range tickerQuantities {
			var pd *PriceData

			if tickerPrices[ticker] != nil {
				pd = tickerPrices[ticker]
				lastKnownPrices[ticker] = pd
			} else if lastKnownPrices[ticker] != nil {
				pd = lastKnownPrices[ticker]
			} else {
				continue // No price data for this ticker yet
			}

			portfolioOpen += pd.Open * quantity
			portfolioHigh += pd.High * quantity
			portfolioLow += pd.Low * quantity
			portfolioClose += pd.Close * quantity
			portfolioVolume += pd.Volume
		}

		// Only add candle if we have data
		if portfolioClose > 0 {
			result = append(result, PortfolioCandle{
				Timestamp: bucket,
				Open:      portfolioOpen,
				High:      portfolioHigh,
				Low:       portfolioLow,
				Close:     portfolioClose,
				Volume:    portfolioVolume,
			})
		}
	}

	return c.JSON(http.StatusOK, result)
}

func getPortfolioValueChange(c echo.Context) error {
	// return day change, day change percent, total change, total change percent
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"current_value":        0,
			"day_change":           0,
			"day_change_percent":   0,
			"total_change":         0,
			"total_change_percent": 0,
			"total_invested":       0,
		})
	}

	now := time.Now().UTC()
	oneDayAgo := now.Add(-24 * time.Hour).Unix()

	var currentValue float64
	var previousDayValue float64
	var totalInvested float64

	for _, holding := range holdings {
		totalInvested += holding.PurchasePrice * holding.Quantity

		// Get current (latest) price
		var latestPrice float64
		err := db.QueryRow(`
			SELECT close FROM prices 
			WHERE ticker = ? 
			ORDER BY CAST(date AS INTEGER) DESC 
			LIMIT 1
		`, holding.Ticker).Scan(&latestPrice)

		if err != nil {
			// Use purchase price as fallback
			latestPrice = holding.PurchasePrice
		}
		currentValue += latestPrice * holding.Quantity

		// Get price from ~24 hours ago (closest available)
		var previousPrice float64
		err = db.QueryRow(`
			SELECT close FROM prices 
			WHERE ticker = ? 
			AND CAST(date AS INTEGER) <= ?
			ORDER BY CAST(date AS INTEGER) DESC 
			LIMIT 1
		`, holding.Ticker, oneDayAgo).Scan(&previousPrice)

		if err != nil {
			// Use current price as fallback (no change)
			previousPrice = latestPrice
		}
		previousDayValue += previousPrice * holding.Quantity
	}

	// Calculate day change
	dayChange := currentValue - previousDayValue
	dayChangePercent := 0.0
	if previousDayValue > 0 {
		dayChangePercent = (dayChange / previousDayValue) * 100
	}

	// Calculate total change (current value vs total invested)
	totalChange := currentValue - totalInvested
	totalChangePercent := 0.0
	if totalInvested > 0 {
		totalChangePercent = (totalChange / totalInvested) * 100
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"current_value":        currentValue,
		"day_change":           dayChange,
		"day_change_percent":   dayChangePercent,
		"total_change":         totalChange,
		"total_change_percent": totalChangePercent,
		"total_invested":       totalInvested,
	})
}

func getAssetValueChange(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID
	assetTicker := c.QueryParam("ticker")

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	var totalQuantity float64
	for _, holding := range holdings {
		if holding.Ticker == assetTicker {
			totalQuantity += holding.Quantity
		}
	}

	if totalQuantity == 0 {
		return c.String(http.StatusNotFound, "No holdings found for the specified asset")
	}

	// Get latest price
	var latestPrice float64
	err = db.QueryRow(`
		SELECT close FROM prices 
		WHERE ticker = ? 
		ORDER BY CAST(date AS INTEGER) DESC 
		LIMIT 1
	`, assetTicker).Scan(&latestPrice)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving latest price")
	}

	// Get price 24 hours ago
	oneDayAgo := time.Now().UTC().Add(-24 * time.Hour).Unix()
	var previousPrice float64
	err = db.QueryRow(`
		SELECT close FROM prices 
		WHERE ticker = ?
		AND CAST(date AS INTEGER) <= ?
		ORDER BY CAST(date AS INTEGER) DESC 
		LIMIT 1
	`, assetTicker, oneDayAgo).Scan(&previousPrice)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving previous price")
	}

	currentValue := latestPrice * totalQuantity
	previousValue := previousPrice * totalQuantity
	dayChange := currentValue - previousValue
	dayChangePercent := 0.0
	if previousValue > 0 {
		dayChangePercent = (dayChange / previousValue) * 100
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"current_value":      currentValue,
		"day_change":         dayChange,
		"day_change_percent": dayChangePercent,
	})
}

func getLatestNewsForPortfolio(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	limitParam := c.QueryParam("limit")
	limit, err := strconv.Atoi(limitParam)
	if err != nil {
		limit = 10 // default limit
	}
	offsetParam := c.QueryParam("offset")
	offset, err := strconv.Atoi(offsetParam)
	if err != nil {
		offset = 0
	}
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, []News{})
	}

	// Build list of tickers from user's holdings
	tickers := make([]string, 0, len(holdings))
	for _, h := range holdings {
		tickers = append(tickers, h.Ticker)
	}

	// Build query with placeholders for tickers
	placeholders := ""
	args := make([]interface{}, 0, len(tickers)+2)
	for i, ticker := range tickers {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, ticker)
	}
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
		FROM news
		WHERE ticker IN (%s)
		ORDER BY CAST(published_at AS INTEGER) DESC
		LIMIT ? OFFSET ?
	`, placeholders)

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error querying news: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving news")
	}
	defer rows.Close()

	var newsList []News
	for rows.Next() {
		var n News
		var idAsset, idHolding sql.NullString
		err := rows.Scan(&n.IdNews, &n.Title, &n.Link, &n.PublishedAt, &n.Summary, &n.Text, &n.Sentiment, &n.Ticker, &idAsset, &idHolding)
		if err != nil {
			log.Printf("Error scanning news row: %v", err)
			continue
		}
		if idAsset.Valid {
			n.idAsset = idAsset.String
		}
		if idHolding.Valid {
			n.idHolding = idHolding.String
		}
		newsList = append(newsList, n)
	}

	return c.JSON(http.StatusOK, newsList)
}

func getLatestNewsForAsset(c echo.Context) error {
	limitParam := c.QueryParam("limit")
	limit, err := strconv.Atoi(limitParam)
	if err != nil {
		limit = 10 // default limit
	}
	offsetParam := c.QueryParam("offset")
	offset, err := strconv.Atoi(offsetParam)
	if err != nil {
		offset = 0
	}
	ticker := c.QueryParam("ticker")

	query := `
		SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
		FROM news
		WHERE ticker = ?
		ORDER BY CAST(published_at AS INTEGER) DESC
		LIMIT ? OFFSET ?
	`

	rows, err := db.Query(query, ticker, limit, offset)
	if err != nil {
		log.Printf("Error querying news: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving news")
	}
	defer rows.Close()

	var newsList []News
	for rows.Next() {
		var n News
		var idAsset, idHolding sql.NullString
		err := rows.Scan(&n.IdNews, &n.Title, &n.Link, &n.PublishedAt, &n.Summary, &n.Text, &n.Sentiment, &n.Ticker, &idAsset, &idHolding)
		if err != nil {
			log.Printf("Error scanning news row: %v", err)
			continue
		}
		if idAsset.Valid {
			n.idAsset = idAsset.String
		}
		if idHolding.Valid {
			n.idHolding = idHolding.String
		}
		newsList = append(newsList, n)
	}

	return c.JSON(http.StatusOK, newsList)
}

func getPortfolioDaySentiment(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"average_sentiment": 0,
			"news_count":        0,
		})
	}

	// Build list of tickers from user's holdings
	tickers := make([]string, 0, len(holdings))
	for _, h := range holdings {
		tickers = append(tickers, h.Ticker)
	}

	// Build query with placeholders for tickers
	placeholders := ""
	args := make([]interface{}, 0, len(tickers)+2)
	for i, ticker := range tickers {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, ticker)
	}

	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
	endOfDay := startOfDay + 86400
	args = append(args, startOfDay, endOfDay)

	query := fmt.Sprintf(`
		SELECT sentiment FROM news
		WHERE ticker IN (%s)
		AND CAST(published_at AS INTEGER) >= ?
		AND CAST(published_at AS INTEGER) < ?
		AND sentiment IS NOT NULL
	`, placeholders)

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error querying news sentiment: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving sentiment")
	}
	defer rows.Close()

	var totalSentiment float64
	var sentimentCount int

	for rows.Next() {
		var sentiment float64
		if err := rows.Scan(&sentiment); err != nil {
			continue
		}
		totalSentiment += sentiment
		sentimentCount++
	}

	averageSentiment := 0.0
	if sentimentCount > 0 {
		averageSentiment = totalSentiment / float64(sentimentCount)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"average_sentiment": averageSentiment,
		"news_count":        sentimentCount,
	})
}

func getAssetDaySentiment(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
	endOfDay := startOfDay + 86400

	query := `
		SELECT sentiment FROM news
		WHERE ticker = ?
		AND CAST(published_at AS INTEGER) >= ?
		AND CAST(published_at AS INTEGER) < ?
		AND sentiment IS NOT NULL
	`

	rows, err := db.Query(query, ticker, startOfDay, endOfDay)
	if err != nil {
		log.Printf("Error querying news sentiment: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving sentiment")
	}
	defer rows.Close()

	var totalSentiment float64
	var sentimentCount int

	for rows.Next() {
		var sentiment float64
		if err := rows.Scan(&sentiment); err != nil {
			continue
		}
		totalSentiment += sentiment
		sentimentCount++
	}

	averageSentiment := 0.0
	if sentimentCount > 0 {
		averageSentiment = totalSentiment / float64(sentimentCount)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"ticker":            ticker,
		"average_sentiment": averageSentiment,
		"news_count":        sentimentCount,
	})
}

func getPortfolioAllocation(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"sectors": []map[string]interface{}{},
			"regions": []map[string]interface{}{},
		})
	}

	// Calculate total portfolio value for weighting
	totalPortfolioValue := 0.0
	holdingValues := make(map[string]float64)

	for _, h := range holdings {
		var latestPrice float64
		err := db.QueryRow(`
			SELECT close FROM prices 
			WHERE ticker = ? 
			ORDER BY CAST(date AS INTEGER) DESC 
			LIMIT 1
		`, h.Ticker).Scan(&latestPrice)
		if err != nil {
			latestPrice = h.PurchasePrice
		}
		holdingValue := latestPrice * h.Quantity
		holdingValues[h.IdHolding] = holdingValue
		totalPortfolioValue += holdingValue
	}

	// Aggregate sectors weighted by holding value
	sectorTotals := make(map[string]float64)
	regionTotals := make(map[string]float64)

	for _, h := range holdings {
		holdingWeight := 0.0
		if totalPortfolioValue > 0 {
			holdingWeight = holdingValues[h.IdHolding] / totalPortfolioValue
		}

		// Get sectors for this holding
		sectorRows, err := db.Query(`SELECT name, percentage FROM sectors WHERE id_holding = ?`, h.IdHolding)
		if err == nil {
			for sectorRows.Next() {
				var name string
				var percentage float64
				if err := sectorRows.Scan(&name, &percentage); err == nil {
					// Weight the sector percentage by the holding's weight in portfolio
					sectorTotals[name] += percentage * holdingWeight
				}
			}
			sectorRows.Close()
		}

		// Get regions for this holding
		regionRows, err := db.Query(`SELECT name, percentage FROM regions WHERE id_holding = ?`, h.IdHolding)
		if err == nil {
			for regionRows.Next() {
				var name string
				var percentage float64
				if err := regionRows.Scan(&name, &percentage); err == nil {
					// Weight the region percentage by the holding's weight in portfolio
					regionTotals[name] += percentage * holdingWeight
				}
			}
			regionRows.Close()
		}

		// For non-ETF holdings (stocks), count them as 100% of their own sector/region if known
		if !h.Etf {
			// Add the holding itself as its own allocation
			sectorTotals["Individual Stocks"] += holdingWeight * 100
		}
	}

	// Convert to response format
	type AllocationItem struct {
		Name       string  `json:"name"`
		Percentage float64 `json:"percentage"`
	}

	sectors := make([]AllocationItem, 0)
	for name, percentage := range sectorTotals {
		sectors = append(sectors, AllocationItem{Name: name, Percentage: percentage})
	}

	regions := make([]AllocationItem, 0)
	for name, percentage := range regionTotals {
		regions = append(regions, AllocationItem{Name: name, Percentage: percentage})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total_value": totalPortfolioValue,
		"sectors":     sectors,
		"regions":     regions,
	})
}

func GetTickerValue(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	// Fetch latest price for the ticker
	var latestPrice float64
	err := db.QueryRow(`
		SELECT close FROM prices
		WHERE ticker = ?
		ORDER BY CAST(date AS INTEGER) DESC
		LIMIT 1
	`, ticker).Scan(&latestPrice)
	if err != nil {
		log.Printf("Error fetching latest price for %s: %v", ticker, err)
		return c.String(http.StatusInternalServerError, "Error retrieving latest price")
	}

	return c.JSON(http.StatusOK, map[string]float64{
		"latest_price": latestPrice,
	})
}

func GetAssetPriceHistory(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	period := c.QueryParam("period")                  // e.g., "1d", "1w", "1m", "3m", "1y"
	candleInterval := c.QueryParam("candle_interval") // e.g., "1m", "5m", "1h", "1d"

	// Calculate start time based on period
	now := time.Now().UTC()
	var startTime time.Time
	switch period {
	case "1d":
		startTime = now.Add(-24 * time.Hour)
	case "1w":
		startTime = now.Add(-7 * 24 * time.Hour)
	case "1m":
		startTime = now.Add(-30 * 24 * time.Hour)
	case "3m":
		startTime = now.Add(-90 * 24 * time.Hour)
	case "1y":
		startTime = now.Add(-365 * 24 * time.Hour)
	default:
		startTime = now.Add(-7 * 24 * time.Hour) // Default to 1 week
	}

	// Determine aggregation interval in seconds
	var intervalSeconds int64
	switch candleInterval {
	case "1m":
		intervalSeconds = 60
	case "5m":
		intervalSeconds = 300
	case "15m":
		intervalSeconds = 900
	case "1h":
		intervalSeconds = 3600
	case "4h":
		intervalSeconds = 14400
	case "1d":
		intervalSeconds = 86400
	default:
		intervalSeconds = 3600 // Default to 1 hour
	}

	startTimestamp := startTime.Unix()
	endTimestamp := now.Unix()

	query := `
		SELECT date, open, high, low, close, volume
		FROM prices
		WHERE ticker = ?
		AND CAST(date AS INTEGER) >= ?
		AND CAST(date AS INTEGER) <= ?
		ORDER BY CAST(date AS INTEGER) ASC
	`

	rows, err := db.Query(query, ticker, startTimestamp, endTimestamp)
	if err != nil {
		log.Printf("Error querying prices for %s: %v", ticker, err)
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}
	defer rows.Close()

	// Group prices by timestamp bucket
	type PriceData struct {
		Open   float64
		High   float64
		Low    float64
		Close  float64
		Volume int64
	}

	bucketData := make(map[int64]*PriceData)

	for rows.Next() {
		var dateStr string
		var open, high, low, closePrice float64
		var volume int64

		err := rows.Scan(&dateStr, &open, &high, &low, &closePrice, &volume)
		if err != nil {
			log.Printf("Error scanning price row: %v", err)
			continue
		}

		timestamp, _ := strconv.ParseInt(dateStr, 10, 64)
		bucket := (timestamp / intervalSeconds) * intervalSeconds

		if bucketData[bucket] == nil {
			bucketData[bucket] = &PriceData{
				Open:   open,
				High:   high,
				Low:    low,
				Close:  closePrice,
				Volume: volume,
			}
		} else {
			// Aggregate within the bucket
			pd := bucketData[bucket]
			if high > pd.High {
				pd.High = high
			}
			if low < pd.Low {
				pd.Low = low
			}
			pd.Close = closePrice // Last close in the bucket
			pd.Volume += volume
		}
	}

	// Sort buckets by timestamp
	bucketTimestamps := make([]int64, 0, len(bucketData))
	for ts := range bucketData {
		bucketTimestamps = append(bucketTimestamps, ts)
	}

	// Sort timestamps
	for i := 0; i < len(bucketTimestamps)-1; i++ {
		for j := i + 1; j < len(bucketTimestamps); j++ {
			if bucketTimestamps[i] > bucketTimestamps[j] {
				bucketTimestamps[i], bucketTimestamps[j] = bucketTimestamps[j], bucketTimestamps[i]
			}
		}
	}

	// Build result
	type Candle struct {
		Timestamp int64   `json:"timestamp"`
		Open      float64 `json:"open"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Close     float64 `json:"close"`
		Volume    int64   `json:"volume"`
	}

	result := make([]Candle, 0, len(bucketTimestamps))

	for _, bucket := range bucketTimestamps {
		pd := bucketData[bucket]
		result = append(result, Candle{
			Timestamp: bucket,
			Open:      pd.Open,
			High:      pd.High,
			Low:       pd.Low,
			Close:     pd.Close,
			Volume:    pd.Volume,
		})
	}

	return c.JSON(http.StatusOK, result)
}

// PortfolioStats represents statistical metrics for a portfolio
type PortfolioStats struct {
	YoYReturn        float64 `json:"yoy_return"`          // Year-over-Year Average Return (%)
	MaxDrawdown      float64 `json:"max_drawdown"`        // Maximum Drawdown (%)
	AvgDrawdown      float64 `json:"avg_drawdown"`        // Average Drawdown (%)
	SortinoRatio     float64 `json:"sortino_ratio"`       // Sortino Ratio
	AggregatedTER    float64 `json:"aggregated_ter"`      // Weighted average TER (%)
	TotalValue       float64 `json:"total_value"`         // Current portfolio value
	TotalCost        float64 `json:"total_cost"`          // Total cost basis
	TotalGainLoss    float64 `json:"total_gain_loss"`     // Total gain/loss
	TotalGainLossPct float64 `json:"total_gain_loss_pct"` // Total gain/loss percentage
}

// AssetStats represents statistical metrics for a single asset
type AssetStats struct {
	Ticker       string  `json:"ticker"`
	YoYReturn    float64 `json:"yoy_return"`    // Year-over-Year Return (%)
	MaxDrawdown  float64 `json:"max_drawdown"`  // Maximum Drawdown (%)
	AvgDrawdown  float64 `json:"avg_drawdown"`  // Average Drawdown (%)
	SortinoRatio float64 `json:"sortino_ratio"` // Sortino Ratio
	TER          float64 `json:"ter"`           // TER (%)
	CurrentPrice float64 `json:"current_price"` // Current price
	Quantity     float64 `json:"quantity"`      // Quantity held
	CurrentValue float64 `json:"current_value"` // Current value
	CostBasis    float64 `json:"cost_basis"`    // Cost basis
	GainLoss     float64 `json:"gain_loss"`     // Gain/loss
	GainLossPct  float64 `json:"gain_loss_pct"` // Gain/loss percentage
}

// calculateDrawdowns calculates max and average drawdown from a series of values
func calculateDrawdowns(values []float64) (maxDD float64, avgDD float64) {
	if len(values) < 2 {
		return 0, 0
	}

	peak := values[0]
	var drawdowns []float64

	for _, value := range values {
		if value > peak {
			peak = value
		}
		if peak > 0 {
			drawdown := (peak - value) / peak * 100
			if drawdown > 0 {
				drawdowns = append(drawdowns, drawdown)
			}
			if drawdown > maxDD {
				maxDD = drawdown
			}
		}
	}

	if len(drawdowns) > 0 {
		sum := 0.0
		for _, dd := range drawdowns {
			sum += dd
		}
		avgDD = sum / float64(len(drawdowns))
	}

	return maxDD, avgDD
}

// calculateSortinoRatio calculates the Sortino ratio from daily returns
// Sortino = (Average Return - Risk-Free Rate) / Downside Deviation
func calculateSortinoRatio(dailyReturns []float64, riskFreeRate float64) float64 {
	if len(dailyReturns) < 2 {
		return 0
	}

	// Calculate average return
	sum := 0.0
	for _, r := range dailyReturns {
		sum += r
	}
	avgReturn := sum / float64(len(dailyReturns))

	// Calculate downside deviation (only negative returns)
	var downsideSquares []float64
	for _, r := range dailyReturns {
		if r < riskFreeRate {
			diff := r - riskFreeRate
			downsideSquares = append(downsideSquares, diff*diff)
		}
	}

	if len(downsideSquares) == 0 {
		return 0 // No downside risk
	}

	downsideSum := 0.0
	for _, sq := range downsideSquares {
		downsideSum += sq
	}
	downsideDeviation := math.Sqrt(downsideSum / float64(len(downsideSquares)))

	if downsideDeviation == 0 {
		return 0
	}

	// Annualize: multiply by sqrt(252) for daily data
	annualizedReturn := avgReturn * 252
	annualizedDownside := downsideDeviation * math.Sqrt(252)

	return (annualizedReturn - riskFreeRate) / annualizedDownside
}

// calculateYoYReturn calculates Year-over-Year return from price history
func calculateYoYReturn(prices []float64, timestamps []int64) float64 {
	if len(prices) < 2 {
		return 0
	}

	// Get prices from approximately 1 year ago and now
	now := time.Now().UTC().Unix()
	oneYearAgo := now - 365*24*3600

	var startPrice, endPrice float64
	startFound := false

	for i, ts := range timestamps {
		if !startFound && ts >= oneYearAgo {
			startPrice = prices[i]
			startFound = true
		}
		endPrice = prices[i] // Last price is the end price
	}

	if !startFound || startPrice == 0 {
		// Use first available price
		startPrice = prices[0]
	}

	return (endPrice - startPrice) / startPrice * 100
}

func getPortfolioStats(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, PortfolioStats{})
	}

	// Build ticker -> quantity and ticker -> holding map
	tickerQuantities := make(map[string]float64)
	tickerHoldings := make(map[string]Holding)
	for _, holding := range holdings {
		tickerQuantities[holding.Ticker] += holding.Quantity
		tickerHoldings[holding.Ticker] = holding
	}

	tickers := make([]string, 0, len(tickerQuantities))
	for ticker := range tickerQuantities {
		tickers = append(tickers, ticker)
	}

	// Get 1 year of historical data
	now := time.Now().UTC()
	startTime := now.Add(-365 * 24 * time.Hour)

	// Query prices for all tickers
	placeholders := ""
	args := make([]interface{}, 0, len(tickers)+2)
	for i, ticker := range tickers {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, ticker)
	}
	args = append(args, startTime.Unix(), now.Unix())

	query := fmt.Sprintf(`
		SELECT ticker, date, close
		FROM prices
		WHERE ticker IN (%s)
		AND CAST(date AS INTEGER) >= ?
		AND CAST(date AS INTEGER) <= ?
		ORDER BY CAST(date AS INTEGER) ASC
	`, placeholders)

	rows, err := db.Query(query, args...)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}
	defer rows.Close()

	// Group prices by day and calculate portfolio value for each day
	type DayPrice struct {
		Ticker string
		Close  float64
	}
	dayPrices := make(map[int64][]DayPrice)

	for rows.Next() {
		var ticker, dateStr string
		var closePrice float64
		if err := rows.Scan(&ticker, &dateStr, &closePrice); err != nil {
			continue
		}
		timestamp, _ := strconv.ParseInt(dateStr, 10, 64)
		dayBucket := (timestamp / 86400) * 86400
		dayPrices[dayBucket] = append(dayPrices[dayBucket], DayPrice{ticker, closePrice})
	}

	// Sort days
	days := make([]int64, 0, len(dayPrices))
	for day := range dayPrices {
		days = append(days, day)
	}
	for i := 0; i < len(days)-1; i++ {
		for j := i + 1; j < len(days); j++ {
			if days[i] > days[j] {
				days[i], days[j] = days[j], days[i]
			}
		}
	}

	// Calculate daily portfolio values
	var portfolioValues []float64
	var timestamps []int64
	lastPrices := make(map[string]float64)

	for _, day := range days {
		// Update last known prices
		for _, dp := range dayPrices[day] {
			lastPrices[dp.Ticker] = dp.Close
		}

		// Calculate portfolio value for this day
		dayValue := 0.0
		allTickersHavePrice := true
		for ticker, qty := range tickerQuantities {
			if price, ok := lastPrices[ticker]; ok {
				dayValue += price * qty
			} else {
				allTickersHavePrice = false
			}
		}

		if allTickersHavePrice && dayValue > 0 {
			portfolioValues = append(portfolioValues, dayValue)
			timestamps = append(timestamps, day)
		}
	}

	// Calculate daily returns
	var dailyReturns []float64
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			ret := (portfolioValues[i] - portfolioValues[i-1]) / portfolioValues[i-1]
			dailyReturns = append(dailyReturns, ret)
		}
	}

	// Calculate statistics
	yoyReturn := calculateYoYReturn(portfolioValues, timestamps)
	maxDD, avgDD := calculateDrawdowns(portfolioValues)
	sortinoRatio := calculateSortinoRatio(dailyReturns, 0.0) // Assuming 0% risk-free rate

	// Calculate aggregated TER (weighted by value)
	totalValue := 0.0
	totalCost := 0.0
	weightedTER := 0.0

	for ticker, qty := range tickerQuantities {
		holding := tickerHoldings[ticker]
		currentPrice := lastPrices[ticker]
		value := currentPrice * qty
		totalValue += value
		totalCost += holding.PurchasePrice * qty
		weightedTER += holding.TER * value
	}

	aggregatedTER := 0.0
	if totalValue > 0 {
		aggregatedTER = weightedTER / totalValue
	}

	totalGainLoss := totalValue - totalCost
	totalGainLossPct := 0.0
	if totalCost > 0 {
		totalGainLossPct = totalGainLoss / totalCost * 100
	}

	stats := PortfolioStats{
		YoYReturn:        math.Round(yoyReturn*100) / 100,
		MaxDrawdown:      math.Round(maxDD*100) / 100,
		AvgDrawdown:      math.Round(avgDD*100) / 100,
		SortinoRatio:     math.Round(sortinoRatio*100) / 100,
		AggregatedTER:    math.Round(aggregatedTER*10000) / 10000,
		TotalValue:       math.Round(totalValue*100) / 100,
		TotalCost:        math.Round(totalCost*100) / 100,
		TotalGainLoss:    math.Round(totalGainLoss*100) / 100,
		TotalGainLossPct: math.Round(totalGainLossPct*100) / 100,
	}

	return c.JSON(http.StatusOK, stats)
}

func getAssetStats(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID
	ticker := c.QueryParam("ticker")

	if ticker == "" {
		return c.String(http.StatusBadRequest, "ticker parameter is required")
	}

	// Get user's holding for this ticker
	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	var holding Holding
	var totalQuantity float64
	var totalCost float64
	found := false

	for _, h := range holdings {
		if h.Ticker == ticker {
			holding = h
			totalQuantity += h.Quantity
			totalCost += h.PurchasePrice * h.Quantity
			found = true
		}
	}

	if !found {
		return c.String(http.StatusNotFound, "Holding not found")
	}

	// Get 1 year of historical data
	now := time.Now().UTC()
	startTime := now.Add(-365 * 24 * time.Hour)

	query := `
		SELECT date, close
		FROM prices
		WHERE ticker = ?
		AND CAST(date AS INTEGER) >= ?
		AND CAST(date AS INTEGER) <= ?
		ORDER BY CAST(date AS INTEGER) ASC
	`

	rows, err := db.Query(query, ticker, startTime.Unix(), now.Unix())
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving price history")
	}
	defer rows.Close()

	var prices []float64
	var timestamps []int64

	for rows.Next() {
		var dateStr string
		var closePrice float64
		if err := rows.Scan(&dateStr, &closePrice); err != nil {
			continue
		}
		timestamp, _ := strconv.ParseInt(dateStr, 10, 64)
		prices = append(prices, closePrice)
		timestamps = append(timestamps, timestamp)
	}

	if len(prices) == 0 {
		return c.String(http.StatusNotFound, "No price history found")
	}

	// Calculate daily returns
	var dailyReturns []float64
	for i := 1; i < len(prices); i++ {
		if prices[i-1] > 0 {
			ret := (prices[i] - prices[i-1]) / prices[i-1]
			dailyReturns = append(dailyReturns, ret)
		}
	}

	// Calculate statistics
	yoyReturn := calculateYoYReturn(prices, timestamps)
	maxDD, avgDD := calculateDrawdowns(prices)
	sortinoRatio := calculateSortinoRatio(dailyReturns, 0.0)

	currentPrice := prices[len(prices)-1]
	currentValue := currentPrice * totalQuantity
	gainLoss := currentValue - totalCost
	gainLossPct := 0.0
	if totalCost > 0 {
		gainLossPct = gainLoss / totalCost * 100
	}

	stats := AssetStats{
		Ticker:       ticker,
		YoYReturn:    math.Round(yoyReturn*100) / 100,
		MaxDrawdown:  math.Round(maxDD*100) / 100,
		AvgDrawdown:  math.Round(avgDD*100) / 100,
		SortinoRatio: math.Round(sortinoRatio*100) / 100,
		TER:          holding.TER,
		CurrentPrice: math.Round(currentPrice*100) / 100,
		Quantity:     totalQuantity,
		CurrentValue: math.Round(currentValue*100) / 100,
		CostBasis:    math.Round(totalCost*100) / 100,
		GainLoss:     math.Round(gainLoss*100) / 100,
		GainLossPct:  math.Round(gainLossPct*100) / 100,
	}

	return c.JSON(http.StatusOK, stats)
}

func getAllPortfolioDaySentiments(c echo.Context) error {
	// Get user ID from JWT token
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID
	date := c.QueryParam("date") // expected format: YYYY-MM-DD
	if date == "" {
		return c.String(http.StatusBadRequest, "date parameter is required")
	}

	portfolioSummary , err := db.getPortfolioDailySummary(userID, date)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving portfolio summary")
	}

	return c.JSON(http.StatusOK, portfolioSummary)
}

func getAssetDailySentimentSummary(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	date := c.QueryParam("date") // expected format: YYYY-MM-DD
	if ticker == "" || date == "" {
		return c.String(http.StatusBadRequest, "ticker and date parameters are required")
	}

	sentimentSummary, err := db.getHoldingDailySummary(ticker, date)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving asset sentiment summary")
	}

	return c.JSON(http.StatusOK, sentimentSummary)
}	

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	sqlDB, err := initDB(false)
	addPriceIndexes(sqlDB)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer sqlDB.Close()

	db = &DB{sqlDB}

	log.Println("Database initialized successfully")

	go fetchNewsPeriodic(15 * time.Minute)
	go fetchPricesPeriodic(5 * time.Minute)
	go fillInBetweenPricesPeriodic(60 * time.Minute)
	go updateSentimentsPeriodic(6 * time.Hour) // Updates daily sentiment for tickers and portfolios every 6 hours

	e := echo.New()
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete},
	}))

	e.POST("/register", addUser)
	e.POST("/login", login)
	e.GET("/news_exists", newsExistsWithTitle) // public for now TODO: secure later

	protected := e.Group("/api")
	protected.Use(echojwt.WithConfig(echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return new(JWTClaims)
		},
		SigningKey: []byte(JWT_SECRET),
	}))

	protected.GET("/profile", getProfile)

	// Portfolio endpoints
	protected.GET("portfolio/holdings", GetHoldings)
	protected.GET("/portfolio/value", GetPortfolioValue)
	protected.GET("/portfolio/history", GetPortfolioValueHistory)
	protected.GET("/portfolio/change", getPortfolioValueChange)
	protected.GET("/portfolio/news", getLatestNewsForPortfolio)
	protected.GET("/portfolio/sentiment", getPortfolioDaySentiment)
	protected.GET("/portfolio/daily_sentiment", getAllPortfolioDaySentiments)
	protected.GET("/portfolio/allocation", getPortfolioAllocation)
	protected.GET("/portfolio/stats", getPortfolioStats)

	// Asset endpoints
	protected.POST("/asset/holdings", AddHolding)
	protected.PUT("/asset/holdings", ModifyHolding)
	protected.DELETE("/asset/holdings", RemoveHolding)
	protected.GET("/asset/news", getLatestNewsForAsset)
	protected.GET("/asset/sentiment", getAssetDaySentiment)
	protected.GET("/asset/change", getAssetValueChange)
	protected.GET("/asset/value", GetTickerValue)
	protected.GET("/asset/history", GetAssetPriceHistory)
	protected.GET("/asset/stats", getAssetStats)
	protected.GET("/asset/daily_sentiment", getAssetDailySentimentSummary)

	goPort := os.Getenv("BACKEND_GO_PORT")
	fmt.Printf("Starting server on port %s...\n", goPort)
	e.Logger.Fatal(e.Start(":" + goPort))
}
