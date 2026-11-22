package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	"portfolio/internal/api"
	"portfolio/internal/bills"
	"portfolio/internal/chat"
	"portfolio/internal/config"
	"portfolio/internal/jobs"
	"portfolio/internal/llm"
	"portfolio/internal/market"
	"portfolio/internal/prices"
	"portfolio/internal/rag"
	"portfolio/internal/store"
	"portfolio/internal/webui"
)

func main() {
	cfg := config.Load()
	applyMemoryLimit()
	if os.Getenv("PRICE_WAL_PROBE") == "1" {
		os.Exit(prices.RunProbe(cfg.PriceDBPath))
	}

	db, err := store.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatal("database: ", err)
	}
	defer db.Close()
	if err := db.EnsurePinsTable(); err != nil {
		log.Fatal("pins table: ", err)
	}
	if err := db.CreateSituationTables(); err != nil {
		log.Fatal("situation tables: ", err)
	}
	db.RecoverStaleSituationReports()
	if err := bills.InitBillDB(db.DB); err != nil {
		log.Fatal("bills: ", err)
	}

	priceStore, err := prices.Init(cfg.PriceDBPath, db.DB, cfg.ChartMaxPoints)
	if err != nil {
		log.Fatal("price store: ", err)
	}
	defer func() {
		if err := priceStore.Close(); err != nil {
			log.Printf("price store close: %v", err)
		}
	}()

	marketClient := market.NewClient()
	llmClient := llm.New(cfg.OpenRouterKey, cfg.ChatModel, cfg.SessionModel, cfg.CheapModel, cfg.EmbeddingModel, db, cfg.LLMDailyBudgetUSD)
	indexer := rag.New(db, llmClient, marketClient, cfg.EnableEmbeddings, cfg.EmbedArticles, cfg.EmbedDimensions)
	chatService := chat.New(llmClient, db, marketClient, priceStore, indexer)
	manager := jobs.New(db, priceStore, marketClient, llmClient, indexer, jobs.Settings{
		Retention5mDays: cfg.PriceRetention5mDays,
		Retention1hDays: cfg.PriceRetention1hDays,
		Lookback5mDays:  cfg.PriceLookback5mDays,
		SummaryMaxChars: 12000,
		EmbedArticles:   cfg.EmbedArticles,
	})

	manager.Start()
	go warmHistory(manager, db)
	go compactOnce(priceStore, cfg)
	go logMemory(db)

	server := api.New(cfg, db, priceStore, marketClient, llmClient, indexer, chatService, manager, webui.FS())
	router := server.Router()
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		<-stop
		log.Println("shutting down")
		if err := priceStore.Close(); err != nil {
			log.Printf("price store close: %v", err)
		}
		if err := db.Close(); err != nil {
			log.Printf("database close: %v", err)
		}
		os.Exit(0)
	}()

	for _, port := range cfg.ExtraHTTPPorts {
		go serveExtraPort(router, port)
	}

	log.Printf("serving on %s (session model %s, summary model %s, cheap model %s)",
		cfg.HTTPAddr, cfg.SessionModel, cfg.ChatModel, cfg.CheapModel)
	if err := router.Start(cfg.HTTPAddr); err != nil {
		log.Fatal(err)
	}
}

// serveExtraPort exposes the same router on an additional port, which keeps the old
// frontend URL working now that a single process serves the API and the UI.
func serveExtraPort(router *echo.Echo, port string) {
	server := &http.Server{Addr: ":" + port, Handler: router, ReadHeaderTimeout: 15 * time.Second}
	log.Printf("serving on :%s (extra listener)", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("extra listener on :%s stopped: %v", port, err)
	}
}

// compactOnce folds intraday bars older than the retention window into coarser ones.
// A database written by the previous version holds 30 days of 5 minute bars, so the
// first start reclaims that space without waiting a day for the periodic job.
func compactOnce(priceStore *prices.Store, cfg config.Config) {
	time.Sleep(30 * time.Second)
	started := time.Now()
	if err := priceStore.Compact(cfg.PriceRetention5mDays, cfg.PriceRetention1hDays); err != nil {
		log.Printf("warm: compaction failed: %v", err)
		return
	}
	log.Printf("warm: compaction finished in %s", time.Since(started).Round(time.Second))
}

// warmHistory fills the store for tickers that have no history yet. It runs after the
// server is listening so startup stays fast on a cold database.
func warmHistory(manager *jobs.Manager, db *store.DB) {
	time.Sleep(5 * time.Second)
	tickers, err := db.UniqueTickers()
	if err != nil {
		return
	}
	for _, ticker := range tickers {
		if err := manager.EnsureHistory(ticker); err != nil {
			log.Printf("warm: history %s: %v", ticker, err)
			continue
		}
		if err := manager.FetchPrices(ticker); err != nil {
			log.Printf("warm: prices %s: %v", ticker, err)
		}
		time.Sleep(2 * time.Second)
	}
	log.Println("warm: price history check finished")
}

func applyMemoryLimit() {
	raw := strings.TrimSpace(os.Getenv("GOMEMLIMIT"))
	if raw == "" {
		return
	}
	multipliers := []struct {
		suffix string
		factor float64
	}{{"gib", 1 << 30}, {"gb", 1e9}, {"mib", 1 << 20}, {"mb", 1e6}, {"kib", 1 << 10}}
	lower := strings.ToLower(raw)
	for _, entry := range multipliers {
		if !strings.HasSuffix(lower, entry.suffix) {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(lower, entry.suffix)), 64)
		if err != nil || value <= 0 {
			return
		}
		debug.SetMemoryLimit(int64(value * entry.factor))
		log.Printf("go heap limit set to %s", raw)
		return
	}
	if value, err := strconv.ParseInt(lower, 10, 64); err == nil && value > 0 {
		debug.SetMemoryLimit(value)
	}
}

func logMemory(db *store.DB) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		sqlite := db.Stats()
		log.Printf("mem: heap=%dMB sys=%dMB goroutines=%d gc=%d sqlite_open=%d sqlite_wait=%d",
			mem.HeapAlloc/1024/1024, mem.Sys/1024/1024, runtime.NumGoroutine(), mem.NumGC,
			sqlite.OpenConnections, sqlite.WaitCount)
	}
}
