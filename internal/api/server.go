package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"portfolio/internal/chat"
	"portfolio/internal/config"
	"portfolio/internal/jobs"
	"portfolio/internal/llm"
	"portfolio/internal/market"
	"portfolio/internal/prices"
	"portfolio/internal/rag"
	"portfolio/internal/store"
)

type Server struct {
	Cfg    config.Config
	DB     *store.DB
	Prices *prices.Store
	Market *market.Client
	LLM    *llm.Client
	RAG    *rag.Indexer
	Chat   *chat.Service
	Jobs   *jobs.Manager
	Web    http.FileSystem
	// Background gates work that reaches external services, so tests stay offline.
	Background bool

	loginMu       sync.Mutex
	loginAttempts map[string][]time.Time
	cooldownMu    sync.Mutex
	cooldowns     map[string]time.Time
}

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

func New(cfg config.Config, db *store.DB, priceStore *prices.Store, marketClient *market.Client,
	llmClient *llm.Client, indexer *rag.Indexer, chatService *chat.Service, manager *jobs.Manager,
	web http.FileSystem) *Server {
	return &Server{
		Cfg: cfg, DB: db, Prices: priceStore, Market: marketClient, LLM: llmClient,
		RAG: indexer, Chat: chatService, Jobs: manager, Web: web, Background: true,
		loginAttempts: map[string][]time.Time{},
		cooldowns:     map[string]time.Time{},
	}
}

func (s *Server) Router() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	// Nothing runs in front of the server, so forwarded headers must not be trusted:
	// otherwise rotating X-Forwarded-For bypasses the login throttle.
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(middleware.Recover())
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:      "1; mode=block",
		ContentTypeNosniff: "nosniff",
		XFrameOptions:      "DENY",
		ReferrerPolicy:     "no-referrer",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; " +
			"base-uri 'self'; form-action 'self'; frame-ancestors 'none'",
	}))
	e.Use(middleware.BodyLimit("8M"))
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{Level: 5}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: s.Cfg.AllowedOrigins(),
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch,
			http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowCredentials: true,
		ExposeHeaders:    []string{echo.HeaderContentLength},
	}))

	e.POST("/register", s.register)
	e.POST("/login", s.login)
	e.GET("/health", s.health)

	protected := e.Group("/api")
	protected.Use(echojwt.WithConfig(echojwt.Config{
		NewClaimsFunc: func(c echo.Context) jwt.Claims { return new(Claims) },
		SigningKey:    []byte(s.Cfg.JWTSecret),
		ErrorHandler: func(c echo.Context, err error) error {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Missing or invalid token"})
		},
	}))

	protected.GET("/profile", s.profile)
	protected.GET("/usage", s.usageSummary)

	protected.GET("/portfolio/holdings", s.getHoldings)
	protected.GET("/portfolio/value", s.getPortfolioValue)
	protected.GET("/portfolio/history", s.getPortfolioHistory)
	protected.GET("/portfolio/change", s.getPortfolioChange)
	protected.GET("/portfolio/news", s.getPortfolioNews)
	protected.GET("/portfolio/sentiment", s.getPortfolioSentiment)
	protected.GET("/portfolio/daily_sentiment", s.getPortfolioDailySentiment)
	protected.GET("/portfolio/allocation", s.getPortfolioAllocation)
	protected.GET("/portfolio/stats", s.getPortfolioStats)
	protected.GET("/portfolio/risk", s.getPortfolioRisk)
	protected.GET("/portfolio/backtest", s.getBacktest)
	protected.GET("/portfolio/top_gainers", s.topGainers)
	protected.GET("/portfolio/top_losers", s.topLosers)
	protected.GET("/portfolio/sentiment_history", s.getSentimentHistory)

	protected.POST("/news/generate-summary", s.generateNewsSummary)
	protected.POST("/news/fetch", s.fetchNews)

	protected.POST("/running-summary/generate", s.generateRunningSummary)
	protected.GET("/running-summary", s.getRunningSummary)
	protected.GET("/running-summary/history", s.getRunningSummaryHistory)

	protected.GET("/asset/sentiments", s.getAssetSentiments)
	protected.POST("/asset/holdings", s.addHolding)
	protected.PUT("/asset/holdings", s.modifyHolding)
	protected.DELETE("/asset/holdings", s.removeHolding)
	protected.GET("/asset/news", s.getAssetNews)
	protected.GET("/asset/sentiment", s.getAssetSentiment)
	protected.GET("/asset/change", s.getAssetChange)
	protected.GET("/asset/value", s.getAssetValue)
	protected.GET("/asset/history", s.getAssetHistory)
	protected.GET("/asset/stats", s.getAssetStats)
	protected.GET("/asset/daily_sentiment", s.getAssetDailySentiment)
	protected.POST("/asset/generate-summary", s.generateHoldingSummary)
	protected.GET("/asset/details", s.getAssetDetails)
	protected.GET("/asset/details/history", s.getAssetDetailsHistory)

	protected.GET("/expenses", s.getExpenses)
	protected.POST("/expenses", s.addExpense)
	protected.PUT("/expenses", s.updateExpense)
	protected.DELETE("/expenses", s.deleteExpense)
	protected.GET("/expenses/stats/category", s.getExpensesByCategory)
	protected.GET("/expenses/stats/monthly", s.getExpensesByMonth)
	protected.GET("/expenses/stats/biweekly", s.getExpensesBiweekly)
	protected.GET("/expenses/categories", s.getExpenseCategories)
	protected.GET("/expenses/rules", s.getMerchantRules)
	protected.DELETE("/expenses/rules", s.deleteMerchantRule)
	protected.GET("/expenses/reports", s.getExpenseReports)
	protected.GET("/expenses/report/:id", s.getExpenseReport)
	protected.POST("/expenses/report/generate", s.generateExpenseReport)

	protected.POST("/bank/import", s.importBankXML)
	protected.GET("/bank/transactions", s.getBankTransactions)
	protected.GET("/bank/savings", s.getSavingsTransactions)
	protected.GET("/bank/stats", s.getBankStats)
	protected.PUT("/bank/transactions", s.updateBankTransaction)
	protected.DELETE("/bank/transactions", s.deleteBankTransaction)

	protected.GET("/search", s.search)
	protected.GET("/articles", s.listArticles)
	protected.GET("/articles/stats", s.articleStats)
	protected.POST("/articles/embed", s.embedArticles)
	protected.GET("/convert_currency", s.convertCurrency)
	protected.GET("/get_price", s.getPrice)

	protected.POST("/chat", s.chatHandler)
	protected.GET("/chat/conversations", s.listConversations)
	protected.GET("/chat/conversations/:id", s.getConversation)
	protected.DELETE("/chat/conversations/:id", s.deleteConversation)
	protected.POST("/chat/reindex", s.reindexRAG)
	protected.GET("/debug/mem", s.debugMem)
	protected.GET("/system/storage", s.getStorage)

	protected.GET("/pins", s.getPins)
	protected.POST("/pins", s.addPin)
	protected.DELETE("/pins", s.removePin)

	protected.GET("/situation/tasks", s.listSituationTasks)
	protected.POST("/situation/tasks", s.createSituationTask)
	protected.PUT("/situation/tasks/:id", s.updateSituationTask)
	protected.DELETE("/situation/tasks/:id", s.deleteSituationTask)
	protected.POST("/situation/tasks/:id/pause", s.pauseSituationTask)
	protected.POST("/situation/tasks/:id/resume", s.resumeSituationTask)
	protected.POST("/situation/tasks/:id/generate", s.generateSituationTask)
	protected.GET("/situation/reports", s.getSituationReports)
	protected.GET("/situation/reports/history", s.getSituationReportHistory)

	s.mountWeb(e)
	return e
}

func (s *Server) userID(c echo.Context) string {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok {
		return ""
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return ""
	}
	return claims.UserID
}

func (s *Server) email(c echo.Context) string {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok {
		return ""
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return ""
	}
	return claims.Email
}

func queryInt(c echo.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.QueryParam(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func queryFloat(c echo.Context, name string, fallback float64) float64 {
	value, err := strconv.ParseFloat(c.QueryParam(name), 64)
	if err != nil {
		return fallback
	}
	return value
}

func tickerList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// allow applies a per key cooldown for expensive generation endpoints.
func (s *Server) allow(key string, window time.Duration) (bool, int) {
	s.cooldownMu.Lock()
	defer s.cooldownMu.Unlock()
	if last, ok := s.cooldowns[key]; ok {
		if elapsed := time.Since(last); elapsed < window {
			return false, int((window - elapsed).Seconds()) + 1
		}
	}
	s.cooldowns[key] = time.Now()
	return true, 0
}
