# TODO

- [x] **Important** check why python and go is eating so much ram on server
      Python: the JustETF page cache was keyed on the full HTML string (getAssets.py) and the gunicorn worker never recycled or returned memory. Go: unlimited SQLite connections with a 64 MB page cache and 256 MB mmap each, unbounded periodic fan-outs and a RAG reindex that reloaded every news/sentiment/search row every 30 minutes. See CONFIG.md "Memory" for the settings and /api/debug/mem on both servers for measurement.
- [x] use duck db for price data for faster operations ( maybe migrate all )
      Prices moved to prices.duckdb (prices only, everything else stays in SQLite). Candles and backtests are aggregated in SQL with time_bucket/date_trunc, the O(n^2) Go sorts are gone and the backtest no longer downloads yfinance history for every holding.
- [x] fix charts when changing candle interval to w or m it makes spaces between candles
      The sentiment pane was always a daily grid while 1w/1M candles were bucketed to 7/30 days, so lightweight-charts reserved the daily slots and left gaps. Sentiment is now bucketed on the same interval grid, weeks start on Monday, months on the 1st, and the chart fits its content when the interval changes.
- [x] enhance expenses so bolt uber ... => transport bold food, wolt => dining and enhance it even more
      Merchant matching is now longest-keyword-first with word boundaries (so "bolt food" wins over "bolt"), the seed list covers food delivery, drugstores, fuel, telecom, streaming and brokers, the amount based guessing is gone and the category list comes from the backend.
- [x] check why the the daily topic summary is not working sometimes -> often we get no specific news reports were found
      The web search returned an empty list when DuckDuckGo rate limited the server. Searches now rotate DDGS backends, fall back to SearXNG (set SEARXNG_URL) and Wikipedia, report which provider failed, are cached for reuse, and a report with zero results is retried later the same day (up to 3 attempts) instead of being stored as "no developments found".
- [x] make statistics tab faster and also maybe other add other interesting indicator
      The backtest reads daily bars from DuckDB with a 15 minute cache (31 ms for a 578 point backtest) and the tab now shows alpha, beta, win rate and the best/worst month.
- [x] on trading view add side bar with item existing in portfolio so we can easily check them and when we open something that is not in holding add option to pin it to this side bar
      Trading view has a watchlist card with holdings and pinned symbols, pins are stored per user through /api/pins.
- [x] replace the deeps seek with  https://openrouter.ai/qwen/qwen3.7-flash

## Follow ups

- [ ] Legacy hourly volume was stored divided by 60 (getPrice.py). New rows are correct; older hourly rows are still inconsistent. Re-run the historic backfill for a ticker to fix its history.
- [ ] gunicorn recycles the worker every ~800 requests, which costs one FinBERT reload. If that gap is too visible, raise --max-requests or run two workers.
- [ ] The SQLite prices table is left in portfolio.db as a backup. Drop it once the DuckDB store has proven stable for a release.
- [ ] Optional: systemd units with MemoryMax and Restart=always for ./main and start_python_server.sh.

