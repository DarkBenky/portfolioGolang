# Configuration

All values are read from `.env` in the working directory.

## Required

```
SALT=...                    # password pepper
JWT_SECRET=...              # token signing key
OPENROUTER_API_KEY=...      # LLM access; leave empty to disable generation
```

## Server

```
DEV_MODE=false              # true allows the Vite dev server origin and skips chat cooldown
BACKEND_GO_PORT=8085        # HTTP port; serves both the API and the UI
SERVER_HOST=...             # host used to build allowed origins in production
JWT_EXPIRY_HOURS=168
GOMEMLIMIT=1200MiB          # optional soft heap limit
DATA_DIR=.                  # optional working data directory
SQLITE_PATH=./portfolio.db
PRICE_DB_PATH=./prices.duckdb
```

## Models and cost

```
OPENROUTER_MODEL=qwen/qwen3.7-flash          # chat
OPENROUTER_CHEAP_MODEL=inclusionai/ling-3.0-flash  # daily summaries and classification
OPENROUTER_EMBEDDING_MODEL=openai/text-embedding-3-small
LLM_DAILY_BUDGET_USD=0      # 0 disables the guard; otherwise stops work per user per day
RAG_USE_EMBEDDINGS=0        # 1 adds vector search on top of BM25
```

## Prices and charts

```
PRICE_RETENTION_5M_DAYS=14   # intraday bars kept at 5 minute resolution
PRICE_RETENTION_1H_DAYS=120  # hourly bars kept before folding into daily
PRICE_LOOKBACK_5M_DAYS=14    # chart window for 5m and 15m intervals
CHART_MAX_POINTS=500         # server side downsampling limit
PRICE_MIGRATION_MEMORY=2GB   # DuckDB memory during the one time import
PRICE_MEMORY_LIMIT=512MB     # DuckDB steady state memory
```

## Search

```
SEARXNG_URL=                 # optional, tried before the built in backends
```

## Frontend development

`frontend/.env` with `VITE_DEV_MODE=true` makes the Vite dev server call
`http://localhost:VITE_BACKEND_PORT`. Production builds use relative URLs because the
Go server serves the bundle from the same origin.

## Deployment notes

Run the binary under a supervisor that restarts on failure, for example a systemd unit
with `Restart=always`, `RequiresMountsFor` for the directory holding the databases and
`EnvironmentFile=/path/to/.env`. The process checkpoints DuckDB on SIGTERM; a hard
crash is covered by the write ahead log probe described in `README.md`.
