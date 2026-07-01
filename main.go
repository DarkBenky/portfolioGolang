package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"strings"

	"main/bills"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
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
var (
	SALT           string
	JWT_SECRET     string
	JWT_EXPIRY     = 7 * 24 * time.Hour
	BASE_URL       string
	devMode        string
	db             *DB
	dbMutex        sync.RWMutex
	candleInterval int64 = 600 // 10 minutes

	loginAttempts    = make(map[string][]time.Time)
	loginAttemptsMu  sync.Mutex
	maxLoginAttempts = 5
	loginBlockWindow = 15 * time.Minute

	lastSummaryGen   = make(map[string]time.Time)
	lastSummaryGenMu sync.Mutex
	summaryCooldown  = 30 * time.Minute

	lastTickerSummaryGen   = make(map[string]time.Time)
	lastTickerSummaryGenMu sync.Mutex
	tickerSummaryCooldown  = 15 * time.Minute
)

func hashPasswordWithSalt(password string) string {
	hash := sha256.Sum256([]byte(password + SALT))
	return hex.EncodeToString(hash[:])
}

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
	IdSentiment string  `json:"id_sentiment"`
	Ticker      string  `json:"ticker"`
	Date        string  `json:"date"`
	Summary     string  `json:"summary"`
	Sentiment   float64 `json:"sentiment"`
}

type PortfolioDailySentiment struct {
	IdSentiment string  `json:"id_sentiment"`
	UserID      string  `json:"user_id"`
	Date        string  `json:"date"`
	Summary     string  `json:"summary"`
	Sentiment   float64 `json:"sentiment"`
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

type AssetDetails struct {
	Ticker        string `json:"ticker"`
	ISIN          string `json:"isin"`
	MarketCap     string `json:"market_cap"`
	MarketCapEur  string `json:"market_cap_eur"`
	Country       string `json:"country"`
	Sector        string `json:"sector"`
	Eps           string `json:"eps"`
	PbRatio       string `json:"pb_ratio"`
	PeRatio       string `json:"pe_ratio"`
	DividendYield string `json:"dividend_yield"`
	Revenue       string `json:"revenue"`
	NetIncome     string `json:"net_income"`
	ProfitMargin  string `json:"profit_margin"`
	Hash          string `json:"hash"`
	Date          string `json:"date"`
}

type News struct {
	IdNews      string `json:"id_news"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	PublishedAt string `json:"published_at"`
	Summary     string `json:"summary"`
	Text        string `json:"text"`
	Author      string `json:"author"`
	idAsset     string
	idHolding   string
	Ticker      string  `json:"ticker"`
	Sentiment   float64 `json:"sentiment"`
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

func (detail *AssetDetails) hashDetails() string {
	var buffer bytes.Buffer
	buffer.WriteString(detail.Ticker)
	buffer.WriteString(detail.MarketCap)
	buffer.WriteString(detail.MarketCapEur)
	buffer.WriteString(detail.Country)
	buffer.WriteString(detail.Sector)
	buffer.WriteString(detail.Eps)
	buffer.WriteString(detail.PbRatio)
	buffer.WriteString(detail.PeRatio)
	buffer.WriteString(detail.DividendYield)
	buffer.WriteString(detail.Revenue)
	buffer.WriteString(detail.NetIncome)
	buffer.WriteString(detail.ProfitMargin)
	hash := sha256.Sum256(buffer.Bytes())
	return hex.EncodeToString(hash[:])
}

func (database *DB) addAssetDetails(detail AssetDetails) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO asset_details (id_asset, ticker, isin, market_cap, market_cap_eur, country, sector, eps, pb_ratio, pe_ratio, dividend_yield, revenue, net_income, profit_margin, hash, date) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, generateID(), detail.Ticker, detail.ISIN, detail.MarketCap, detail.MarketCapEur, detail.Country, detail.Sector, detail.Eps, detail.PbRatio, detail.PeRatio, detail.DividendYield, detail.Revenue, detail.NetIncome, detail.ProfitMargin, detail.Hash, detail.Date)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) getLatestAssetDetails(ticker string) (*AssetDetails, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	var detail AssetDetails
	err := database.QueryRow(`SELECT ticker, isin, market_cap, market_cap_eur, country, sector, eps, pb_ratio, pe_ratio, dividend_yield, revenue, net_income, profit_margin, hash, date FROM asset_details WHERE ticker = ? OR isin = ? ORDER BY CAST(date AS INTEGER) DESC LIMIT 1`, ticker, ticker).Scan(&detail.Ticker, &detail.ISIN, &detail.MarketCap, &detail.MarketCapEur, &detail.Country, &detail.Sector, &detail.Eps, &detail.PbRatio, &detail.PeRatio, &detail.DividendYield, &detail.Revenue, &detail.NetIncome, &detail.ProfitMargin, &detail.Hash, &detail.Date)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &detail, err
}

func (database *DB) isETF(ticker string, isin string) (bool, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	var etfFlag interface{}
	err := database.QueryRow(`SELECT etf FROM holdings WHERE ticker = ? OR isin = ? LIMIT 1`, ticker, isin).Scan(&etfFlag)
	if err != nil {
		return false, err
	}
	switch v := etfFlag.(type) {
	case int64:
		return v == 1, nil
	case bool:
		return v, nil
	case string:
		return v == "true" || v == "1", nil
	default:
		return false, nil
	}
}

func (database *DB) getUnderlyingAssetsForETF(ticker string, isin string) ([]Asset, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	rows, err := database.Query(`SELECT a.id_asset, a.name, a.ticker, a.isin, a.exchange, a.sector, a.region, a.id_holding, a.currency FROM assets a JOIN holdings h ON a.id_holding = h.id_holding WHERE h.ticker = ? OR h.isin = ?`, ticker, isin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assets []Asset
	for rows.Next() {
		var asset Asset
		err := rows.Scan(&asset.IdAsset, &asset.Name, &asset.Ticker, &asset.ISIN, &asset.Exchange, &asset.Sector, &asset.Region, &asset.idHolding, &asset.currency)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func (database *DB) getUnderlyingAssetTickers(ticker string, isin string) ([]string, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	rows, err := database.Query(`SELECT DISTINCT a.ticker FROM assets a JOIN holdings h ON a.id_holding = h.id_holding WHERE h.ticker = ? OR h.isin = ?`, ticker, isin)
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

type GainerLoser struct {
	Asset          *Asset
	Holding        *Holding
	PriceChangePct float64
	RelatedNews    []News
}

func (database *DB) topGainersLosers(userId string, topN int, interval time.Duration, isGainer bool) ([]GainerLoser, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	now := time.Now().UTC()
	startTime := now.Add(-interval)
	startTimestamp := startTime.Unix()
	endTimestamp := now.Unix()

	newsStartDate := startTime.Format("2006-01-02")
	newsEndDate := now.Format("2006-01-02")

	rows, err := database.Query(`
		SELECT DISTINCT h.id_holding, h.name, h.ticker, h.isin, h.exchange, h.policy, 
		       h.user_id, h.currency, h.quantity, h.purchase_price, h.ter, h.etf
		FROM holdings h
		WHERE h.user_id = ?
	`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []GainerLoser

	for rows.Next() {
		var holding Holding
		err := rows.Scan(&holding.IdHolding, &holding.Name, &holding.Ticker, &holding.ISIN,
			&holding.Exchange, &holding.Policy, &holding.userID, &holding.currency,
			&holding.Quantity, &holding.PurchasePrice, &holding.TER, &holding.Etf)
		if err != nil {
			continue
		}

		priceRows, err := database.Query(`
			SELECT close
			FROM prices
			WHERE ticker = ?
			AND CAST(date AS INTEGER) >= ?
			AND CAST(date AS INTEGER) <= ?
			ORDER BY CAST(date AS INTEGER) ASC
		`, holding.Ticker, startTimestamp, endTimestamp)
		if err != nil {
			continue
		}

		var prices []float64
		for priceRows.Next() {
			var price float64
			if err := priceRows.Scan(&price); err == nil {
				prices = append(prices, price)
			}
		}
		priceRows.Close()

		if len(prices) < 2 {
			continue
		}

		startPrice := prices[0]
		endPrice := prices[len(prices)-1]
		changePct := ((endPrice - startPrice) / startPrice) * 100

		assetRows, err := database.Query(`
			SELECT id_asset, name, ticker, isin, exchange, sector, region, id_holding, currency
			FROM assets
			WHERE id_holding = ?
		`, holding.IdHolding)
		if err != nil {
			continue
		}

		var assets []*Asset
		for assetRows.Next() {
			var asset Asset
			if err := assetRows.Scan(&asset.IdAsset, &asset.Name, &asset.Ticker, &asset.ISIN,
				&asset.Exchange, &asset.Sector, &asset.Region, &asset.idHolding, &asset.currency); err == nil {
				assets = append(assets, &asset)
			}
		}
		assetRows.Close()

		newsRows, err := database.Query(`
			SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
			FROM news
			WHERE (ticker = ? OR id_holding = ?)
			AND date(datetime(published_at, 'unixepoch')) >= ?
			AND date(datetime(published_at, 'unixepoch')) <= ?
			ORDER BY published_at DESC
		`, holding.Ticker, holding.IdHolding, newsStartDate, newsEndDate)

		var newsList []News
		if err == nil {
			defer newsRows.Close()
			for newsRows.Next() {
				var n News
				var idAsset, idHolding sql.NullString
				if err := newsRows.Scan(&n.IdNews, &n.Title, &n.Link, &n.PublishedAt, &n.Summary,
					&n.Text, &n.Sentiment, &n.Ticker, &idAsset, &idHolding); err == nil {
					if idAsset.Valid {
						n.idAsset = idAsset.String
					}
					if idHolding.Valid {
						n.idHolding = idHolding.String
					}
					newsList = append(newsList, n)
				}
			}
		}

		var asset *Asset
		if len(assets) > 0 {
			asset = assets[0]
		}

		results = append(results, GainerLoser{
			Asset:          asset,
			Holding:        &holding,
			PriceChangePct: changePct,
			RelatedNews:    newsList,
		})
	}

	if isGainer {
		for i := 0; i < len(results); i++ {
			for j := i + 1; j < len(results); j++ {
				if results[i].PriceChangePct < results[j].PriceChangePct {
					results[i], results[j] = results[j], results[i]
				}
			}
		}
	} else {
		for i := 0; i < len(results); i++ {
			for j := i + 1; j < len(results); j++ {
				if results[i].PriceChangePct > results[j].PriceChangePct {
					results[i], results[j] = results[j], results[i]
				}
			}
		}
	}

	if len(results) > topN {
		results = results[:topN]
	}

	return results, nil
}

func (database *DB) getAssetDetailsHistory(ticker string) ([]AssetDetails, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	rows, err := database.Query(`SELECT ticker, isin, market_cap, market_cap_eur, country, sector, eps, pb_ratio, pe_ratio, dividend_yield, revenue, net_income, profit_margin, hash, date FROM asset_details WHERE ticker = ? OR isin = ? ORDER BY CAST(date AS INTEGER) DESC`, ticker, ticker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var details []AssetDetails
	for rows.Next() {
		var detail AssetDetails
		err := rows.Scan(&detail.Ticker, &detail.ISIN, &detail.MarketCap, &detail.MarketCapEur, &detail.Country, &detail.Sector, &detail.Eps, &detail.PbRatio, &detail.PeRatio, &detail.DividendYield, &detail.Revenue, &detail.NetIncome, &detail.ProfitMargin, &detail.Hash, &detail.Date)
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
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

		var wg sync.WaitGroup

		for _, tickerSymbol := range tickers {
			wg.Add(1)

			go func(ticker string) {
				defer wg.Done()

				log.Printf("Fetching prices for %s...", ticker)
				err := fetchPrices(ticker)
				if err != nil {
					log.Printf("Error fetching prices for %s: %v", ticker, err)
				} else {
					log.Printf("Successfully fetched prices for %s", ticker)
				}
			}(tickerSymbol)

			time.Sleep(200 * time.Millisecond)
		}

		wg.Wait()
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
	params.Add("interval", "5m")

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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body for %s: %v", ticker, err)
		return err
	}

	if len(bodyBytes) == 0 {
		log.Printf("Empty response body for %s, skipping", ticker)
		return nil
	}

	if bodyBytes[0] == 'N' || bodyBytes[0] == 'n' {
		log.Printf("API returned non-JSON response for %s: %s", ticker, string(bodyBytes[:min(100, len(bodyBytes))]))
		return nil
	}

	var candles []struct {
		Timestamp int64   `json:"timestamp"`
		Open      float64 `json:"open"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Close     float64 `json:"close"`
		Volume    float64 `json:"volume"`
	}

	err = json.Unmarshal(bodyBytes, &candles)
	if err != nil {
		log.Printf("Error decoding price data for %s: %v, body: %s", ticker, err, string(bodyBytes[:min(200, len(bodyBytes))]))
		return nil
	}

	log.Printf("Fetched %d price candles for %s", len(candles), ticker)

	var validPrices []Price
	for _, candle := range candles {
		if candle.Timestamp > now {
			log.Printf("Warning: Skipping future timestamp %d for %s", candle.Timestamp, ticker)
			continue
		}

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
		validPrices = append(validPrices, price)
	}

	if len(validPrices) > 0 {
		err = db.addPrices(validPrices)
		if err != nil {
			log.Printf("Error batch inserting prices for %s: %v", ticker, err)
			return err
		}
	}

	log.Printf("Successfully processed %d valid price candles for %s", len(validPrices), ticker)
	return nil
}

func fetchNewsPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting periodic news fetch...")

		etfTickers, err := db.getETFTickers()
		if err != nil {
			log.Printf("Error getting ETF tickers: %v", err)
		} else {
			log.Printf("Found %d ETF tickers to fetch composite news for", len(etfTickers))
			for _, etfTicker := range etfTickers {
				topTickers, err := db.getTopHoldingTickersForETF(etfTicker, 10)
				if err != nil {
					log.Printf("Error getting top holdings for ETF %s: %v", etfTicker, err)
					continue
				}
				if len(topTickers) == 0 {
					log.Printf("ETF %s has no underlying holdings in DB, falling back to direct fetch", etfTicker)
					fetchNews(etfTicker, 10)
					continue
				}
				log.Printf("ETF %s: fetching news for %d underlying holdings: %v", etfTicker, len(topTickers), topTickers)
				for _, underlying := range topTickers {
					fetchNewsAs(underlying, etfTicker, 3)
					time.Sleep(200 * time.Millisecond)
				}
			}
		}

		tickers, err := db.getUniqueTickers()
		if err != nil {
			log.Printf("Error getting unique tickers: %v", err)
			continue
		}

		etfSet := make(map[string]bool)
		for _, t := range etfTickers {
			etfSet[t] = true
		}

		log.Printf("Found %d unique tickers to fetch news for", len(tickers))

		var wg sync.WaitGroup

		for _, tickerSymbol := range tickers {
			if etfSet[tickerSymbol] {
				continue
			}
			wg.Add(1)

			go func(ticker string) {
				defer wg.Done()
				log.Printf("Fetching news for %s...", ticker)
				err := fetchNews(ticker, 10)
				if err != nil {
					log.Printf("Error fetching news for %s: %v", ticker, err)
				} else {
					log.Printf("Successfully fetched news for %s", ticker)
				}
			}(tickerSymbol)

			time.Sleep(250 * time.Millisecond)
		}

		wg.Wait()
		log.Println("Completed periodic news fetch cycle")
	}
}

func fetchNewsAs(actualTicker string, storeAsTicker string, numArticles int) error {
	baseURL := BASE_URL + "/fetch_news"
	params := url.Values{}
	params.Add("ticker", actualTicker)
	params.Add("num_articles", strconv.Itoa(numArticles))

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	resp, err := http.Get(fullURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch news: %s", resp.Status)
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

	if err := json.NewDecoder(resp.Body).Decode(&newsArticles); err != nil {
		return err
	}

	holdings, _ := db.getHoldingsByTicker(storeAsTicker)
	assets, _ := db.getAssetsByTicker(storeAsTicker)

	for _, article := range newsArticles {
		news := News{
			IdNews:      generateID(),
			Title:       fmt.Sprintf("[via %s] %s", actualTicker, article.Title),
			Link:        article.URL,
			PublishedAt: strconv.FormatInt(article.PublishedAt, 10),
			Summary:     article.Summary,
			Text:        article.Text,
			Author:      article.Author,
			Ticker:      storeAsTicker,
			Sentiment:   article.Sentiment,
		}
		if len(assets) > 0 {
			news.idAsset = assets[0].IdAsset
		}
		if len(holdings) > 0 {
			news.idHolding = holdings[0].IdHolding
		}
		if err := db.addNews(news); err != nil {
			log.Printf("Error adding composite news '%s': %v", article.Title, err)
		}
	}

	log.Printf("Fetched %d composite articles for ETF %s (via %s)", len(newsArticles), storeAsTicker, actualTicker)
	return nil
}

func (database *DB) newsExists(title string, summary string, text string) (bool, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	var count int
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
			Author:      article.Author,
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
			Name       string  `json:"name"`
			Ticker     string  `json:"ticker"`
			ISIN       string  `json:"isin"`
			Exchange   string  `json:"exchange"`
			Sector     string  `json:"sector"`
			Region     string  `json:"region"`
			Percentage float64 `json:"percentage"`
		} `json:"holdings"`
		Sectors map[string]float64 `json:"sectors"`
		Regions map[string]float64 `json:"regions"`
		ISIN    string             `json:"isin"`
		Ticker  string             `json:"ticker"`
	}

	err = json.NewDecoder(resp.Body).Decode(&etfData)
	if err != nil {
		log.Printf("Error decoding ETF data for %s: %v", ticker, err)
		return err
	}

	log.Printf("Fetched ETF data for holding ID %s: %d holdings, %d sectors, %d regions",
		holdingID, len(etfData.Holdings), len(etfData.Sectors), len(etfData.Regions))

	if etfData.ISIN != "" && etfData.ISIN != "N/A" {
		log.Printf("Updating holding ISIN to %s from JustETF", etfData.ISIN)
		err = db.updateHoldingISINByID(holdingID, etfData.ISIN)
		if err != nil {
			log.Printf("Warning: Could not update holding ISIN: %v", err)
		}
	}

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
	db, err := sql.Open("sqlite3", "./portfolio.db?_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL&cache=shared&_cache_size=-64000&_mmap_size=268435456&_temp_store=MEMORY")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA busy_timeout=30000;")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA synchronous=NORMAL;")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA cache_size=-64000;")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA temp_store=MEMORY;")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA mmap_size=268435456;")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA wal_autocheckpoint=1000;")
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
			author TEXT,
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
	if err != nil {
		return nil, err
	}

	// Create Daily Sentiment/Summary table for whole user portfolio
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

	// Crete Asset Details table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS asset_details (
			id_asset TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			isin TEXT,
			market_cap TEXT,
			market_cap_eur TEXT,
			country TEXT,
			sector TEXT,
			eps TEXT,
			pb_ratio TEXT,
			pe_ratio TEXT,
			dividend_yield TEXT,
			revenue TEXT,
			net_income TEXT,
			profit_margin TEXT,
			hash TEXT,
			date TEXT
		)
	`)

	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_holdings_user_id ON holdings(user_id)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_assets_id_holding ON assets(id_holding)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_assets_ticker ON assets(ticker)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_news_id_asset ON news(id_asset)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_news_id_holding ON news(id_holding)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_news_ticker ON news(ticker)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_regions_id_holding ON regions(id_holding)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_sectors_id_holding ON sectors(id_holding)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_prices_ticker ON prices(ticker)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_prices_date ON prices(date)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_prices_ticker_date ON prices(ticker, date)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_daily_sentiment_ticker ON daily_sentiment(ticker)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_daily_sentiment_date ON daily_sentiment(date)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_daily_sentiment_ticker_date ON daily_sentiment(ticker, date)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_portfolio_daily_sentiment_user_id ON portfolio_daily_sentiment(user_id)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_portfolio_daily_sentiment_date ON portfolio_daily_sentiment(date)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_asset_details_ticker ON asset_details(ticker)`)
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
	dbMutex.Lock()
	defer dbMutex.Unlock()

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

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO users (id, user_name, email, password)
		VALUES (?, ?, ?, ?)
	`, user.Id, user.userName, user.Email, user.Password)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) getUserByEmail(email string) (*User, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingHolding Holding
	err = tx.QueryRow(`
		SELECT id_holding, quantity, purchase_price 
		FROM holdings 
		WHERE user_id = ? AND ticker = ? AND exchange = ?
	`, holding.userID, holding.Ticker, holding.Exchange).Scan(
		&existingHolding.IdHolding,
		&existingHolding.Quantity,
		&existingHolding.PurchasePrice,
	)

	if err == sql.ErrNoRows {
		_, err = tx.Exec(`
			INSERT INTO holdings (id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, holding.IdHolding, holding.Name, holding.Ticker, holding.ISIN, holding.Exchange, holding.Etf, holding.Quantity, holding.PurchasePrice, holding.TER, holding.Policy, holding.userID, holding.currency)
		if err != nil {
			return err
		}
		err = tx.Commit()
		if err != nil {
			return err
		}
		return nil
	} else if err != nil {
		return err
	}

	totalCost := (existingHolding.Quantity * existingHolding.PurchasePrice) + (holding.Quantity * holding.PurchasePrice)
	newQuantity := existingHolding.Quantity + holding.Quantity
	newAvgPrice := totalCost / newQuantity

	_, err = tx.Exec(`
		UPDATE holdings 
		SET quantity = ?, purchase_price = ?, name = ?, isin = ?, ter = ?, policy = ?, currency = ?
		WHERE id_holding = ?
	`, newQuantity, newAvgPrice, holding.Name, holding.ISIN, holding.TER, holding.Policy, holding.currency, existingHolding.IdHolding)

	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	updatedHolding := holding
	updatedHolding.IdHolding = existingHolding.IdHolding
	updatedHolding.Quantity = newQuantity
	updatedHolding.PurchasePrice = newAvgPrice
	return nil
}

func (database *DB) removeHolding(holdingID string, userID string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
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
		return tx.Rollback()
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) modifyHolding(holding Holding) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
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

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) getHoldingsByUser(userID string) ([]Holding, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	var summary PortfolioDailySentiment
	err := database.QueryRow(`
		SELECT id_sentiment, user_id, date, summary, sentiment
		FROM portfolio_daily_sentiment
		WHERE user_id = ? AND date = ?
	`, userID, date).Scan(&summary.IdSentiment, &summary.UserID, &summary.Date, &summary.Summary, &summary.Sentiment)
	if err != nil {
		err = database.QueryRow(`
			SELECT id_sentiment, user_id, date, summary, sentiment
			FROM portfolio_daily_sentiment
			WHERE user_id = ? AND date >= date('now', '-7 days')
			ORDER BY date DESC
			LIMIT 1
		`, userID).Scan(&summary.IdSentiment, &summary.UserID, &summary.Date, &summary.Summary, &summary.Sentiment)
		if err != nil {
			return nil, err
		}
	}
	return &summary, nil
}

func (database *DB) getHoldingDailySummary(holdingID string, date string) (*DailySentiment, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	var summary DailySentiment
	err := database.QueryRow(`
		SELECT id_sentiment, ticker, date, summary, sentiment
		FROM daily_sentiment
		WHERE ticker = ? AND date = ?
	`, holdingID, date).Scan(&summary.IdSentiment, &summary.Ticker, &summary.Date, &summary.Summary, &summary.Sentiment)
	if err != nil {
		err = database.QueryRow(`
			SELECT id_sentiment, ticker, date, summary, sentiment
			FROM daily_sentiment
			WHERE ticker = ? AND date >= date('now', '-7 days')
			ORDER BY date DESC
			LIMIT 1
		`, holdingID).Scan(&summary.IdSentiment, &summary.Ticker, &summary.Date, &summary.Summary, &summary.Sentiment)
		if err != nil {
			return nil, err
		}
	}
	return &summary, nil
}

func (database *DB) addAsset(asset Asset) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO assets (id_asset, name, ticker, isin, exchange, sector, region, id_holding, currency)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, asset.IdAsset, asset.Name, asset.Ticker, asset.ISIN, asset.Exchange, asset.Sector, asset.Region, asset.idHolding, asset.currency)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) updateAssetISIN(ticker string, isin string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE assets SET isin = ? WHERE ticker = ?`, isin, ticker)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) updateAssetTicker(isin string, ticker string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE assets SET ticker = ? WHERE isin = ?`, ticker, isin)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) updateHoldingISIN(ticker string, isin string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE holdings SET isin = ? WHERE ticker = ?`, isin, ticker)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) updateHoldingTicker(isin string, ticker string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE holdings SET ticker = ? WHERE isin = ?`, ticker, isin)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) updateHoldingISINByID(holdingID string, isin string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE holdings SET isin = ? WHERE id_holding = ?`, isin, holdingID)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) updateHoldingTickerByID(holdingID string, ticker string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE holdings SET ticker = ? WHERE id_holding = ?`, ticker, holdingID)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) addSector(sector Sector) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO sectors (name, id_holding, percentage)
		VALUES (?, ?, ?)
	`, sector.Name, sector.IdHolding, sector.Percentage)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) addRegion(region Region) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO regions (name, id_holding, percentage)
		VALUES (?, ?, ?)
	`, region.Name, region.IdHolding, region.Percentage)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) addNews(news News) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT OR IGNORE INTO news (id_news, title, link, published_at, summary, text, author, sentiment, ticker, id_asset, id_holding)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, news.IdNews, news.Title, news.Link, news.PublishedAt, news.Summary, news.Text, news.Author, news.Sentiment, news.Ticker, news.idAsset, news.idHolding)

	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) deleteETFDataForHolding(holdingID string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM assets WHERE id_holding = ?`, holdingID)
	if err != nil {
		return fmt.Errorf("error deleting assets: %v", err)
	}

	_, err = tx.Exec(`DELETE FROM sectors WHERE id_holding = ?`, holdingID)
	if err != nil {
		return fmt.Errorf("error deleting sectors: %v", err)
	}

	_, err = tx.Exec(`DELETE FROM regions WHERE id_holding = ?`, holdingID)
	if err != nil {
		return fmt.Errorf("error deleting regions: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) getAllETFHoldings() ([]Holding, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	rows, err := database.Query(`
		SELECT id_holding, name, ticker, isin, exchange, etf, quantity, purchase_price, ter, policy, user_id, currency
		FROM holdings
		WHERE etf = 1
	`)

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

func (database *DB) upsertDailySentiment(sentiment DailySentiment) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO daily_sentiment (id_sentiment, ticker, date, summary, sentiment)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(ticker, date) DO UPDATE SET
			summary = excluded.summary,
			sentiment = excluded.sentiment
	`, sentiment.IdSentiment, sentiment.Ticker, sentiment.Date, sentiment.Summary, sentiment.Sentiment)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) upsertPortfolioDailySentiment(sentiment PortfolioDailySentiment) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO portfolio_daily_sentiment (id_sentiment, user_id, date, summary, sentiment)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			summary = excluded.summary,
			sentiment = excluded.sentiment
	`, sentiment.IdSentiment, sentiment.UserID, sentiment.Date, sentiment.Summary, sentiment.Sentiment)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

func (database *DB) getNewsForTickerToday(ticker string, todayDate string) ([]News, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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

func (database *DB) getRecentNewsForTicker(ticker string, limit int) ([]News, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	rows, err := database.Query(`
		SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
		FROM news
		WHERE ticker = ?
		ORDER BY published_at DESC
		LIMIT ?
	`, ticker, limit)
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
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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

func (database *DB) getETFTickers() ([]string, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	rows, err := database.Query(`SELECT DISTINCT ticker FROM holdings WHERE etf = 1 AND ticker != ''`)
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
	return tickers, nil
}

func (database *DB) getTopHoldingTickersForETF(etfTicker string, limit int) ([]string, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	rows, err := database.Query(`
		SELECT a.ticker FROM assets a
		JOIN holdings h ON a.id_holding = h.id_holding
		WHERE h.ticker = ? AND a.ticker != ''
		LIMIT ?
	`, etfTicker, limit)
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
	return tickers, nil
}

func (database *DB) getUniqueISINs() ([]string, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

	rows, err := database.Query(`
		SELECT DISTINCT isin FROM (
			SELECT isin FROM holdings WHERE isin != ''
			UNION
			SELECT isin FROM assets WHERE isin != ''
		) AS combined_isins
		ORDER BY isin
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var isins []string
	for rows.Next() {
		var isin string
		err := rows.Scan(&isin)
		if err != nil {
			return nil, err
		}
		isins = append(isins, isin)
	}

	return isins, nil
}

func (database *DB) getHoldingsByTicker(ticker string) ([]Holding, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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

func (database *DB) addPrices(prices []Price) error {
	if len(prices) == 0 {
		return nil
	}

	dbMutex.Lock()
	defer dbMutex.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO prices (id_price, ticker, date, open, close, high, low, volume) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, price := range prices {
		_, err = stmt.Exec(price.IdPrice, price.Ticker, price.Date, price.Open, price.Close, price.High, price.Low, price.Volume)
		if err != nil {
			log.Printf("Error adding price for %s on %s: %v", price.Ticker, price.Date, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
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
	dbMutex.RLock()
	defer dbMutex.RUnlock()

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

func resolveTickerOrISIN(identifier string) (string, error) {
	var ticker string
	dbMutex.Lock()
	err := db.QueryRow(`
		SELECT ticker FROM holdings WHERE ticker = ? OR isin = ? LIMIT 1
	`, identifier, identifier).Scan(&ticker)
	dbMutex.Unlock()

	if err == nil {
		return ticker, nil
	}

	dbMutex.Lock()
	err = db.QueryRow(`
		SELECT ticker FROM assets WHERE ticker = ? OR isin = ? LIMIT 1
	`, identifier, identifier).Scan(&ticker)
	dbMutex.Unlock()

	if err == nil {
		return ticker, nil
	}

	return identifier, nil
}

func buildPlaceholders(count int) string {
	if count == 0 {
		return ""
	}
	placeholders := make([]byte, 0, count*2-1)
	for i := 0; i < count; i++ {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
	}
	return string(placeholders)
}

func toInterfaceSlice(slice []string) []interface{} {
	result := make([]interface{}, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

func generateJWT(userID, email string) (string, error) {
	claims := &JWTClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(JWT_EXPIRY)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWT_SECRET))
}

func isEmailValid(email string) bool {
	if len(email) < 5 || len(email) > 254 {
		return false
	}
	atIdx := strings.IndexByte(email, '@')
	if atIdx <= 0 || atIdx == len(email)-1 {
		return false
	}
	dotIdx := strings.LastIndexByte(email, '.')
	return dotIdx > atIdx+1 && dotIdx < len(email)-1
}

func checkLoginRateLimit(ip string) bool {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-loginBlockWindow)

	var recent []time.Time
	for _, t := range loginAttempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	loginAttempts[ip] = recent

	if len(recent) >= maxLoginAttempts {
		return false
	}
	loginAttempts[ip] = append(loginAttempts[ip], now)
	return true
}

func clearExpiredLoginAttempts() {
	for {
		time.Sleep(10 * time.Minute)
		loginAttemptsMu.Lock()
		cutoff := time.Now().Add(-loginBlockWindow)
		for ip, attempts := range loginAttempts {
			var recent []time.Time
			for _, t := range attempts {
				if t.After(cutoff) {
					recent = append(recent, t)
				}
			}
			if len(recent) == 0 {
				delete(loginAttempts, ip)
			} else {
				loginAttempts[ip] = recent
			}
		}
		loginAttemptsMu.Unlock()
	}
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

	if !isEmailValid(req.Email) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid email format"})
	}

	if len(req.Password) < 8 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Password must be at least 8 characters"})
	}

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
	ip := c.RealIP()
	if !checkLoginRateLimit(ip) {
		return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Too many login attempts. Try again later."})
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if req.Email == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Email and password are required"})
	}

	valid, err := db.verifyUser(req.Email, req.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error verifying user"})
	}
	if !valid {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
	}

	user, err := db.getUserByEmail(req.Email)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error retrieving user"})
	}

	// Set secure cookie flags via response headers
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("X-Frame-Options", "DENY")

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

func healthCheck(c echo.Context) error {
	if err := db.Ping(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

func fillInBetweenPricesPeriodic(interval time.Duration) {
	// TODO: make the filling better to avoid the big triangles in between missing data
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
	dbMutex.Lock()
	rows, err := db.Query(`
		SELECT date, open, close, high, low, volume
		FROM prices
		WHERE ticker = ?
		ORDER BY CAST(date AS INTEGER) ASC
	`, Ticker)
	dbMutex.Unlock()
	if err != nil {
		return err
	}
	defer rows.Close()

	var existingPrices []Price
	for rows.Next() {
		var p Price
		err := rows.Scan(&p.Date, &p.Open, &p.Close, &p.High, &p.Low, &p.Volume)
		if err != nil {
			return err
		}
		existingPrices = append(existingPrices, p)
	}

	var missingPrices []Price
	for index, price := range existingPrices {
		if index == 0 {
			continue
		}
		prevPrice := existingPrices[index-1]
		currentDateInt, _ := strconv.ParseInt(price.Date, 10, 64)
		prevDateInt, _ := strconv.ParseInt(prevPrice.Date, 10, 64)

		if currentDateInt-prevDateInt > candleInterval*2 {
			fillsNeeded := (currentDateInt-prevDateInt)/candleInterval - 1
			for i := int64(1); i <= fillsNeeded; i++ {
				missingDate := prevDateInt + i*candleInterval
				interpolatedPrice := prevPrice.Close + (price.Open-prevPrice.Close)*float64(i)/float64(fillsNeeded+1)
				missingPrice := Price{
					IdPrice: generateID(),
					Ticker:  Ticker,
					Date:    strconv.FormatInt(missingDate, 10),
					Open:    interpolatedPrice,
					Close:   interpolatedPrice,
					High:    interpolatedPrice,
					Low:     interpolatedPrice,
					Volume:  0,
				}
				missingPrices = append(missingPrices, missingPrice)
			}
		}
	}

	if len(missingPrices) > 0 {
		dbMutex.Lock()
		defer dbMutex.Unlock()

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		stmt, err := tx.Prepare(`INSERT OR IGNORE INTO prices (id_price, ticker, date, open, close, high, low, volume) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, price := range missingPrices {
			_, err = stmt.Exec(price.IdPrice, price.Ticker, price.Date, price.Open, price.Close, price.High, price.Low, price.Volume)
			if err != nil {
				log.Printf("Error adding interpolated price for %s on %s: %v", Ticker, price.Date, err)
			}
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// updateSentimentsPeriodic runs once per day to generate daily summaries for tickers and portfolios
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
					if !strings.Contains(err.Error(), "no news available") {
						log.Printf("Error updating sentiment for %s: %v", tickerSym, err)
					}
				} else {
					log.Printf("Successfully updated sentiment for %s", tickerSym)
				}
			}(tickerSymbol)
			time.Sleep(500 * time.Millisecond)
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
			time.Sleep(1 * time.Second)
		}

		log.Println("Completed periodic sentiment/summary update cycle")
	}
}

func updateETFDataPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting periodic ETF data update...")

		etfHoldings, err := db.getAllETFHoldings()
		if err != nil {
			log.Printf("Error getting ETF holdings: %v", err)
			continue
		}

		log.Printf("Updating ETF data for %d ETF holdings", len(etfHoldings))

		for _, holding := range etfHoldings {
			go func(h Holding) {
				log.Printf("Updating ETF data for %s (ID: %s)", h.Ticker, h.IdHolding)

				err := db.deleteETFDataForHolding(h.IdHolding)
				if err != nil {
					log.Printf("Error deleting old ETF data for %s: %v", h.Ticker, err)
					return
				}

				err = fetchAndStoreETFData(h.IdHolding, h.Ticker, h.ISIN, h.Name)
				if err != nil {
					log.Printf("Error fetching new ETF data for %s: %v", h.Ticker, err)
					return
				}

				log.Printf("Successfully updated ETF data for %s", h.Ticker)
			}(holding)
			time.Sleep(10 * time.Second)
		}

		log.Println("Completed periodic ETF data update cycle")
	}
}

func updateTickerDailySentiment(tickerSymbol string, todayDate string) error {
	newsList, err := db.getRecentNewsForTicker(tickerSymbol, 30)
	if err != nil {
		return fmt.Errorf("error fetching news: %v", err)
	}

	if len(newsList) == 0 {
		return fmt.Errorf("no news available for %s", tickerSymbol)
	}

	// Prepare data for API call
	var summaries []string
	var sentiments []float64
	var fullTexts []string
	for _, news := range newsList {
		summaries = append(summaries, news.Summary)
		sentiments = append(sentiments, news.Sentiment)
		fullTexts = append(fullTexts, news.Text)
	}

	// Call Python API to generate summary
	requestBody := map[string]interface{}{
		"ticker":         tickerSymbol,
		"date":           todayDate,
		"news_list":      summaries,
		"sentiment_list": sentiments,
		"full_text_list": fullTexts,
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
	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return fmt.Errorf("error fetching holdings: %v", err)
	}

	if len(holdings) == 0 {
		log.Printf("No holdings found for user %s, skipping", userID)
		return nil
	}

	type HoldingSummary struct {
		Ticker    string  `json:"ticker"`
		Summary   string  `json:"summary"`
		Sentiment float64 `json:"sentiment"`
	}

	var holdingSummaries []HoldingSummary

	for _, holding := range holdings {
		ds, dsErr := db.getHoldingDailySummary(holding.Ticker, todayDate)
		if dsErr != nil || ds == nil {
			continue
		}
		if ds.Summary == "" {
			continue
		}
		holdingSummaries = append(holdingSummaries, HoldingSummary{
			Ticker:    ds.Ticker,
			Summary:   ds.Summary,
			Sentiment: ds.Sentiment,
		})
	}

	if len(holdingSummaries) == 0 {
		log.Printf("No holding summaries found for user %s on %s, skipping", userID, todayDate)
		return nil
	}

	requestBody := map[string]interface{}{
		"user_id":           userID,
		"date":              todayDate,
		"holding_summaries": holdingSummaries,
		"max_tokens":        4096,
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

	sentiment := PortfolioDailySentiment{
		IdSentiment: generateID(),
		UserID:      result.UserID,
		Date:        result.Date,
		Summary:     result.Summary,
		Sentiment:   result.Sentiment,
	}

	return db.upsertPortfolioDailySentiment(sentiment)
}

func triggerNewsSummary(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	lastSummaryGenMu.Lock()
	if devMode != "true" {
		if last, ok := lastSummaryGen[userID]; ok && time.Since(last) < summaryCooldown {
			remaining := summaryCooldown - time.Since(last)
			lastSummaryGenMu.Unlock()
			return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
				"error":           "Please wait before generating another summary",
				"retry_after_min": int(remaining.Minutes()) + 1,
			})
		}
	}
	lastSummaryGenMu.Unlock()

	todayDate := time.Now().UTC().Format("2006-01-02")

	log.Printf("Manual news summary triggered by user %s - regenerating from existing news", userID)

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		log.Printf("Error getting holdings for summary: %v", err)
	} else {
		tickerSet := make(map[string]bool)
		var tickerList []string
		for _, h := range holdings {
			if !tickerSet[h.Ticker] {
				tickerSet[h.Ticker] = true
				tickerList = append(tickerList, h.Ticker)
			}
		}
		log.Printf("Regenerating summaries for %d holding tickers", len(tickerList))

		var wg sync.WaitGroup
		for _, t := range tickerList {
			wg.Add(1)
			go func(tickerSym string) {
				defer wg.Done()
				if err := updateTickerDailySentiment(tickerSym, todayDate); err != nil {
					if !strings.Contains(err.Error(), "no news available") {
						log.Printf("Error updating ticker sentiment for %s: %v", tickerSym, err)
					}
				}
			}(t)
		}
		wg.Wait()
	}

	if err := updatePortfolioDailySentiment(userID, todayDate); err != nil {
		log.Printf("Error updating portfolio sentiment for user %s: %v", userID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate summary"})
	}

	lastSummaryGenMu.Lock()
	lastSummaryGen[userID] = time.Now()
	lastSummaryGenMu.Unlock()

	sentiment, err := db.getPortfolioDailySummary(userID, todayDate)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Summary generated but failed to retrieve"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":   "News summary generated successfully",
		"sentiment": sentiment.Sentiment,
		"summary":   sentiment.Summary,
		"date":      sentiment.Date,
	})
}

func triggerHoldingSummary(c echo.Context) error {
	ticker := c.FormValue("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "ticker is required")
	}

	resolvedTicker, err := resolveTickerOrISIN(ticker)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", ticker, err)
		resolvedTicker = ticker
	}

	cooldownKey := resolvedTicker
	lastTickerSummaryGenMu.Lock()
	if devMode != "true" {
		if last, ok := lastTickerSummaryGen[cooldownKey]; ok && time.Since(last) < tickerSummaryCooldown {
			remaining := tickerSummaryCooldown - time.Since(last)
			lastTickerSummaryGenMu.Unlock()
			return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
				"error":           "Please wait before generating another summary for this holding",
				"retry_after_min": int(remaining.Minutes()) + 1,
			})
		}
	}
	lastTickerSummaryGenMu.Unlock()

	todayDate := time.Now().UTC().Format("2006-01-02")

	log.Printf("Manual holding summary triggered for ticker %s", resolvedTicker)

	if err := updateTickerDailySentiment(resolvedTicker, todayDate); err != nil {
		log.Printf("Error updating ticker sentiment for %s: %v", resolvedTicker, err)
		if strings.Contains(err.Error(), "no news available") {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"message": "No news articles available yet for this holding. News is fetched periodically.",
				"ticker":  resolvedTicker,
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate holding summary"})
	}

	lastTickerSummaryGenMu.Lock()
	lastTickerSummaryGen[cooldownKey] = time.Now()
	lastTickerSummaryGenMu.Unlock()

	sentiment, err := db.getHoldingDailySummary(resolvedTicker, todayDate)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Summary generated but failed to retrieve"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":   "Holding summary generated successfully",
		"ticker":    sentiment.Ticker,
		"sentiment": sentiment.Sentiment,
		"summary":   sentiment.Summary,
		"date":      sentiment.Date,
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

	err := db.addHolding(holding)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error adding holding to database")
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

	var oldHolding Holding
	dbMutex.Lock()
	err := db.QueryRow(`SELECT etf FROM holdings WHERE id_holding = ? AND user_id = ?`, holdingID, userID).Scan(&oldHolding.Etf)
	dbMutex.Unlock()

	wasETF := (err == nil && oldHolding.Etf)
	isNowETF := holding.Etf

	err = db.modifyHolding(holding)
	if err == sql.ErrNoRows {
		return c.String(http.StatusNotFound, "Holding not found")
	}
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error modifying holding in database")
	}

	if wasETF && !isNowETF {
		go func(hID string) {
			err := db.deleteETFDataForHolding(hID)
			if err != nil {
				log.Printf("Error deleting ETF data for modified holding %s: %v", hID, err)
			}
		}(holdingID)
	} else if !wasETF && isNowETF {
		go func(hID, ticker, isin, name string) {
			log.Printf("Fetching ETF data for newly converted ETF holding %s...", ticker)
			err := fetchAndStoreETFData(hID, ticker, isin, name)
			if err != nil {
				log.Printf("Error fetching ETF data for modified holding %s: %v", ticker, err)
			}
		}(holdingID, Ticker, ISIN, Name)
	} else if wasETF && isNowETF {
		go func(hID, ticker, isin, name string) {
			log.Printf("Updating ETF data for modified ETF holding %s...", ticker)
			err := db.deleteETFDataForHolding(hID)
			if err != nil {
				log.Printf("Error deleting old ETF data for %s: %v", ticker, err)
				return
			}
			err = fetchAndStoreETFData(hID, ticker, isin, name)
			if err != nil {
				log.Printf("Error fetching new ETF data for %s: %v", ticker, err)
			}
		}(holdingID, Ticker, ISIN, Name)
	}

	return c.String(http.StatusOK, "Holding modified successfully")
}

func GetHoldings(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

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

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, result)
	}

	holdingIDs := make([]string, len(holdings))
	for i, h := range holdings {
		holdingIDs[i] = h.IdHolding
	}

	sectorsMap := make(map[string][]SectorData)
	regionsMap := make(map[string][]RegionData)
	assetsMap := make(map[string][]AssetData)

	dbMutex.Lock()
	sectorRows, err := db.Query(`SELECT id_holding, name, percentage FROM sectors WHERE id_holding IN (`+buildPlaceholders(len(holdingIDs))+`)`, toInterfaceSlice(holdingIDs)...)
	if err == nil {
		defer sectorRows.Close()
		for sectorRows.Next() {
			var idHolding, name string
			var percentage float64
			if err := sectorRows.Scan(&idHolding, &name, &percentage); err == nil {
				sectorsMap[idHolding] = append(sectorsMap[idHolding], SectorData{Name: name, Percentage: percentage})
			}
		}
	}
	dbMutex.Unlock()

	dbMutex.Lock()
	regionRows, err := db.Query(`SELECT id_holding, name, percentage FROM regions WHERE id_holding IN (`+buildPlaceholders(len(holdingIDs))+`)`, toInterfaceSlice(holdingIDs)...)
	if err == nil {
		defer regionRows.Close()
		for regionRows.Next() {
			var idHolding, name string
			var percentage float64
			if err := regionRows.Scan(&idHolding, &name, &percentage); err == nil {
				regionsMap[idHolding] = append(regionsMap[idHolding], RegionData{Name: name, Percentage: percentage})
			}
		}
	}
	dbMutex.Unlock()

	dbMutex.Lock()
	assetRows, err := db.Query(`
		SELECT id_holding, id_asset, name, ticker, isin, exchange, sector, region 
		FROM assets 
		WHERE id_holding IN (`+buildPlaceholders(len(holdingIDs))+`)
	`, toInterfaceSlice(holdingIDs)...)
	if err == nil {
		defer assetRows.Close()
		for assetRows.Next() {
			var idHolding string
			var a AssetData
			if err := assetRows.Scan(&idHolding, &a.IdAsset, &a.Name, &a.Ticker, &a.ISIN, &a.Exchange, &a.Sector, &a.Region); err == nil {
				assetsMap[idHolding] = append(assetsMap[idHolding], a)
			}
		}
	}
	dbMutex.Unlock()

	for _, h := range holdings {
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
			Sectors:       sectorsMap[h.IdHolding],
			Regions:       regionsMap[h.IdHolding],
			Assets:        assetsMap[h.IdHolding],
		})
	}

	return c.JSON(http.StatusOK, result)
}

func GetPortfolioValue(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	if len(holdings) == 0 {
		return c.JSON(http.StatusOK, map[string]float64{"total_value": 0.0})
	}

	tickers := make([]string, len(holdings))
	tickerQuantities := make(map[string]float64)
	tickerPurchasePrice := make(map[string]float64)

	for i, holding := range holdings {
		tickers[i] = holding.Ticker
		tickerQuantities[holding.Ticker] += holding.Quantity
		tickerPurchasePrice[holding.Ticker] = holding.PurchasePrice
	}

	query := `
		SELECT ticker, close 
		FROM prices 
		WHERE ticker IN (` + buildPlaceholders(len(tickers)) + `) 
		AND CAST(date AS INTEGER) = (
			SELECT MAX(CAST(date AS INTEGER)) 
			FROM prices p2 
			WHERE p2.ticker = prices.ticker
		)
	`

	dbMutex.Lock()
	rows, err := db.Query(query, toInterfaceSlice(tickers)...)
	dbMutex.Unlock()
	if err != nil {
		log.Printf("Error fetching latest prices: %v", err)
		return c.String(http.StatusInternalServerError, "Error fetching prices")
	}
	defer rows.Close()

	latestPrices := make(map[string]float64)
	for rows.Next() {
		var ticker string
		var price float64
		if err := rows.Scan(&ticker, &price); err == nil {
			latestPrices[ticker] = price
		}
	}

	totalValue := 0.0
	for ticker, quantity := range tickerQuantities {
		if price, exists := latestPrices[ticker]; exists {
			totalValue += price * quantity
		} else {
			totalValue += tickerPurchasePrice[ticker] * quantity
		}
	}

	return c.JSON(http.StatusOK, map[string]float64{
		"total_value": totalValue,
	})
}

func GetPortfolioValueHistory(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	interval := c.QueryParam("interval")
	userID := claims.UserID

	now := time.Now().UTC()

	var intervalSeconds int64
	var startTime time.Time

	switch interval {
	case "5m":
		intervalSeconds = 300
		startTime = now.Add(-24 * time.Hour)
	case "15m":
		intervalSeconds = 900
		startTime = now.Add(-7 * 24 * time.Hour)
	case "1h":
		intervalSeconds = 3600
		startTime = now.Add(-30 * 24 * time.Hour)
	case "4h":
		intervalSeconds = 14400
		startTime = now.Add(-90 * 24 * time.Hour)
	case "1d":
		intervalSeconds = 86400
		startTime = now.Add(-365 * 24 * time.Hour)
	case "1w":
		intervalSeconds = 604800
		startTime = now.Add(-730 * 24 * time.Hour)
	case "1M":
		intervalSeconds = 2592000
		startTime = time.Time{}
	default:
		intervalSeconds = 3600
		startTime = now.Add(-30 * 24 * time.Hour)
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

	dbMutex.Lock()
	rows, err := db.Query(query, args...)
	dbMutex.Unlock()
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

func GetPortfolioSentimentHistory(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID
	interval := c.QueryParam("interval")

	now := time.Now().UTC()
	var startTime time.Time

	switch interval {
	case "5m":
		startTime = now.Add(-24 * time.Hour)
	case "15m":
		startTime = now.Add(-7 * 24 * time.Hour)
	case "1h":
		startTime = now.Add(-30 * 24 * time.Hour)
	case "4h":
		startTime = now.Add(-90 * 24 * time.Hour)
	case "1d":
		startTime = now.Add(-365 * 24 * time.Hour)
	case "1w":
		startTime = now.Add(-730 * 24 * time.Hour)
	case "1M":
		startTime = time.Time{}
	default:
		startTime = now.Add(-365 * 24 * time.Hour)
	}

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil || len(holdings) == 0 {
		return c.JSON(http.StatusOK, []map[string]interface{}{})
	}

	totalValue := 0.0
	for _, h := range holdings {
		totalValue += h.Quantity * h.PurchasePrice
	}

	weightMap := make(map[string]float64)
	for _, h := range holdings {
		if totalValue > 0 {
			weightMap[h.Ticker] += (h.Quantity * h.PurchasePrice) / totalValue
		}
	}

	for _, h := range holdings {
		if !h.Etf {
			continue
		}
		components, cerr := db.getUnderlyingAssetTickers(h.Ticker, h.ISIN)
		if cerr != nil || len(components) == 0 {
			continue
		}
		etfWeight := weightMap[h.Ticker]
		weightMap[h.Ticker] = etfWeight / 2
		componentWeight := (etfWeight / 2) / float64(len(components))
		for _, ct := range components {
			weightMap[ct] += componentWeight
		}
	}

	tickers := make([]string, 0, len(weightMap))
	for ticker := range weightMap {
		tickers = append(tickers, ticker)
	}

	phParts := make([]string, len(tickers))
	for i := range tickers {
		phParts[i] = "?"
	}
	placeholders := strings.Join(phParts, ",")

	args := make([]interface{}, 0, len(tickers)+2)
	for _, t := range tickers {
		args = append(args, t)
	}

	endDateStr := now.Format("2006-01-02")
	var queryStr string
	if startTime.IsZero() {
		args = append(args, endDateStr)
		queryStr = fmt.Sprintf(
			`SELECT ticker, date, sentiment FROM daily_sentiment WHERE ticker IN (%s) AND date <= ? ORDER BY date ASC`,
			placeholders,
		)
	} else {
		startDateStr := startTime.Format("2006-01-02")
		args = append(args, startDateStr, endDateStr)
		queryStr = fmt.Sprintf(
			`SELECT ticker, date, sentiment FROM daily_sentiment WHERE ticker IN (%s) AND date >= ? AND date <= ? ORDER BY date ASC`,
			placeholders,
		)
	}

	dbMutex.RLock()
	rows, err := db.Query(queryStr, args...)
	dbMutex.RUnlock()
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error querying sentiment data")
	}
	defer rows.Close()

	type DateEntry struct {
		totalWeight    float64
		totalSentiment float64
	}

	dateMap := make(map[string]*DateEntry)
	orderedDates := make([]string, 0)

	for rows.Next() {
		var ticker, date string
		var sentiment float64
		if err := rows.Scan(&ticker, &date, &sentiment); err != nil {
			continue
		}
		w := weightMap[ticker]
		if _, ok := dateMap[date]; !ok {
			dateMap[date] = &DateEntry{}
			orderedDates = append(orderedDates, date)
		}
		dateMap[date].totalWeight += w
		dateMap[date].totalSentiment += sentiment * w
	}

	sort.Strings(orderedDates)

	type SentimentPoint struct {
		Timestamp int64   `json:"timestamp"`
		Sentiment float64 `json:"sentiment"`
	}

	result := make([]SentimentPoint, 0, len(orderedDates))
	for _, date := range orderedDates {
		entry := dateMap[date]
		if entry.totalWeight <= 0 {
			continue
		}
		t, parseErr := time.Parse("2006-01-02", date)
		if parseErr != nil {
			continue
		}
		result = append(result, SentimentPoint{
			Timestamp: t.Unix(),
			Sentiment: entry.totalSentiment / entry.totalWeight,
		})
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

		var latestPrice float64
		dbMutex.Lock()
		err := db.QueryRow(`
			SELECT close FROM prices 
			WHERE ticker = ? 
			ORDER BY CAST(date AS INTEGER) DESC 
			LIMIT 1
		`, holding.Ticker).Scan(&latestPrice)
		dbMutex.Unlock()

		if err != nil {
			latestPrice = holding.PurchasePrice
		}
		currentValue += latestPrice * holding.Quantity

		var previousPrice float64
		dbMutex.Lock()
		err = db.QueryRow(`
			SELECT close FROM prices 
			WHERE ticker = ? 
			AND CAST(date AS INTEGER) <= ?
			ORDER BY CAST(date AS INTEGER) DESC 
			LIMIT 1
		`, holding.Ticker, oneDayAgo).Scan(&previousPrice)
		dbMutex.Unlock()

		if err != nil {
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
	identifier := c.QueryParam("ticker")

	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker parameter is required")
	}

	assetTicker, err := resolveTickerOrISIN(identifier)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", identifier, err)
		assetTicker = identifier
	}

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		log.Printf("Error retrieving holdings for user %s: %v", userID, err)
		return c.String(http.StatusInternalServerError, "Error retrieving holdings")
	}

	var totalQuantity float64
	for _, holding := range holdings {
		if holding.Ticker == assetTicker || holding.ISIN == identifier || holding.Ticker == identifier {
			totalQuantity += holding.Quantity
		}
	}

	if totalQuantity == 0 {
		log.Printf("No holdings found for asset %s (resolved: %s) for user %s", identifier, assetTicker, userID)
		return c.String(http.StatusNotFound, "No holdings found for the specified asset")
	}

	var latestPrice float64
	dbMutex.Lock()
	err = db.QueryRow(`
		SELECT close FROM prices 
		WHERE ticker = ? 
		ORDER BY CAST(date AS INTEGER) DESC 
		LIMIT 1
	`, assetTicker).Scan(&latestPrice)
	dbMutex.Unlock()
	if err != nil {
		log.Printf("Error retrieving latest price for %s: %v", assetTicker, err)
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error retrieving latest price for %s", assetTicker))
	}

	oneDayAgo := time.Now().UTC().Add(-24 * time.Hour).Unix()
	var previousPrice float64
	dbMutex.Lock()
	err = db.QueryRow(`
		SELECT close FROM prices 
		WHERE ticker = ?
		AND CAST(date AS INTEGER) <= ?
		ORDER BY CAST(date AS INTEGER) DESC 
		LIMIT 1
	`, assetTicker, oneDayAgo).Scan(&previousPrice)
	dbMutex.Unlock()
	if err != nil {
		log.Printf("Error retrieving previous price for %s: %v", assetTicker, err)
		previousPrice = latestPrice
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

	// Build list of tickers from user's holdings, including underlying assets for ETFs
	tickerSet := make(map[string]bool)
	for _, h := range holdings {
		tickerSet[h.Ticker] = true

		if isETF, err := db.isETF(h.Ticker, h.ISIN); err == nil && isETF {
			underlyingTickers, err := db.getUnderlyingAssetTickers(h.Ticker, h.ISIN)
			if err == nil {
				for _, ut := range underlyingTickers {
					tickerSet[ut] = true
				}
			}
		}
	}

	tickers := make([]string, 0, len(tickerSet))
	for ticker := range tickerSet {
		tickers = append(tickers, ticker)
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

	dbMutex.Lock()
	rows, err := db.Query(query, args...)
	dbMutex.Unlock()
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
	identifier := c.QueryParam("ticker")
	ticker, err := resolveTickerOrISIN(identifier)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", identifier, err)
	}
	log.Printf("getLatestNewsForAsset: identifier=%s, resolved ticker=%s", identifier, ticker)

	var query string
	var args []interface{}

	isETF, etfErr := db.isETF(ticker, identifier)
	log.Printf("isETF check: ticker=%s, identifier=%s, isETF=%v, err=%v", ticker, identifier, isETF, etfErr)

	if etfErr == nil && isETF {
		log.Printf("Detected ETF for ticker=%s, identifier=%s", ticker, identifier)
		underlyingTickers, err := db.getUnderlyingAssetTickers(ticker, identifier)
		log.Printf("Underlying tickers: %v, error: %v", underlyingTickers, err)
		if err == nil && len(underlyingTickers) > 0 {
			allTickers := append([]string{ticker}, underlyingTickers...)
			placeholders := ""
			args = make([]interface{}, 0, len(allTickers)+2)
			for i, t := range allTickers {
				if i > 0 {
					placeholders += ","
				}
				placeholders += "?"
				args = append(args, t)
			}
			args = append(args, limit, offset)

			query = fmt.Sprintf(`
				SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
				FROM news
				WHERE ticker IN (%s)
				ORDER BY CAST(published_at AS INTEGER) DESC
				LIMIT ? OFFSET ?
			`, placeholders)
		} else {
			query = `
				SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
				FROM news
				WHERE ticker = ?
				ORDER BY CAST(published_at AS INTEGER) DESC
				LIMIT ? OFFSET ?
			`
			args = []interface{}{ticker, limit, offset}
		}
	} else {
		query = `
			SELECT id_news, title, link, published_at, summary, text, sentiment, ticker, id_asset, id_holding
			FROM news
			WHERE ticker = ?
			ORDER BY CAST(published_at AS INTEGER) DESC
			LIMIT ? OFFSET ?
		`
		args = []interface{}{ticker, limit, offset}
	}

	dbMutex.Lock()
	rows, err := db.Query(query, args...)
	dbMutex.Unlock()
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

	// Build list of tickers from user's holdings, including underlying assets for ETFs
	tickerSet := make(map[string]bool)
	for _, h := range holdings {
		tickerSet[h.Ticker] = true

		if isETF, err := db.isETF(h.Ticker, h.ISIN); err == nil && isETF {
			underlyingTickers, err := db.getUnderlyingAssetTickers(h.Ticker, h.ISIN)
			if err == nil {
				for _, ut := range underlyingTickers {
					tickerSet[ut] = true
				}
			}
		}
	}

	tickers := make([]string, 0, len(tickerSet))
	for ticker := range tickerSet {
		tickers = append(tickers, ticker)
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

	dbMutex.Lock()
	rows, err := db.Query(query, args...)
	dbMutex.Unlock()
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
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	ticker, err := resolveTickerOrISIN(identifier)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", identifier, err)
	}

	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
	endOfDay := startOfDay + 86400

	var query string
	var args []interface{}

	if isETF, err := db.isETF(ticker, identifier); err == nil && isETF {
		underlyingTickers, err := db.getUnderlyingAssetTickers(ticker, identifier)
		if err == nil && len(underlyingTickers) > 0 {
			allTickers := append([]string{ticker}, underlyingTickers...)
			placeholders := ""
			args = make([]interface{}, 0, len(allTickers)+2)
			for i, t := range allTickers {
				if i > 0 {
					placeholders += ","
				}
				placeholders += "?"
				args = append(args, t)
			}
			args = append(args, startOfDay, endOfDay)

			query = fmt.Sprintf(`
				SELECT sentiment FROM news
				WHERE ticker IN (%s)
				AND CAST(published_at AS INTEGER) >= ?
				AND CAST(published_at AS INTEGER) < ?
				AND sentiment IS NOT NULL
			`, placeholders)
		} else {
			query = `
				SELECT sentiment FROM news
				WHERE ticker = ?
				AND CAST(published_at AS INTEGER) >= ?
				AND CAST(published_at AS INTEGER) < ?
				AND sentiment IS NOT NULL
			`
			args = []interface{}{ticker, startOfDay, endOfDay}
		}
	} else {
		query = `
			SELECT sentiment FROM news
			WHERE ticker = ?
			AND CAST(published_at AS INTEGER) >= ?
			AND CAST(published_at AS INTEGER) < ?
			AND sentiment IS NOT NULL
		`
		args = []interface{}{ticker, startOfDay, endOfDay}
	}

	dbMutex.Lock()
	rows, err := db.Query(query, args...)
	dbMutex.Unlock()
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
		dbMutex.Lock()
		err := db.QueryRow(`
			SELECT close FROM prices 
			WHERE ticker = ? 
			ORDER BY CAST(date AS INTEGER) DESC 
			LIMIT 1
		`, h.Ticker).Scan(&latestPrice)
		dbMutex.Unlock()
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

		dbMutex.Lock()
		sectorRows, err := db.Query(`SELECT name, percentage FROM sectors WHERE id_holding = ?`, h.IdHolding)
		if err == nil {
			for sectorRows.Next() {
				var name string
				var percentage float64
				if err := sectorRows.Scan(&name, &percentage); err == nil {
					sectorTotals[name] += percentage * holdingWeight
				}
			}
			sectorRows.Close()
		}
		dbMutex.Unlock()

		dbMutex.Lock()
		regionRows, err := db.Query(`SELECT name, percentage FROM regions WHERE id_holding = ?`, h.IdHolding)
		if err == nil {
			for regionRows.Next() {
				var name string
				var percentage float64
				if err := regionRows.Scan(&name, &percentage); err == nil {
					regionTotals[name] += percentage * holdingWeight
				}
			}
			regionRows.Close()
		}
		dbMutex.Unlock()

		// For non-ETF holdings (stocks), count them as 100% of their own sector/region if known
		if !h.Etf {
			// Add the holding itself as its own allocation
			sectorTotals["Individual Stocks"] += holdingWeight * 100
		}
	}

	// Calculate company allocation
	companyTotals := make(map[string]float64)

	for _, h := range holdings {
		holdingWeight := 0.0
		if totalPortfolioValue > 0 {
			holdingWeight = holdingValues[h.IdHolding] / totalPortfolioValue
		}

		if h.Etf {
			dbMutex.Lock()
			assetRows, err := db.Query(`SELECT name FROM assets WHERE id_holding = ? ORDER BY id_asset LIMIT 10`, h.IdHolding)
			assets := make([]string, 0)
			if err == nil {
				for assetRows.Next() {
					var name string
					if err := assetRows.Scan(&name); err == nil {
						assets = append(assets, name)
					}
				}
				assetRows.Close()
			}
			dbMutex.Unlock()

			top10Count := len(assets)
			if top10Count > 0 {
				decay := 0.9
				totalWeight := 0.0
				for i := range top10Count {
					totalWeight += math.Pow(decay, float64(i))
				}

				top10Allocation := 0.0
				for i, assetName := range assets {
					weight := math.Pow(decay, float64(i))
					assetPercentage := (weight / totalWeight) * 100
					companyTotals[assetName] += assetPercentage * holdingWeight
					top10Allocation += assetPercentage
				}

				if top10Allocation < 100 {
					otherPercentage := 100 - top10Allocation
					companyTotals["Other"] += otherPercentage * holdingWeight
				}
			}
		} else {
			companyTotals[h.Name] += holdingWeight * 100
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

	companies := make([]AllocationItem, 0)
	for name, percentage := range companyTotals {
		companies = append(companies, AllocationItem{Name: name, Percentage: percentage})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"total_value": totalPortfolioValue,
		"sectors":     sectors,
		"regions":     regions,
		"companies":   companies,
	})
}

func GetTickerValue(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	ticker, err := resolveTickerOrISIN(identifier)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", identifier, err)
	}

	var latestPrice float64
	dbMutex.Lock()
	err = db.QueryRow(`
		SELECT close FROM prices
		WHERE ticker = ?
		ORDER BY CAST(date AS INTEGER) DESC
		LIMIT 1
	`, ticker).Scan(&latestPrice)
	dbMutex.Unlock()
	if err != nil {
		log.Printf("Error fetching latest price for %s: %v", ticker, err)
		return c.String(http.StatusInternalServerError, "Error retrieving latest price")
	}

	return c.JSON(http.StatusOK, map[string]float64{
		"latest_price": latestPrice,
	})
}

func GetAssetPriceHistory(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	ticker, err := resolveTickerOrISIN(identifier)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", identifier, err)
	}

	interval := c.QueryParam("interval")
	now := time.Now().UTC()

	var intervalSeconds int64
	var startTime time.Time

	switch interval {
	case "5m":
		intervalSeconds = 300
		startTime = now.Add(-24 * time.Hour)
	case "15m":
		intervalSeconds = 900
		startTime = now.Add(-7 * 24 * time.Hour)
	case "1h":
		intervalSeconds = 3600
		startTime = now.Add(-30 * 24 * time.Hour)
	case "4h":
		intervalSeconds = 14400
		startTime = now.Add(-90 * 24 * time.Hour)
	case "1d":
		intervalSeconds = 86400
		startTime = now.Add(-365 * 24 * time.Hour)
	case "1w":
		intervalSeconds = 604800
		startTime = now.Add(-730 * 24 * time.Hour)
	case "1M":
		intervalSeconds = 2592000
		startTime = time.Time{}
	default:
		intervalSeconds = 3600
		startTime = now.Add(-30 * 24 * time.Hour)
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

	dbMutex.Lock()
	rows, err := db.Query(query, ticker, startTimestamp, endTimestamp)
	dbMutex.Unlock()
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

func calculateAnnualizedVolatility(dailyReturns []float64) float64 {
	if len(dailyReturns) < 2 {
		return 0
	}
	sum := 0.0
	for _, r := range dailyReturns {
		sum += r
	}
	mean := sum / float64(len(dailyReturns))
	variance := 0.0
	for _, r := range dailyReturns {
		diff := r - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(dailyReturns)))
	return stdDev * math.Sqrt(252) * 100
}

func calculateCalmarRatio(cagrPct float64, maxDrawdownPct float64) float64 {
	dd := math.Abs(maxDrawdownPct)
	if dd == 0 {
		return 0
	}
	return cagrPct / dd
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

	dbMutex.Lock()
	rows, err := db.Query(query, args...)
	dbMutex.Unlock()
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
		weightedTER += (holding.TER / 100) * value
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
	isin := c.QueryParam("isin")

	if ticker == "" && isin == "" {
		return c.String(http.StatusBadRequest, "ticker or isin parameter is required")
	}

	identifier := ticker
	if identifier == "" {
		identifier = isin
	}

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

	dbMutex.Lock()
	rows, err := db.Query(query, identifier, startTime.Unix(), now.Unix())
	dbMutex.Unlock()
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

	var dailyReturns []float64
	for i := 1; i < len(prices); i++ {
		if prices[i-1] > 0 {
			ret := (prices[i] - prices[i-1]) / prices[i-1]
			dailyReturns = append(dailyReturns, ret)
		}
	}

	yoyReturn := calculateYoYReturn(prices, timestamps)
	maxDD, avgDD := calculateDrawdowns(prices)
	sortinoRatio := calculateSortinoRatio(dailyReturns, 0.0)

	currentPrice := prices[len(prices)-1]

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		holdings = []Holding{}
	}

	var totalQuantity float64
	var totalCost float64
	var ter float64

	for _, h := range holdings {
		if h.Ticker == identifier || h.ISIN == identifier {
			totalQuantity += h.Quantity
			totalCost += h.PurchasePrice * h.Quantity
			ter = h.TER
		}
	}

	currentValue := currentPrice * totalQuantity
	gainLoss := currentValue - totalCost
	gainLossPct := 0.0
	if totalCost > 0 {
		gainLossPct = gainLoss / totalCost * 100
	}

	stats := AssetStats{
		Ticker:       identifier,
		YoYReturn:    math.Round(yoyReturn*100) / 100,
		MaxDrawdown:  math.Round(maxDD*100) / 100,
		AvgDrawdown:  math.Round(avgDD*100) / 100,
		SortinoRatio: math.Round(sortinoRatio*100) / 100,
		TER:          ter,
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

	portfolioSummary, err := db.getPortfolioDailySummary(userID, date)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error retrieving portfolio summary")
	}

	return c.JSON(http.StatusOK, portfolioSummary)
}

func getAssetDailySentimentSummary(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	date := c.QueryParam("date")
	if identifier == "" || date == "" {
		return c.String(http.StatusBadRequest, "ticker and date parameters are required")
	}

	ticker, err := resolveTickerOrISIN(identifier)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", identifier, err)
	}

	var sentimentSummary *DailySentiment
	sentimentSummary, err = db.getHoldingDailySummary(ticker, date)

	isETF, etfErr := db.isETF(ticker, identifier)
	if etfErr == nil && isETF {
		underlyingTickers, utErr := db.getUnderlyingAssetTickers(ticker, identifier)
		if utErr == nil && len(underlyingTickers) > 0 {
			var totalSentiment float64
			count := 0
			var summaries []string

			if sentimentSummary != nil && !math.IsNaN(sentimentSummary.Sentiment) {
				totalSentiment = sentimentSummary.Sentiment
				count = 1
				if sentimentSummary.Summary != "" {
					summaries = append(summaries, sentimentSummary.Summary)
				}
			}

			for _, ut := range underlyingTickers {
				ds, dsErr := db.getHoldingDailySummary(ut, date)
				if dsErr == nil && ds != nil && !math.IsNaN(ds.Sentiment) {
					totalSentiment += ds.Sentiment
					count++
					if ds.Summary != "" {
						summaries = append(summaries, ds.Summary)
					}
				}
			}

			if count > 0 {
				if sentimentSummary == nil {
					sentimentSummary = &DailySentiment{
						Ticker: ticker,
						Date:   date,
					}
				}
				sentimentSummary.Sentiment = totalSentiment / float64(count)
				if len(summaries) > 0 {
					combinedSummary := "Combined ETF sentiment: "
					for i, s := range summaries {
						if i > 0 {
							combinedSummary += " | "
						}
						combinedSummary += s
					}
					sentimentSummary.Summary = combinedSummary
				}
			}
		}
	}

	if sentimentSummary != nil {
		return c.JSON(http.StatusOK, sentimentSummary)
	}

	if err != nil && err.Error() == "sql: no rows in result set" {
		return c.JSON(http.StatusOK, nil)
	}

	return c.String(http.StatusInternalServerError, "Error retrieving asset sentiment summary")
}

func GetAssetSentiments(c echo.Context) error {
	identifier := c.QueryParam("ticker")
	if identifier == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	ticker, err := resolveTickerOrISIN(identifier)
	if err != nil {
		log.Printf("Error resolving ticker/ISIN %s: %v", identifier, err)
	}

	var query string
	var args []interface{}

	if isETF, err := db.isETF(ticker, identifier); err == nil && isETF {
		underlyingTickers, err := db.getUnderlyingAssetTickers(ticker, identifier)
		if err == nil && len(underlyingTickers) > 0 {
			allTickers := append([]string{ticker}, underlyingTickers...)
			placeholders := ""
			args = make([]interface{}, 0, len(allTickers))
			for i, t := range allTickers {
				if i > 0 {
					placeholders += ","
				}
				placeholders += "?"
				args = append(args, t)
			}

			query = fmt.Sprintf(`
				SELECT published_at, sentiment
				FROM news
				WHERE ticker IN (%s)
				AND sentiment IS NOT NULL
				ORDER BY CAST(published_at AS INTEGER) DESC
				LIMIT 100
			`, placeholders)
		} else {
			query = `
				SELECT published_at, sentiment
				FROM news
				WHERE ticker = ?
				AND sentiment IS NOT NULL
				ORDER BY CAST(published_at AS INTEGER) DESC
				LIMIT 100
			`
			args = []interface{}{ticker}
		}
	} else {
		query = `
			SELECT published_at, sentiment
			FROM news
			WHERE ticker = ?
			AND sentiment IS NOT NULL
			ORDER BY CAST(published_at AS INTEGER) DESC
			LIMIT 100
		`
		args = []interface{}{ticker}
	}

	dbMutex.Lock()
	rows, err := db.Query(query, args...)
	dbMutex.Unlock()
	if err != nil {
		log.Printf("Error querying asset sentiments: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving sentiments")
	}
	defer rows.Close()

	type SentimentEntry struct {
		PublishedAt int64   `json:"published_at"`
		Sentiment   float64 `json:"sentiment"`
	}

	var sentiments []SentimentEntry
	for rows.Next() {
		var publishedAtStr string
		var sentiment float64
		if err := rows.Scan(&publishedAtStr, &sentiment); err != nil {
			log.Printf("Error scanning sentiment row: %v", err)
			continue
		}
		publishedAt, _ := strconv.ParseInt(publishedAtStr, 10, 64)
		sentiments = append(sentiments, SentimentEntry{
			PublishedAt: publishedAt,
			Sentiment:   sentiment,
		})
	}

	return c.JSON(http.StatusOK, sentiments)
}

func fetchAssetDetails(isin string) error {
	url := fmt.Sprintf("%s/stock/%s", BASE_URL, isin)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error fetching asset details for %s: %v", isin, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Error fetching asset details for %s: status %d", isin, resp.StatusCode)
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	var response struct {
		ISIN    string `json:"isin"`
		Metrics struct {
			MarketCap     string  `json:"market_cap"`
			MarketCapEur  string  `json:"market_cap_eur"`
			Country       string  `json:"country"`
			Sector        string  `json:"sector"`
			DividendYield float64 `json:"dividend_yield"`
			Eps           float64 `json:"eps"`
			PbRatio       float64 `json:"pb_ratio"`
			PeRatio       float64 `json:"pe_ratio"`
		} `json:"metrics"`
		Financials struct {
			Revenue      string  `json:"revenue"`
			NetIncome    string  `json:"net_income"`
			ProfitMargin float64 `json:"profit_margin"`
		} `json:"financials"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Printf("Error decoding asset details for %s: %v", isin, err)
		return err
	}

	newDetail := AssetDetails{
		Ticker:        isin,
		ISIN:          response.ISIN,
		MarketCap:     response.Metrics.MarketCap,
		MarketCapEur:  response.Metrics.MarketCapEur,
		Country:       response.Metrics.Country,
		Sector:        response.Metrics.Sector,
		Eps:           fmt.Sprintf("%.2f", response.Metrics.Eps),
		PbRatio:       fmt.Sprintf("%.2f", response.Metrics.PbRatio),
		PeRatio:       fmt.Sprintf("%.2f", response.Metrics.PeRatio),
		DividendYield: fmt.Sprintf("%.2f", response.Metrics.DividendYield),
		Revenue:       response.Financials.Revenue,
		NetIncome:     response.Financials.NetIncome,
		ProfitMargin:  fmt.Sprintf("%.2f", response.Financials.ProfitMargin),
		Date:          fmt.Sprintf("%d", time.Now().UTC().Unix()),
	}

	newDetail.Hash = newDetail.hashDetails()

	latestDetail, err := db.getLatestAssetDetails(isin)
	if err != nil {
		log.Printf("Error getting latest asset details for %s: %v", isin, err)
		return err
	}

	if latestDetail == nil || latestDetail.Hash != newDetail.Hash {
		err = db.addAssetDetails(newDetail)
		if err != nil {
			log.Printf("Error saving asset details for %s: %v", isin, err)
			return err
		}
		log.Printf("Added new asset details for %s", isin)
	} else {
		log.Printf("No changes in asset details for %s, keeping existing record", isin)
	}

	return nil
}

func fetchAssetDetailsPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Fetching asset details for all stock holdings and ETF components...")

		processedISINs := make(map[string]bool)

		allHoldings, err := db.Query(`SELECT isin, ticker, etf FROM holdings`)
		if err != nil {
			log.Printf("Error getting all holdings: %v", err)
			continue
		}

		var stockHoldings []struct {
			ISIN   string
			Ticker string
			IsETF  bool
		}

		for allHoldings.Next() {
			var h struct {
				ISIN   string
				Ticker string
				IsETF  bool
			}
			if err := allHoldings.Scan(&h.ISIN, &h.Ticker, &h.IsETF); err == nil {
				if !h.IsETF && h.ISIN != "" && h.ISIN != "N/A" {
					stockHoldings = append(stockHoldings, h)
				}
			}
		}
		allHoldings.Close()

		for _, holding := range stockHoldings {
			if !processedISINs[holding.ISIN] {
				err := fetchAssetDetails(holding.ISIN)
				if err != nil {
					log.Printf("Failed to fetch asset details for %s (ISIN: %s): %v", holding.Ticker, holding.ISIN, err)
				} else {
					processedISINs[holding.ISIN] = true
				}
				time.Sleep(2 * time.Second)
			}
		}

		assetRows, err := db.Query(`SELECT DISTINCT isin, ticker FROM assets WHERE isin != '' AND isin != 'N/A'`)
		if err != nil {
			log.Printf("Error getting assets: %v", err)
		} else {
			for assetRows.Next() {
				var assetISIN, assetTicker string
				if err := assetRows.Scan(&assetISIN, &assetTicker); err == nil {
					if !processedISINs[assetISIN] {
						err := fetchAssetDetails(assetISIN)
						if err != nil {
							log.Printf("Failed to fetch asset details for %s (ISIN: %s): %v", assetTicker, assetISIN, err)
						} else {
							processedISINs[assetISIN] = true
						}
						time.Sleep(2 * time.Second)
					}
				}
			}
			assetRows.Close()
		}

		log.Printf("Finished fetching asset details for %d unique ISINs", len(processedISINs))
	}
}

func getAssetDetails(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	details, err := db.getAssetDetailsHistory(ticker)
	if err != nil {
		log.Printf("Error getting asset details: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving asset details")
	}

	return c.JSON(http.StatusOK, details)
}

func getLatestAssetDetailsEndpoint(c echo.Context) error {
	ticker := c.QueryParam("ticker")
	if ticker == "" {
		return c.String(http.StatusBadRequest, "Ticker is required")
	}

	detail, err := db.getLatestAssetDetails(ticker)
	if err != nil {
		log.Printf("Error getting latest asset details: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving asset details")
	}

	if detail == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"message": "No details found"})
	}

	return c.JSON(http.StatusOK, detail)
}

func getOldHistoricPriceData(ticker string) ([]Price, error) {
	url := fmt.Sprintf("%s/stock/history/%s", BASE_URL, ticker)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error fetching historic price data for %s: %v", ticker, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("Historic data not found for %s (404), skipping", ticker)
		return []Price{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to fetch historic data: %s", resp.Status)
		log.Printf("Historic data API error for %s: %s", ticker, resp.Status)
		return nil, err
	}

	var response struct {
		History []struct {
			Timestamp int64   `json:"timestamp"`
			Open      float64 `json:"open"`
			High      float64 `json:"high"`
			Low       float64 `json:"low"`
			Close     float64 `json:"close"`
			Volume    float64 `json:"volume"`
		} `json:"history"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Printf("Error decoding historic price data for %s: %v", ticker, err)
		return nil, err
	}

	var prices []Price
	for _, candle := range response.History {
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
		prices = append(prices, price)
	}

	log.Printf("Retrieved %d historic price candles for %s", len(prices), ticker)
	return prices, nil
}

func convertIsinToTicker(isin string) (string, error) {
	encodedIsin := url.QueryEscape(isin)
	url := fmt.Sprintf("%s/isin_to_ticker?isin=%s", BASE_URL, encodedIsin)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error converting ISIN to ticker for %s: %v", isin, err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("ISIN to ticker conversion not found for %s (404), skipping", isin)
		return "", nil
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to convert ISIN to ticker: %s", resp.Status)
		log.Printf("ISIN to ticker API error for %s: %s", isin, resp.Status)
		return "", err
	}

	var response struct {
		Ticker string `json:"ticker"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Printf("Error decoding ISIN to ticker response for %s: %v", isin, err)
		return "", err
	}

	return response.Ticker, nil
}

func convertTickerToIsin(ticker string) (string, error) {
	encodedTicker := url.QueryEscape(ticker)
	url := fmt.Sprintf("%s/ticker_to_isin?ticker=%s", BASE_URL, encodedTicker)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error converting ticker to ISIN for %s: %v", ticker, err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("Ticker to ISIN conversion not found for %s (404), skipping", ticker)
		return "", nil
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("failed to convert ticker to ISIN: %s", resp.Status)
		log.Printf("Ticker to ISIN API error for %s: %s", ticker, resp.Status)
		return "", err
	}

	var response struct {
		ISIN string `json:"isin"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Printf("Error decoding ticker to ISIN response for %s: %v", ticker, err)
		return "", err
	}

	return response.ISIN, nil
}

func fetchOldPriceDataPeriodic(interval time.Duration) {
	tickerTimer := time.NewTicker(interval)
	defer tickerTimer.Stop()

	for range tickerTimer.C {
		log.Println("Fetching old historic price data for all assets...")
		tickers, err := db.getUniqueTickers()
		if err != nil {
			log.Printf("Error getting unique tickers: %v", err)
			continue
		}

		isins, err := db.getUniqueISINs()
		if err != nil {
			log.Printf("Error getting unique ISINs: %v", err)
			continue
		}

		for _, isin := range isins {
			ticker, err := convertIsinToTicker(isin)
			if err != nil {
				log.Printf("Error converting ISIN %s to ticker: %v", isin, err)
				continue
			}
			if ticker != "" {
				tickers = append(tickers, ticker)
			}
		}

		for _, tickerSymbol := range tickers {
			prices, err := getOldHistoricPriceData(tickerSymbol)
			if err != nil {
				log.Printf("Failed to fetch historic price data for %s: %v", tickerSymbol, err)
				continue
			}

			if len(prices) > 0 {
				err = db.addPrices(prices)
				if err != nil {
					log.Printf("Failed to insert historic prices for %s: %v", tickerSymbol, err)
				} else {
					log.Printf("Inserted %d historic prices for %s", len(prices), tickerSymbol)
				}
			}

			time.Sleep(200 * time.Millisecond)
		}

		log.Println("Finished fetching old historic price data for all assets")
	}
}

func fillMissingTickerIsinPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Filling missing ticker/ISIN mappings for assets...")

		dbMutex.RLock()
		rows, err := db.Query(`SELECT DISTINCT ticker, isin FROM assets`)
		if err != nil {
			dbMutex.RUnlock()
			log.Printf("Error querying assets: %v", err)
			continue
		}
		type AssetEntry struct {
			Ticker string
			ISIN   string
		}
		var assets []AssetEntry
		for rows.Next() {
			var entry AssetEntry
			if err := rows.Scan(&entry.Ticker, &entry.ISIN); err == nil {
				assets = append(assets, entry)
			}
		}
		rows.Close()
		dbMutex.RUnlock()

		for _, asset := range assets {
			if (asset.ISIN == "" || asset.ISIN == "N/A") && (asset.Ticker != "" && asset.Ticker != "N/A") {
				isin, err := convertTickerToIsin(asset.Ticker)
				if err != nil {
					log.Printf("Error converting ticker %s to ISIN: %v", asset.Ticker, err)
					continue
				}
				if isin != "" {
					updateErr := db.updateAssetISIN(asset.Ticker, isin)
					if updateErr != nil {
						log.Printf("Error updating ISIN for ticker %s: %v", asset.Ticker, updateErr)
					} else {
						log.Printf("Updated ISIN for ticker %s to %s", asset.Ticker, isin)
					}
				}
			}
			if (asset.Ticker == "" || asset.Ticker == "N/A") && (asset.ISIN != "" && asset.ISIN != "N/A") {
				ticker, err := convertIsinToTicker(asset.ISIN)
				if err != nil {
					log.Printf("Error converting ISIN %s to ticker: %v", asset.ISIN, err)
					continue
				}
				if ticker != "" {
					updateErr := db.updateAssetTicker(asset.ISIN, ticker)
					if updateErr != nil {
						log.Printf("Error updating ticker for ISIN %s: %v", asset.ISIN, updateErr)
					} else {
						log.Printf("Updated ticker for ISIN %s to %s", asset.ISIN, ticker)
					}
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		log.Println("Finished filling missing ticker/ISIN mappings for assets")

		dbMutex.RLock()
		rows, err = db.Query(`SELECT DISTINCT ticker, isin FROM holdings`)
		if err != nil {
			dbMutex.RUnlock()
			log.Printf("Error querying holdings: %v", err)
			continue
		}
		var holdings []AssetEntry
		for rows.Next() {
			var entry AssetEntry
			if err := rows.Scan(&entry.Ticker, &entry.ISIN); err == nil {
				holdings = append(holdings, entry)
			}
		}
		rows.Close()
		dbMutex.RUnlock()

		for _, holding := range holdings {
			if (holding.ISIN == "" || holding.ISIN == "N/A") && (holding.Ticker != "" && holding.Ticker != "N/A") {
				isin, err := convertTickerToIsin(holding.Ticker)
				if err != nil {
					log.Printf("Error converting ticker %s to ISIN: %v", holding.Ticker, err)
					continue
				}
				if isin != "" {
					updateErr := db.updateHoldingISIN(holding.Ticker, isin)
					if updateErr != nil {
						log.Printf("Error updating ISIN for ticker %s: %v", holding.Ticker, updateErr)
					} else {
						log.Printf("Updated ISIN for ticker %s to %s", holding.Ticker, isin)
					}
				}
			}
			if (holding.Ticker == "" || holding.Ticker == "N/A") && (holding.ISIN != "" && holding.ISIN != "N/A") {
				ticker, err := convertIsinToTicker(holding.ISIN)
				if err != nil {
					log.Printf("Error converting ISIN %s to ticker: %v", holding.ISIN, err)
					continue
				}
				if ticker != "" {
					updateErr := db.updateHoldingTicker(holding.ISIN, ticker)
					if updateErr != nil {
						log.Printf("Error updating ticker for ISIN %s: %v", holding.ISIN, updateErr)
					} else {
						log.Printf("Updated ticker for ISIN %s to %s", holding.ISIN, ticker)
					}
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		log.Println("Finished filling missing ticker/ISIN mappings for holdings")
	}
}

type HoldingReturn struct {
	Ticker             string  `json:"ticker"`
	Name               string  `json:"name"`
	ISIN               string  `json:"isin"`
	TotalReturn        float64 `json:"total_return"`
	VsBenchmark        float64 `json:"vs_benchmark"`
	CurrentPrice       float64 `json:"current_price"`
	Weight             float64 `json:"weight"`
	Return1M           float64 `json:"return_1m"`
	Return3M           float64 `json:"return_3m"`
	DrawdownFromPeak   float64 `json:"drawdown_from_peak"`
	MeanReversionScore float64 `json:"mean_reversion_score"`
	Signal             string  `json:"signal"`
}

type BackTestResult struct {
	PortfolioValues     []float64                `json:"portfolio_values"`
	BenchmarkValues     []float64                `json:"benchmark_values"`
	Timestamps          []int64                  `json:"timestamps"`
	CAGRPortfolio       float64                  `json:"cagr_portfolio"`
	CAGRBenchmark       float64                  `json:"cagr_benchmark"`
	MaxDDPortfolio      float64                  `json:"max_drawdown_portfolio"`
	MaxDDBenchmark      float64                  `json:"max_drawdown_benchmark"`
	SharpePortfolio     float64                  `json:"sharpe_ratio_portfolio"`
	SharpeBenchmark     float64                  `json:"sharpe_ratio_benchmark"`
	SortinoPortfolio    float64                  `json:"sortino_ratio_portfolio"`
	SortinoBenchmark    float64                  `json:"sortino_ratio_benchmark"`
	VolatilityPortfolio float64                  `json:"volatility_portfolio"`
	VolatilityBenchmark float64                  `json:"volatility_benchmark"`
	CalmarPortfolio     float64                  `json:"calmar_ratio_portfolio"`
	CalmarBenchmark     float64                  `json:"calmar_ratio_benchmark"`
	HoldingReturns      map[string]HoldingReturn `json:"holding_returns"`
}

func creteBackTestForPortfolio(userID string, startDate string, endDate string, benchmark string, tickerFilter []string) (*BackTestResult, error) {
	startTime, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start date: %v", err)
	}

	endTime, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end date: %v", err)
	}

	benchmarkPrices, err := getOldHistoricPriceData(benchmark)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch benchmark prices: %v", err)
	}

	holdings, err := db.getHoldingsByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch holdings: %v", err)
	}

	if len(tickerFilter) > 0 {
		filterSet := make(map[string]bool, len(tickerFilter))
		for _, t := range tickerFilter {
			filterSet[t] = true
		}
		filtered := make([]Holding, 0, len(holdings))
		for _, h := range holdings {
			if filterSet[h.Ticker] {
				filtered = append(filtered, h)
			}
		}
		holdings = filtered
	}

	if len(holdings) == 0 {
		return nil, fmt.Errorf("no holdings found for user")
	}

	holdingPrices := make(map[string][]Price)
	for _, holding := range holdings {
		prices, err := getOldHistoricPriceData(holding.Ticker)
		if err != nil {
			log.Printf("Warning: Could not fetch prices for %s: %v", holding.Ticker, err)
			continue
		}
		holdingPrices[holding.Ticker] = prices
	}

	var timestamps []int64
	portfolioValues := make(map[int64]float64)
	benchmarkValues := make(map[int64]float64)

	for _, price := range benchmarkPrices {
		timestamp, _ := strconv.ParseInt(price.Date, 10, 64)
		if timestamp >= startTime.Unix() && timestamp <= endTime.Unix() {
			timestamps = append(timestamps, timestamp)
			benchmarkValues[timestamp] = price.Close
		}
	}

	if len(timestamps) == 0 {
		return nil, fmt.Errorf("no benchmark data in date range")
	}

	holdingStartPrices := make(map[string]float64, len(holdings))
	totalInitialValue := 0.0
	for _, holding := range holdings {
		prices, exists := holdingPrices[holding.Ticker]
		if !exists {
			continue
		}
		var startPrice float64
		minDiff := int64(math.MaxInt64)
		for _, price := range prices {
			priceTs, _ := strconv.ParseInt(price.Date, 10, 64)
			diff := priceTs - startTime.Unix()
			if diff >= 0 && diff < minDiff {
				minDiff = diff
				startPrice = price.Close
			}
		}
		if startPrice > 0 {
			holdingStartPrices[holding.Ticker] = startPrice
			totalInitialValue += startPrice * holding.Quantity
		}
	}

	holdingWeights := make(map[string]float64, len(holdings))
	if totalInitialValue > 0 {
		for _, holding := range holdings {
			if sp, ok := holdingStartPrices[holding.Ticker]; ok {
				holdingWeights[holding.Ticker] = (sp * holding.Quantity) / totalInitialValue
			}
		}
	}

	for _, ts := range timestamps {
		portfolioValue := 0.0
		for _, holding := range holdings {
			startPrice, hasStart := holdingStartPrices[holding.Ticker]
			weight, hasWeight := holdingWeights[holding.Ticker]
			if !hasStart || !hasWeight || startPrice == 0 {
				continue
			}
			prices, exists := holdingPrices[holding.Ticker]
			if !exists {
				continue
			}
			var closestPrice float64
			minDiff := int64(math.MaxInt64)
			for _, price := range prices {
				priceTs, _ := strconv.ParseInt(price.Date, 10, 64)
				diff := int64(math.Abs(float64(ts - priceTs)))
				if diff < minDiff && priceTs <= ts {
					minDiff = diff
					closestPrice = price.Close
				}
			}
			if closestPrice > 0 {
				portfolioValue += weight * (closestPrice / startPrice)
			}
		}
		portfolioValues[ts] = portfolioValue
	}

	portfolioValuesSlice := make([]float64, len(timestamps))
	benchmarkValuesSlice := make([]float64, len(timestamps))

	initialPortfolio := portfolioValues[timestamps[0]]
	initialBenchmark := benchmarkValues[timestamps[0]]

	for i, ts := range timestamps {
		portfolioValuesSlice[i] = (portfolioValues[ts] / initialPortfolio) * 100
		benchmarkValuesSlice[i] = (benchmarkValues[ts] / initialBenchmark) * 100
	}

	portfolioReturns := make([]float64, len(portfolioValuesSlice)-1)
	benchmarkReturns := make([]float64, len(benchmarkValuesSlice)-1)

	for i := 1; i < len(portfolioValuesSlice); i++ {
		portfolioReturns[i-1] = (portfolioValuesSlice[i] - portfolioValuesSlice[i-1]) / portfolioValuesSlice[i-1]
		benchmarkReturns[i-1] = (benchmarkValuesSlice[i] - benchmarkValuesSlice[i-1]) / benchmarkValuesSlice[i-1]
	}

	years := float64(endTime.Sub(startTime).Hours()) / (24 * 365.25)
	cagrPortfolio := (math.Pow(portfolioValuesSlice[len(portfolioValuesSlice)-1]/100, 1/years) - 1) * 100
	cagrBenchmark := (math.Pow(benchmarkValuesSlice[len(benchmarkValuesSlice)-1]/100, 1/years) - 1) * 100

	maxDDPortfolio, _ := calculateDrawdowns(portfolioValuesSlice)
	maxDDBenchmark, _ := calculateDrawdowns(benchmarkValuesSlice)

	sharpePortfolio := calculateSharpeRatio(portfolioReturns, 0.0)
	sharpeBenchmark := calculateSharpeRatio(benchmarkReturns, 0.0)

	sortinoPortfolio := calculateSortinoRatio(portfolioReturns, 0.0)
	sortinoBenchmark := calculateSortinoRatio(benchmarkReturns, 0.0)

	volatilityPortfolio := calculateAnnualizedVolatility(portfolioReturns)
	volatilityBenchmark := calculateAnnualizedVolatility(benchmarkReturns)

	calmarPortfolio := calculateCalmarRatio(cagrPortfolio, maxDDPortfolio)
	calmarBenchmark := calculateCalmarRatio(cagrBenchmark, maxDDBenchmark)

	benchmarkTotalReturn := benchmarkValuesSlice[len(benchmarkValuesSlice)-1] - 100
	holdingReturnMap := make(map[string]HoldingReturn, len(holdings))
	totalEndValue := 0.0
	holdingEndPrices := make(map[string]float64, len(holdings))

	for _, holding := range holdings {
		prices, exists := holdingPrices[holding.Ticker]
		if !exists {
			continue
		}
		var endPrice float64
		var maxTs int64 = -1
		for _, price := range prices {
			priceTs, _ := strconv.ParseInt(price.Date, 10, 64)
			if priceTs <= endTime.Unix() && priceTs > maxTs {
				maxTs = priceTs
				endPrice = price.Close
			}
		}
		holdingEndPrices[holding.Ticker] = endPrice
		totalEndValue += endPrice * holding.Quantity
	}

	for _, holding := range holdings {
		prices, exists := holdingPrices[holding.Ticker]
		if !exists {
			continue
		}
		var startPrice float64
		minStartDiff := int64(math.MaxInt64)
		var endPrice float64
		var maxTs int64 = -1
		for _, price := range prices {
			priceTs, _ := strconv.ParseInt(price.Date, 10, 64)
			diff := priceTs - startTime.Unix()
			if diff >= 0 && diff < minStartDiff {
				minStartDiff = diff
				startPrice = price.Close
			}
			if priceTs <= endTime.Unix() && priceTs > maxTs {
				maxTs = priceTs
				endPrice = price.Close
			}
		}
		if startPrice == 0 || endPrice == 0 {
			continue
		}
		totalReturn := ((endPrice - startPrice) / startPrice) * 100
		vsBenchmark := totalReturn - benchmarkTotalReturn
		weight := 0.0
		if totalEndValue > 0 {
			weight = (holdingEndPrices[holding.Ticker] * holding.Quantity / totalEndValue) * 100
		}

		var sortedPrices []Price
		for _, p := range prices {
			priceTs, _ := strconv.ParseInt(p.Date, 10, 64)
			if priceTs >= startTime.Unix() && priceTs <= endTime.Unix() {
				sortedPrices = append(sortedPrices, p)
			}
		}
		sort.Slice(sortedPrices, func(i, j int) bool {
			ts1, _ := strconv.ParseInt(sortedPrices[i].Date, 10, 64)
			ts2, _ := strconv.ParseInt(sortedPrices[j].Date, 10, 64)
			return ts1 < ts2
		})

		var return1M, return3M, drawdownFromPeak float64
		n := len(sortedPrices)
		if n > 0 {
			peakPrice := sortedPrices[0].Close
			for _, p := range sortedPrices {
				if p.Close > peakPrice {
					peakPrice = p.Close
				}
			}
			if peakPrice > 0 {
				drawdownFromPeak = ((endPrice - peakPrice) / peakPrice) * 100
			}
			idx1M := n - 21
			if idx1M < 0 {
				idx1M = 0
			}
			price1MAgo := sortedPrices[idx1M].Close
			if price1MAgo > 0 {
				return1M = ((endPrice - price1MAgo) / price1MAgo) * 100
			}
			idx3M := n - 63
			if idx3M < 0 {
				idx3M = 0
			}
			price3MAgo := sortedPrices[idx3M].Close
			if price3MAgo > 0 {
				return3M = ((endPrice - price3MAgo) / price3MAgo) * 100
			}
		}

		var meanReversionScore float64
		if n > 21 {
			totalDays := float64(n)
			avgDailyReturn := (endPrice - startPrice) / startPrice / totalDays * 100
			expected1M := avgDailyReturn * 21
			meanReversionScore = expected1M - return1M
		}

		var signal string
		switch {
		case totalReturn > 0 && meanReversionScore > 3 && drawdownFromPeak < -5:
			signal = "BUY_DIP"
		case return1M > 3 && return3M > 5 && vsBenchmark > 0:
			signal = "STRONG"
		case totalReturn < 0 && return1M < 0 && return3M < 0:
			signal = "WEAK"
		default:
			signal = "NEUTRAL"
		}

		holdingReturnMap[holding.Ticker] = HoldingReturn{
			Ticker:             holding.Ticker,
			Name:               holding.Name,
			ISIN:               holding.ISIN,
			TotalReturn:        math.Round(totalReturn*100) / 100,
			VsBenchmark:        math.Round(vsBenchmark*100) / 100,
			CurrentPrice:       math.Round(endPrice*100) / 100,
			Weight:             math.Round(weight*100) / 100,
			Return1M:           math.Round(return1M*100) / 100,
			Return3M:           math.Round(return3M*100) / 100,
			DrawdownFromPeak:   math.Round(drawdownFromPeak*100) / 100,
			MeanReversionScore: math.Round(meanReversionScore*100) / 100,
			Signal:             signal,
		}
	}

	return &BackTestResult{
		PortfolioValues:     portfolioValuesSlice,
		BenchmarkValues:     benchmarkValuesSlice,
		Timestamps:          timestamps,
		CAGRPortfolio:       math.Round(cagrPortfolio*100) / 100,
		CAGRBenchmark:       math.Round(cagrBenchmark*100) / 100,
		MaxDDPortfolio:      math.Round(maxDDPortfolio*100) / 100,
		MaxDDBenchmark:      math.Round(maxDDBenchmark*100) / 100,
		SharpePortfolio:     math.Round(sharpePortfolio*100) / 100,
		SharpeBenchmark:     math.Round(sharpeBenchmark*100) / 100,
		SortinoPortfolio:    math.Round(sortinoPortfolio*100) / 100,
		SortinoBenchmark:    math.Round(sortinoBenchmark*100) / 100,
		VolatilityPortfolio: math.Round(volatilityPortfolio*100) / 100,
		VolatilityBenchmark: math.Round(volatilityBenchmark*100) / 100,
		CalmarPortfolio:     math.Round(calmarPortfolio*100) / 100,
		CalmarBenchmark:     math.Round(calmarBenchmark*100) / 100,
		HoldingReturns:      holdingReturnMap,
	}, nil
}

func calculateSharpeRatio(dailyReturns []float64, riskFreeRate float64) float64 {
	if len(dailyReturns) == 0 {
		return 0
	}

	sum := 0.0
	for _, r := range dailyReturns {
		sum += r
	}
	avgReturn := sum / float64(len(dailyReturns))

	variance := 0.0
	for _, r := range dailyReturns {
		diff := r - avgReturn
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(dailyReturns)))

	if stdDev == 0 {
		return 0
	}

	annualizedReturn := avgReturn * 252
	annualizedStdDev := stdDev * math.Sqrt(252)

	return (annualizedReturn - riskFreeRate) / annualizedStdDev
}

func getBacktest(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	startDate := c.QueryParam("start_date")
	endDate := c.QueryParam("end_date")
	benchmark := c.QueryParam("benchmark")
	tickersParam := c.QueryParam("tickers")

	if startDate == "" || endDate == "" {
		return c.String(http.StatusBadRequest, "start_date and end_date are required")
	}

	if benchmark == "" {
		benchmark = "SPY"
	}

	var tickerFilter []string
	if tickersParam != "" {
		for _, t := range strings.Split(tickersParam, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tickerFilter = append(tickerFilter, t)
			}
		}
	}

	result, err := creteBackTestForPortfolio(userID, startDate, endDate, benchmark, tickerFilter)
	if err != nil {
		log.Printf("Error creating backtest: %v", err)
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to create backtest: %v", err))
	}

	return c.JSON(http.StatusOK, result)
}

func topGainers(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	gainers, err := db.topGainersLosers(userID, 5, 24*time.Hour, true)
	if err != nil {
		log.Printf("Error getting top gainers: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving top gainers")
	}

	return c.JSON(http.StatusOK, gainers)
}

func topLosers(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	losers, err := db.topGainersLosers(userID, 5, 24*time.Hour, false)
	if err != nil {
		log.Printf("Error getting top losers: %v", err)
		return c.String(http.StatusInternalServerError, "Error retrieving top losers")
	}

	return c.JSON(http.StatusOK, losers)
}

type CamtDocument struct {
	XMLName       xml.Name      `xml:"Document"`
	BkToCstmrStmt BkToCstmrStmt `xml:"BkToCstmrStmt"`
}

type BkToCstmrStmt struct {
	Stmt CamtStmt `xml:"Stmt"`
}

type CamtStmt struct {
	Entries []CamtEntry `xml:"Ntry"`
}

type CamtEntry struct {
	NtryRef   string       `xml:"NtryRef"`
	Amt       CamtAmount   `xml:"Amt"`
	CdtDbtInd string       `xml:"CdtDbtInd"`
	Sts       string       `xml:"Sts"`
	BookgDt   CamtDate     `xml:"BookgDt"`
	ValDt     CamtDate     `xml:"ValDt"`
	NtryDtls  CamtNtryDtls `xml:"NtryDtls"`
}

type CamtAmount struct {
	Value float64 `xml:",chardata"`
	Ccy   string  `xml:"Ccy,attr"`
}

type CamtDate struct {
	Dt string `xml:"Dt"`
}

type CamtNtryDtls struct {
	TxDtls CamtTxDtls `xml:"TxDtls"`
}

type CamtTxDtls struct {
	Refs      CamtRefs      `xml:"Refs"`
	RltdPties CamtRltdPties `xml:"RltdPties"`
	RmtInf    CamtRmtInf    `xml:"RmtInf"`
}

type CamtRefs struct {
	AcctSvcrRef string `xml:"AcctSvcrRef"`
}

type CamtRltdPties struct {
	DbtrAcct CamtAcct  `xml:"DbtrAcct"`
	CdtrAcct CamtAcct  `xml:"CdtrAcct"`
	Cdtr     CamtParty `xml:"Cdtr"`
	Dbtr     CamtParty `xml:"Dbtr"`
}

type CamtAcct struct {
	IBAN string `xml:"Id>IBAN"`
	Nm   string `xml:"Nm"`
}

type CamtParty struct {
	Nm string `xml:"Nm"`
}

type CamtRmtInf struct {
	Ustrd string `xml:"Ustrd"`
}

func categorizeWithAI(description string, userID string) string {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return "Other"
	}

	prompt := fmt.Sprintf(`Classify this bank transaction into exactly one category from this list: Groceries, Dining, Transport, Entertainment, Shopping, Utilities, Healthcare, Insurance, Housing, Investments, Subscriptions, Services, Snacks, Other.

Transaction description: %s

Reply with ONLY the category name, nothing else.`, description)

	reqBody := OpenRouterRequest{
		Model: "deepseek/deepseek-v4-flash",
		Messages: []OpenRouterMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "Other"
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("HTTP-Referer", "http://localhost:8085")
	httpReq.Header.Set("X-Title", "Portfolio Expense Tracker")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "Other"
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var orResp OpenRouterResponse
	if err := json.Unmarshal(respBody, &orResp); err != nil {
		return "Other"
	}

	if len(orResp.Choices) == 0 {
		return "Other"
	}

	category := strings.TrimSpace(orResp.Choices[0].Message.Content)
	if bills.ValidCategory(category) {
		bills.SaveMerchantCategory(bills.NormalizeMerchantKey(description), category, userID, "llm")
		return category
	}
	return "Other"
}

func importBankXML(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No file provided"})
	}

	if file.Size > 10*1024*1024 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "File too large (max 10MB)"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to open file"})
	}
	defer src.Close()

	xmlBytes, err := io.ReadAll(src)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read file"})
	}

	xmlBytes = bytes.ReplaceAll(xmlBytes, []byte(`xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.02"`), []byte(""))
	xmlBytes = bytes.ReplaceAll(xmlBytes, []byte(`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`), []byte(""))

	var doc CamtDocument
	if err := xml.Unmarshal(xmlBytes, &doc); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid XML format"})
	}

	imported := 0
	skipped := 0
	var aiPending []bills.BankTransaction

	for _, entry := range doc.BkToCstmrStmt.Stmt.Entries {
		desc := entry.NtryDtls.TxDtls.RmtInf.Ustrd

		counterpartyName := entry.NtryDtls.TxDtls.RltdPties.CdtrAcct.Nm
		if counterpartyName == "" {
			counterpartyName = entry.NtryDtls.TxDtls.RltdPties.Cdtr.Nm
		}
		if entry.CdtDbtInd == "CRDT" {
			counterpartyName = entry.NtryDtls.TxDtls.RltdPties.DbtrAcct.Nm
			if counterpartyName == "" {
				counterpartyName = entry.NtryDtls.TxDtls.RltdPties.Dbtr.Nm
			}
		}

		if desc == "" && counterpartyName != "" {
			desc = counterpartyName
		} else if desc != "" && counterpartyName != "" {
			desc = desc + " | " + counterpartyName
		}
		if desc == "" {
			desc = "No description"
		}

		tx := bills.BankTransaction{
			NtryRef:     entry.NtryRef,
			AcctSvcrRef: entry.NtryDtls.TxDtls.Refs.AcctSvcrRef,
			Amount:      entry.Amt.Value,
			Currency:    entry.Amt.Ccy,
			Direction:   entry.CdtDbtInd,
			Status:      entry.Sts,
			BookingDate: entry.BookgDt.Dt,
			ValueDate:   entry.ValDt.Dt,
			Description: desc,
			UserID:      userID,
		}

		tx.IsSavingsRoundup = strings.Contains(strings.ToLower(tx.Description), "drobne bokom")
		tx.Category = bills.CategorizeBankTransaction(tx)

		if tx.Category == "Other" && tx.Direction == "DBIT" && !tx.IsSavingsRoundup {
			aiPending = append(aiPending, tx)
		}

		added, err := bills.ImportBankTransaction(tx)
		if err != nil {
			log.Printf("Error importing transaction %s: %v", tx.NtryRef, err)
			continue
		}
		if added {
			imported++
		} else {
			skipped++
		}
	}

	if len(aiPending) > 0 {
		go func(txs []bills.BankTransaction, uid string) {
			log.Printf("Running AI categorization for %d uncategorized transactions", len(txs))
			for _, tx := range txs {
				aiCategory := categorizeWithAI(tx.Description, uid)
				if aiCategory != "Other" {
					log.Printf("AI categorized '%s' as %s", tx.Description[:min(50, len(tx.Description))], aiCategory)
				}
				time.Sleep(300 * time.Millisecond)
			}
		}(aiPending, userID)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"imported": imported,
		"skipped":  skipped,
		"total":    imported + skipped,
	})
}

func getBankTransactions(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	txs, err := bills.GetBankTransactions(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get transactions"})
	}

	if txs == nil {
		txs = []bills.BankTransaction{}
	}
	return c.JSON(http.StatusOK, txs)
}

func getSavingsTransactions(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	txs, err := bills.GetSavingsTransactions(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get savings transactions"})
	}

	if txs == nil {
		txs = []bills.BankTransaction{}
	}
	return c.JSON(http.StatusOK, txs)
}

func getBankStats(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	stats, err := bills.GetBankTransactionStats(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get bank stats"})
	}

	return c.JSON(http.StatusOK, stats)
}

func updateBankTransaction(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	var tx bills.BankTransaction
	if err := c.Bind(&tx); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if tx.ID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing transaction ID"})
	}

	tx.UserID = userID
	if err := bills.UpdateBankTransaction(tx); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update transaction"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Transaction updated"})
}

func deleteBankTransaction(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	txID := c.QueryParam("id")
	if txID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing transaction ID"})
	}

	id, err := strconv.Atoi(txID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid transaction ID"})
	}

	if err := bills.DeleteBankTransaction(id, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete transaction"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Transaction deleted"})
}

type OpenRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterRequest struct {
	Model    string              `json:"model"`
	Messages []OpenRouterMessage `json:"messages"`
}

type OpenRouterChoice struct {
	Message OpenRouterMessage `json:"message"`
}

type OpenRouterResponse struct {
	Choices []OpenRouterChoice `json:"choices"`
}

func computePeriodDates(period string) (string, string) {
	now := time.Now()
	switch period {
	case "week":
		weekday := now.Weekday()
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -int(weekday)+1)
		return monday.Format("2006-01-02"), monday.AddDate(0, 0, 6).Format("2006-01-02")
	case "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start.Format("2006-01-02"), start.AddDate(0, 1, -1).Format("2006-01-02")
	case "quarter":
		qStart := time.Date(now.Year(), ((now.Month()-1)/3)*3+1, 1, 0, 0, 0, 0, now.Location())
		return qStart.Format("2006-01-02"), qStart.AddDate(0, 3, -1).Format("2006-01-02")
	default:
		return "", ""
	}
}

func tryAutoGenerateReport(userID, period string) (string, string, string, error) {
	periodStart, periodEnd := computePeriodDates(period)
	if periodStart == "" {
		return "", "", "", fmt.Errorf("invalid period")
	}

	exists, err := bills.ReportExistsForPeriod(userID, period, periodStart, periodEnd)
	if err != nil {
		return "", "", "", err
	}
	if exists {
		if delErr := bills.DeleteExpenseReportForPeriod(userID, period, periodStart, periodEnd); delErr != nil {
			log.Printf("Error deleting old report for regeneration: %v", delErr)
		}
	}

	expenses, _ := bills.GetExpensesByUserID(userID)
	bankTxs, _ := bills.GetBankTransactions(userID)

	var dataLines []string
	totalOut := 0.0
	totalIn := 0.0
	byCategory := map[string]float64{}

	for _, e := range expenses {
		if e.Date >= periodStart && e.Date <= periodEnd {
			dataLines = append(dataLines, fmt.Sprintf("MANUAL | %s | %s | €%.2f", e.Date, e.Category, e.Amount))
			totalOut += e.Amount
			byCategory[e.Category] += e.Amount
		}
	}
	for _, t := range bankTxs {
		if t.BookingDate >= periodStart && t.BookingDate <= periodEnd {
			dir := "OUT"
			if t.Direction == "CRDT" {
				dir = "IN"
				totalIn += t.Amount
			} else {
				totalOut += t.Amount
				byCategory[t.Category] += t.Amount
			}
			dataLines = append(dataLines, fmt.Sprintf("BANK | %s | %s | %s | €%.2f | %s", t.BookingDate, t.Category, dir, t.Amount, t.Description))
		}
	}

	if len(dataLines) == 0 {
		return periodStart, periodEnd, "No transactions found for this period.", nil
	}

	categoryBreakdown := ""
	for cat, val := range byCategory {
		categoryBreakdown += fmt.Sprintf("  - %s: €%.2f\n", cat, val)
	}

	prompt := fmt.Sprintf(`You are a personal finance analyst. Analyze the following expense data for the period %s to %s (%s).

Summary:
- Total Out: €%.2f
- Total In: €%.2f
- Net: €%.2f
- Transaction count: %d

Category breakdown:
%s

Transactions:
%s

Provide a concise report with these sections in plain text (no markdown):
1. OVERVIEW (2-3 sentences about the period's spending patterns)
2. KEY INSIGHTS (3-4 bullet points with - prefix, highlighting notable spending, trends, or concerns)
3. CATEGORY ANALYSIS (which categories were highest, any unusual spending)
4. SAVINGS OBSERVATIONS
5. RECOMMENDATIONS (2-3 actionable tips to improve finances)

Keep it concise, maximum 500 words.`, periodStart, periodEnd, period, totalOut, totalIn, totalOut-totalIn, len(dataLines), categoryBreakdown, strings.Join(dataLines, "\n"))

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return periodStart, periodEnd, "", fmt.Errorf("OpenRouter API key not configured")
	}

	reqBody := OpenRouterRequest{
		Model: "deepseek/deepseek-v4-flash",
		Messages: []OpenRouterMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return periodStart, periodEnd, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("HTTP-Referer", "http://localhost:8085")
	httpReq.Header.Set("X-Title", "Portfolio Expense Tracker")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return periodStart, periodEnd, "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var orResp OpenRouterResponse
	if err := json.Unmarshal(respBody, &orResp); err != nil {
		return periodStart, periodEnd, "", fmt.Errorf("failed to parse AI response")
	}

	if len(orResp.Choices) == 0 {
		return periodStart, periodEnd, "", fmt.Errorf("no response from AI model")
	}

	summary := orResp.Choices[0].Message.Content

	report := bills.ExpenseReport{
		Period:      period,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Summary:     summary,
		CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
		UserID:      userID,
	}

	if err := bills.SaveExpenseReport(report); err != nil {
		log.Printf("Error saving report for user %s: %v", userID, err)
	}

	return periodStart, periodEnd, summary, nil
}

func generateExpenseReport(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	period := c.QueryParam("period")
	if period != "week" && period != "month" && period != "quarter" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Period must be week, month, or quarter"})
	}

	periodStart, periodEnd, summary, err := tryAutoGenerateReport(userID, period)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"period":       period,
		"period_start": periodStart,
		"period_end":   periodEnd,
		"summary":      summary,
		"created_at":   time.Now().Format("2006-01-02 15:04:05"),
	})
}

func startAutoReportScheduler() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		log.Printf("Auto-report scheduler started (every 30 min)")

		for range ticker.C {
			userIDs, err := bills.GetAllUserIDs()
			if err != nil {
				log.Printf("Auto-report: failed to get user IDs: %v", err)
				continue
			}

			for _, uid := range userIDs {
				for _, period := range []string{"week", "month", "quarter"} {
					start, end := computePeriodDates(period)
					if start == "" {
						continue
					}
					exists, _ := bills.ReportExistsForPeriod(uid, period, start, end)
					if exists {
						continue
					}
					log.Printf("Auto-report: generating %s report for user %s (%s to %s)", period, uid, start, end)
					_, _, summary, err := tryAutoGenerateReport(uid, period)
					if err != nil {
						log.Printf("Auto-report: failed for user %s period %s: %v", uid, period, err)
					} else {
						log.Printf("Auto-report: generated %s report for user %s (%d chars)", period, uid, len(summary))
					}
				}
			}
		}
	}()
}

func getExpenseReports(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	reports, err := bills.GetExpenseReports(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get reports"})
	}

	if reports == nil {
		reports = []bills.ExpenseReport{}
	}
	return c.JSON(http.StatusOK, reports)
}

func getExpenseReport(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	reportID := c.Param("id")
	id, err := strconv.Atoi(reportID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid report ID"})
	}

	report, err := bills.GetExpenseReportByID(id, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Report not found"})
	}

	return c.JSON(http.StatusOK, report)
}

func getExpenses(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	expenses, err := bills.GetExpensesByUserID(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get expenses"})
	}

	if expenses == nil {
		expenses = []bills.Expense{}
	}
	return c.JSON(http.StatusOK, expenses)
}

func addExpense(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	var expense bills.Expense
	if err := c.Bind(&expense); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	expense.UserID = userID

	if expense.Description == "" || expense.Amount <= 0 || expense.Category == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing or invalid required fields"})
	}

	if err := bills.AddExpense(expense); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to add expense"})
	}

	return c.JSON(http.StatusCreated, map[string]string{"message": "Expense added successfully"})
}

func updateExpense(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	var expense bills.Expense
	if err := c.Bind(&expense); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	expense.UserID = userID

	if expense.ID == 0 || expense.Description == "" || expense.Amount <= 0 || expense.Category == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing or invalid required fields"})
	}

	if err := bills.UpdateExpense(expense); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update expense"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Expense updated successfully"})
}

func deleteExpense(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	expenseID := c.QueryParam("id")
	if expenseID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing expense ID"})
	}

	id, err := strconv.Atoi(expenseID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid expense ID"})
	}

	if err := bills.DeleteExpense(id, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete expense"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Expense deleted successfully"})
}

func getExpensesByCategory(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	categoryStats, err := bills.GroupExpensesByCategory(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get category stats"})
	}

	return c.JSON(http.StatusOK, categoryStats)
}

func getExpensesByMonth(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	monthlyStats, err := bills.GroupExpensesByMonth(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get monthly stats"})
	}

	return c.JSON(http.StatusOK, monthlyStats)
}

func getExpensesBiweekly(c echo.Context) error {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	userID := claims.UserID

	biweeklyStats, err := bills.GroupExpensesBiweekly(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get biweekly stats"})
	}

	return c.JSON(http.StatusOK, biweeklyStats)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	SALT = os.Getenv("SALT")
	JWT_SECRET = os.Getenv("JWT_SECRET")
	jwtExpiryHoursStr := os.Getenv("JWT_EXPIRY_HOURS")
	if jwtExpiryHoursStr != "" {
		jwtExpiryHours, parseErr := strconv.Atoi(jwtExpiryHoursStr)
		if parseErr != nil || jwtExpiryHours <= 0 {
			log.Fatal("JWT_EXPIRY_HOURS must be a positive integer")
		}
		JWT_EXPIRY = time.Duration(jwtExpiryHours) * time.Hour
	}

	devMode = os.Getenv("DEV_MODE")
	pythonPort := os.Getenv("BACKEND_PYTHON_PORT")
	serverHost := os.Getenv("SERVER_HOST")

	if devMode == "true" {
		BASE_URL = "http://localhost:" + pythonPort + "/api"
	} else {
		BASE_URL = "http://" + serverHost + ":" + pythonPort + "/api"
	}

	if SALT == "" || JWT_SECRET == "" {
		log.Fatal("Required environment variables (SALT, JWT_SECRET) not set")
	}

	log.Printf("Running in dev mode: %s", devMode)
	log.Printf("Python API URL: %s", BASE_URL)

	sqlDB, err := initDB(false)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer sqlDB.Close()

	err = addPriceIndexes(sqlDB)
	if err != nil {
		log.Fatal("Failed to create price indexes:", err)
	}

	db = &DB{DB: sqlDB}

	err = bills.InitBillDB(sqlDB)
	if err != nil {
		log.Fatal("Failed to initialize bills database:", err)
	}

	log.Println("Database initialized successfully")

	log.Println("Running initial historic price data fetch...")
	go func() {
		tickers, err := db.getUniqueTickers()
		if err != nil {
			log.Printf("Error getting unique tickers for initial fetch: %v", err)
			return
		}

		for _, tickerSymbol := range tickers {
			prices, err := getOldHistoricPriceData(tickerSymbol)
			if err != nil {
				log.Printf("Failed to fetch historic price data for %s: %v", tickerSymbol, err)
				continue
			}

			if len(prices) > 0 {
				err = db.addPrices(prices)
				if err != nil {
					log.Printf("Failed to insert historic prices for %s: %v", tickerSymbol, err)
				} else {
					log.Printf("Inserted %d historic prices for %s", len(prices), tickerSymbol)
				}
			}

			time.Sleep(2 * time.Second)
		}

		log.Println("Finished initial historic price data fetch")
	}()

	go fetchOldPriceDataPeriodic(7 * 24 * time.Hour)
	go fetchNewsPeriodic(45 * time.Minute)
	go fetchPricesPeriodic(10 * time.Minute)
	go fillInBetweenPricesPeriodic(90 * time.Minute)
	go updateSentimentsPeriodic(24 * time.Hour)
	go updateETFDataPeriodic(30 * time.Minute)
	go clearExpiredLoginAttempts()
	go fetchAssetDetailsPeriodic(1 * time.Hour)
	// go fillMissingTickerIsinPeriodic(5 * time.Minute)

	e := echo.New()

	var allowOrigins []string
	if devMode == "true" {
		allowOrigins = []string{"http://localhost:5173"}
	} else {
		allowOrigins = []string{"http://" + serverHost + ":5173"}
	}

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
		ExposeHeaders:    []string{echo.HeaderContentLength},
	}))

	e.POST("/register", addUser)
	e.POST("/login", login)
	e.GET("/health", healthCheck)
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
	protected.GET("/portfolio/holdings", GetHoldings)
	protected.GET("/portfolio/value", GetPortfolioValue)
	protected.GET("/portfolio/history", GetPortfolioValueHistory)
	protected.GET("/portfolio/change", getPortfolioValueChange)
	protected.GET("/portfolio/news", getLatestNewsForPortfolio)
	protected.GET("/portfolio/sentiment", getPortfolioDaySentiment)
	protected.GET("/portfolio/daily_sentiment", getAllPortfolioDaySentiments)
	protected.GET("/portfolio/allocation", getPortfolioAllocation)
	protected.GET("/portfolio/stats", getPortfolioStats)
	protected.GET("/portfolio/backtest", getBacktest)
	protected.GET("/portfolio/top_gainers", topGainers)
	protected.GET("/portfolio/top_losers", topLosers)
	protected.GET("/portfolio/sentiment_history", GetPortfolioSentimentHistory)

	protected.POST("/news/generate-summary", triggerNewsSummary)

	// Asset endpoints
	protected.GET("/asset/sentiments", GetAssetSentiments)
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
	protected.POST("/asset/generate-summary", triggerHoldingSummary)
	protected.GET("/asset/details", getLatestAssetDetailsEndpoint)
	protected.GET("/asset/details/history", getAssetDetails)

	protected.GET("/expenses", getExpenses)
	protected.POST("/expenses", addExpense)
	protected.PUT("/expenses", updateExpense)
	protected.DELETE("/expenses", deleteExpense)
	protected.GET("/expenses/stats/category", getExpensesByCategory)
	protected.GET("/expenses/stats/monthly", getExpensesByMonth)
	protected.GET("/expenses/stats/biweekly", getExpensesBiweekly)

	protected.POST("/bank/import", importBankXML)
	protected.GET("/bank/transactions", getBankTransactions)
	protected.GET("/bank/savings", getSavingsTransactions)
	protected.GET("/bank/stats", getBankStats)
	protected.PUT("/bank/transactions", updateBankTransaction)
	protected.DELETE("/bank/transactions", deleteBankTransaction)

	protected.GET("/expenses/reports", getExpenseReports)
	protected.GET("/expenses/report/:id", getExpenseReport)
	protected.POST("/expenses/report/generate", generateExpenseReport)

	startAutoReportScheduler()

	goPort := os.Getenv("BACKEND_GO_PORT")
	fmt.Printf("Starting server on port %s...\n", goPort)
	e.Logger.Fatal(e.Start(":" + goPort))
}
