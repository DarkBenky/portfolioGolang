# Environment Configuration

This project uses environment variables to switch between development and production modes.

## Configuration Files

### Backend (.env in root directory)
```
DEV_MODE=true

BACKEND_GO_PORT=8085
BACKEND_PYTHON_PORT=5123
SERVER_HOST=91.98.145.193

SALT="your-salt-here"
JWT_SECRET="your-jwt-secret-here"

PYTHON_API_KEY="shared-secret-between-go-and-python"

# Optional: SearXNG instance used as a web search fallback (leave empty to skip)
SEARXNG_URL=""

# Optional: soft heap limit for the Go server, for example 1200MiB
GOMEMLIMIT=1200MiB

# Optional: DuckDB price store memory limits (defaults shown below)
PRICE_MIGRATION_MEMORY=2GB
PRICE_MEMORY_LIMIT=512MB
```

### Frontend (.env in frontend directory)
```
VITE_DEV_MODE=true
VITE_SERVER_HOST=91.98.145.193
VITE_BACKEND_PORT=8085
```

## Modes

### Development Mode (DEV_MODE=true)
- Backend connects to Python API at `http://127.0.0.1:5123/api`
- Frontend connects to Go backend at `http://localhost:8085`
- CORS allows `http://localhost:5173`
- All services run on localhost

### Production Mode (DEV_MODE=false)
- Backend connects to Python API at `http://127.0.0.1:5123/api` (Python is localhost-only)
- Frontend connects to Go backend at `http://{SERVER_HOST}:8085`
- CORS allows `http://{SERVER_HOST}:5173`

## Security

- The Python API binds to `127.0.0.1` only (see `start_python_server.sh`, `getData.py`). It is NOT reachable from outside the machine; only the Go server (same host) can call it.
- All Python `/api/*` routes (except `/api/health`) require the `X-API-Key` header matching `PYTHON_API_KEY` in `.env`. The Go server sends this header automatically (all internal Python calls go through `pythonHTTPClient`).
- The frontend never calls Python directly. Features that previously did (`convert_currency`, `search`, `get_price`) now go through Go proxy endpoints (`/api/convert_currency`, `/api/search`, `/api/get_price`) which require a valid JWT.
- CORS on Python is restricted to `http://localhost:5173` / `http://127.0.0.1:5173`.

## Switching Between Modes

### To run locally (Development):
1. Set `DEV_MODE=true` in root `.env`
2. Set `VITE_DEV_MODE=true` in `frontend/.env`
3. Access frontend at `http://localhost:5173`

### To run on remote server (Production):
1. Set `DEV_MODE=false` in root `.env`
2. Set `VITE_DEV_MODE=false` in `frontend/.env`
3. Update `SERVER_HOST` to your server IP
4. Access frontend at `http://{SERVER_HOST}:5173`

## Starting Services

### Backend (Go)
```bash
./main
```

### Backend (Python)
```bash
./start_python_server.sh
```
or manually:
```bash
python getData.py
```

## Memory

Both processes used to grow until a restart. The fixes and the settings that matter:

- Go: the SQLite pool is capped (`SetMaxOpenConns(4)`, 30 min connection lifetime), the page cache is 16 MB and the mmap window is 64 MB per connection. Periodic jobs run through a semaphore of 3 concurrent external fetches. RAG reindexing is skipped when no source changed and report triggered reindexes are debounced to once per 5 minutes. Set `GOMEMLIMIT=1200MiB` in the service environment to make the Go garbage collector keep the heap bounded.
- Python: `ttl_cache` keys are hashed instead of retaining page markup, the big asset caches are capped at 64 to 512 entries, article downloads are capped at 2 MB, the thread pool runs 4 workers and gunicorn recycles its worker (`--max-requests 800 --max-requests-jitter 200`) with glibc trim settings so freed memory returns to the OS.
- Python request handling: interactive endpoints (`/get_price`, `/search`, `/stock/<isin>`, `/convert_currency`) run on their own 6 worker pool, while heavy background work (`/fetch_news`, `/etf_data`, `/stock/history/...`, summaries) runs on a separate 3 worker pool. A long backfill or a slow scrape therefore cannot starve the UI any more.
- Trading view charts are served from the price store whenever the symbol is already tracked (milliseconds, no Python round trip) and fall back to the live data source for untracked symbols and for the 1m interval, which is not stored.
- Where the data lives: price bars are in `prices.duckdb` (DuckDB, single writer = the Go server, memory limit raised to `PRICE_MIGRATION_MEMORY` during the one time migration and lowered to `PRICE_MEMORY_LIMIT` afterwards, 2 GB and 512 MB by default). Everything else stays in `portfolio.db`. The old `prices` table in SQLite is kept read only as a backup and is only read once, by the migration.

### First start after the price store change
- On startup, if the DuckDB table is empty and the SQLite `prices` table has rows, every bar is copied over (about a second per 100k rows) and logged as `prices: migrating N rows` / `Price store ready`.
- Back up `portfolio.db` **and** `prices.duckdb` before the first start: once the migration has run, new bars only go to DuckDB, so the SQLite copy is a point in time snapshot and must not be treated as the source of truth afterwards. Re-running the migration is only possible from that snapshot, so deleting `prices.duckdb` loses everything collected after the switch.
- Symbols that have no stored bars (benchmarks such as SPY, or a holding added a moment ago) are fetched on demand from the Python data source, stored and then reused; the log shows `prices: backfilled N bars for <ticker>`. Only the first backtest for such a symbol waits for that fetch.
- The backfill asks `/stock/history/<ticker>` for the last 730 days of hourly bars (the most yfinance serves at that interval) and falls back to the live `/get_price` endpoint when that route returns nothing, so a broken or stale `/stock/history` no longer leaves benchmarks without data. Both the 404 and the error path log the response body.
- The Go server runs `CHECKPOINT` and closes both databases when it receives SIGINT, SIGTERM or SIGHUP, so a normal stop leaves no unapplied WAL behind. An abrupt kill (SIGKILL, OOM, power loss) can leave a WAL that DuckDB refuses to replay with `Failure while replaying WAL file ... Invalid bitmask for FixedSizeAllocator` (duckdb/duckdb#13306): back up `prices.duckdb`, move `prices.duckdb.wal` aside, and the store reopens at the last checkpoint.
- Price writes commit at most 5000 rows per transaction with 500 row statements per insert, so a big backfill stays inside `PRICE_MEMORY_LIMIT`. Thousands of single row statements in one transaction exhausted the 512 MB limit, aborted the transaction and dropped every row while the log still reported `Inserted N historic prices`.

### Measuring
- Go: `curl -H "Authorization: Bearer <jwt>" http://127.0.0.1:8085/api/debug/mem` and the `mem:` log line every 5 minutes.
- Python: `curl -H "X-API-Key: $PYTHON_API_KEY" http://127.0.0.1:5123/api/debug/mem`.
- Both: `ps -eo pid,user,%mem,rss,comm --sort=-rss | head -5` sampled over a day.

### Frontend
```bash
cd frontend
npm run dev
```
