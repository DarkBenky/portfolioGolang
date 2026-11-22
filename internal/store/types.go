package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
)

type DB struct {
	*sql.DB
}

type User struct {
	ID       string `json:"id"`
	UserName string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Holding struct {
	IDHolding     string  `json:"id_holding"`
	Name          string  `json:"name"`
	Ticker        string  `json:"ticker"`
	ISIN          string  `json:"isin"`
	Exchange      string  `json:"exchange"`
	Policy        string  `json:"policy"`
	UserID        string  `json:"user_id"`
	Currency      string  `json:"currency"`
	Quantity      float64 `json:"quantity"`
	PurchasePrice float64 `json:"purchase_price"`
	TER           float64 `json:"ter"`
	ETF           bool    `json:"etf"`
}

type Asset struct {
	IDAsset   string `json:"id_asset"`
	Name      string `json:"name"`
	Ticker    string `json:"ticker"`
	ISIN      string `json:"isin"`
	Exchange  string `json:"exchange"`
	Sector    string `json:"sector"`
	Region    string `json:"region"`
	IDHolding string `json:"id_holding"`
	Currency  string `json:"currency"`
}

type Sector struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
	IDHolding  string  `json:"id_holding"`
}

type Region struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
	IDHolding  string  `json:"id_holding"`
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
	IDNews      string  `json:"id_news"`
	Title       string  `json:"title"`
	Link        string  `json:"link"`
	PublishedAt string  `json:"published_at"`
	Summary     string  `json:"summary"`
	Text        string  `json:"text"`
	Author      string  `json:"author"`
	IDAsset     string  `json:"-"`
	IDHolding   string  `json:"-"`
	Ticker      string  `json:"ticker"`
	Sentiment   float64 `json:"sentiment"`
}

type DailySentiment struct {
	IDSentiment string  `json:"id_sentiment"`
	Ticker      string  `json:"ticker"`
	Date        string  `json:"date"`
	Summary     string  `json:"summary"`
	Sentiment   float64 `json:"sentiment"`
}

type PortfolioDailySentiment struct {
	IDSentiment string  `json:"id_sentiment"`
	UserID      string  `json:"user_id"`
	Date        string  `json:"date"`
	Summary     string  `json:"summary"`
	Sentiment   float64 `json:"sentiment"`
}

type RunningSummary struct {
	ID                string  `json:"id"`
	UserID            string  `json:"user_id"`
	WindowDays        int     `json:"window_days"`
	Date              string  `json:"date"`
	Summary           string  `json:"summary"`
	Sentiment         float64 `json:"sentiment"`
	SectorPredictions string  `json:"sector_predictions"`
	ThemePredictions  string  `json:"theme_predictions"`
}

type Conversation struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type ChatMessage struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	SearchResults  string `json:"search_results"`
	Status         string `json:"status"`
	CreatedAt      int64  `json:"created_at"`
}

type RagChunk struct {
	ID        string
	Source    string
	Ticker    string
	UserID    string
	Date      string
	Content   string
	Embedding []float32
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Text    string `json:"text"`
}

type GainerLoser struct {
	Asset          *Asset   `json:"asset"`
	Holding        *Holding `json:"holding"`
	PriceChangePct float64  `json:"price_change_pct"`
	RelatedNews    []News   `json:"related_news"`
}

func NewID() string {
	return uuid.NewString()
}

func HashString(values ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(values, "|")))
	return hex.EncodeToString(hash[:])
}

func Now() int64 {
	return time.Now().Unix()
}

func (d *DB) Ping() error {
	return d.DB.Ping()
}
