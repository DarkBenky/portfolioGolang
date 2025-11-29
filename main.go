package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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
	Name      string
	IdHolding string
}

type Sector struct {
	Name      string
	IdHolding string
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
				err := fetchNews(ticker)
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

func (database *DB) newsExists(title string, summary string) (bool, error) {
	var count int
	err := database.QueryRow(`
        SELECT COUNT(1) FROM news WHERE title = ? OR summary = ?
    `, title, summary).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func newsExistsWithTitle(c echo.Context) error {
	title := c.QueryParam("title")
	summary := c.QueryParam("summary")
	exists, err := db.newsExists(title, summary)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error checking news existence")
	}
	return c.JSON(http.StatusOK, map[string]bool{"exists": exists})
}

func fetchNews(ticker string) error {
	baseURL := BASE_URL + "/fetch_news"
	params := url.Values{}
	params.Add("ticker", ticker)
	params.Add("num_articles", "5")

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

	// Insert sectors
	for sectorName := range etfData.Sectors {
		sector := Sector{
			Name:      sectorName,
			IdHolding: holdingID,
		}
		err = db.addSector(sector)
		if err != nil {
			log.Printf("Error adding sector %s: %v", sectorName, err)
		}
	}

	// Insert regions
	for regionName := range etfData.Regions {
		region := Region{
			Name:      regionName,
			IdHolding: holdingID,
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
	hashedPassword := password + SALT
	hashed, err := bcrypt.GenerateFromPassword([]byte(hashedPassword), bcrypt.DefaultCost)
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

	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password+SALT))
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
		INSERT INTO sectors (name, id_holding)
		VALUES (?, ?)
	`, sector.Name, sector.IdHolding)
	return err
}

func (database *DB) addRegion(region Region) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO regions (name, id_holding)
		VALUES (?, ?)
	`, region.Name, region.IdHolding)
	return err
}

func (database *DB) addNews(news News) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		INSERT INTO news (id_news, title, link, published_at, summary, text, sentiment, id_asset, id_holding)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, news.IdNews, news.Title, news.Link, news.PublishedAt, news.Summary, news.Text, news.Sentiment, news.idAsset, news.idHolding)
	return err
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
	userName := c.FormValue("userName")
	email := c.FormValue("email")
	password := c.FormValue("password")

	password = password + SALT
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error hashing password")
	}

	user := User{
		Id:       generateID(),
		userName: userName,
		Email:    email,
		Password: string(hashed),
	}

	err = db.addUser(user)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error adding user to database")
	}

	return c.String(http.StatusOK, "User added successfully")
}

func login(c echo.Context) error {
	email := c.FormValue("email")
	password := c.FormValue("password")

	valid, err := db.verifyUser(email, password)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error verifying user")
	}
	if !valid {
		return c.String(http.StatusUnauthorized, "Invalid email or password")
	}

	// Get user details
	user, err := db.getUserByEmail(email)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving user")
	}

	// Generate JWT token
	token, err := generateJWT(user.Id, user.Email)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error generating token")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"token":   token,
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
		err := fetchNews(ticker)
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

	return c.JSON(http.StatusOK, holdings)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	sqlDB, err := initDB(true)
	addPriceIndexes(sqlDB)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer sqlDB.Close()

	db = &DB{sqlDB}

	log.Println("Database initialized successfully")

	go fetchNewsPeriodic(15 * time.Minute)
	go fetchPricesPeriodic(5 * time.Minute)

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

	// Holdings endpoints
	protected.POST("/holdings", AddHolding)
	protected.GET("/holdings", GetHoldings)
	protected.PUT("/holdings", ModifyHolding)
	protected.DELETE("/holdings", RemoveHolding)

	goPort := os.Getenv("BACKEND_GO_PORT")
	fmt.Printf("Starting server on port %s...\n", goPort)
	e.Logger.Fatal(e.Start(":" + goPort))
}
