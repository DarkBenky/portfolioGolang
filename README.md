# Portfolio Tracker

One Go binary serves the JSON API and the Vue single page app. There is no Python
service and no separate frontend server; `make build` produces `./portfolio`.

## Layout

```
cmd/server            entry point: config, stores, jobs, HTTP server
internal/config       environment parsing and tunables
internal/store        SQLite schema and queries (users, holdings, news,
                      sentiment, chat, RAG, pins, situations, LLM accounting)
internal/bills        expenses, bank transactions, merchant rules, reports
internal/prices       DuckDB price store, candle buckets, compaction, WAL guard
internal/market       Yahoo Finance, JustETF, OpenFIGI, web search, articles,
                      sentiment scoring
internal/analytics    pure risk and backtest math
internal/llm          OpenRouter client, response cache, spend budget, prompts
internal/rag          BM25 retrieval over stored news and reports
internal/chat         chat harness: local tools first, web search last
internal/jobs         periodic pipelines and report generation
internal/api          Echo router and handlers grouped by domain
internal/webui        embedded frontend bundle
frontend              Vue 3 + Vuetify source (builds into internal/webui/assets)
```

Data flow: handlers read SQLite for entities, DuckDB for bars, and never call an LLM
inline except the chat stream. Background jobs own ingestion and generation.

## Build and run

```
make web      # build the UI into internal/webui/assets
make build    # build ./portfolio with the UI embedded
make test     # unit and integration tests
make bench    # benchmarks
make run
```

`.env` in the working directory is required (`SALT`, `JWT_SECRET`,
`OPENROUTER_API_KEY`). See `CONFIG.md`.

## Data sources (all Go)

- Prices, quotes, news and fundamentals: Yahoo Finance HTTP endpoints, including the
  cookie and crumb handshake that `quoteSummary` requires.
- ETF composition, TER and distribution policy: the public JustETF profile page.
- Ticker to ISIN mapping: OpenFIGI.
- Web search: SearXNG when configured, then Bing RSS, Yahoo news and Wikipedia.
- Article text: paragraph extraction with scripts, navigation and forms removed.
- News sentiment: a weighted finance lexicon, so ingestion has no model cost.

## LLM cost model

- Daily and portfolio summaries use `OPENROUTER_CHEAP_MODEL`; chat uses
  `OPENROUTER_MODEL`. `reasoning` is disabled for summaries.
- Every non chat completion is cached in `llm_cache` under a content hash, so the same
  news set is never billed twice. Chat is the only streaming path.
- Retrieval is BM25 over text already stored in SQLite. Embeddings are opt in with
  `RAG_USE_EMBEDDINGS=1` and are only needed for semantic search. `RAG_EMBED_ARTICLES=1`
  stores vectors for new chunks without loading them at query time. Articles and their
  embeddings are never pruned.
- Chat offers local tools (`get_portfolio`, `get_price_history`, `get_news`,
  `get_spending`, `search_documents`) before `web_search`; searches per turn and per
  situation run are capped.
- Every call is recorded in `llm_usage` with tokens and cost. `GET /api/usage` shows
  today's spend per kind and model. `LLM_DAILY_BUDGET_USD` stops work once a user
  reaches the limit.
- Situation reports and summaries are guarded by cooldowns and content signatures, so
  repeat requests and untouched days are free.

## Storage and chart resolution

- Bars live in `prices.duckdb`. Intraday bars are kept for 14 days, hourly for 120
  days, older data is folded into daily bars. Tune with `PRICE_RETENTION_5M_DAYS` and
  `PRICE_RETENTION_1H_DAYS`.
- Chart windows are short by design (`PRICE_LOOKBACK_5M_DAYS`, default 14 days) and the
  server downsamples any series to `CHART_MAX_POINTS` (default 500) before sending it,
  which cuts payload size and disk pressure.
- Portfolio sentiment is merged into the portfolio candle array as a `sentiment` field
  on the same timestamps, so the chart never reserves extra time slots for a second
  series.
- A corrupt DuckDB write ahead log is replayed in a child process before startup; if
  the replay dies the log is quarantined and the server starts from the last
  checkpoint.

## Tests

`internal/*_test.go` covers the pure math, candle downsampling, sentiment scoring,
JustETF parsing and BM25 ranking, plus an API integration test that runs the real
router against temporary SQLite and DuckDB files with background fetching disabled.
Benchmarks cover backtesting (10 years, 8 holdings), candle downsampling, BM25 search
over 2000 chunks, sentiment scoring and a 20 holding holdings request.
