# Integration Guide for PriceDB

This document provides the wrapper functions to use in `main.go` to integrate the new in-memory PriceDB.

## Setup

Add this global variable in main.go:

```go
var (
    // ... existing variables
    priceDB *inmem.PriceDB
)
```

## Initialization

Replace the database initialization in `main()` or `initDB()`:

```go
func initPriceDB() {
    snapshotPeriod := 5 * time.Minute
    snapShotPath := "./price_snapshot.gob"
    
    loadFromSnapshot := true
    priceDB = inmem.NewPriceDB(snapshotPeriod, snapShotPath, loadFromSnapshot)
    
    log.Println("PriceDB initialized successfully")
}
```

Or load from existing SQL database:

```go
func initPriceDBFromSQL(sqlDBPath string) error {
    snapshotPeriod := 5 * time.Minute
    snapShotPath := "./price_snapshot.gob"
    
    var err error
    priceDB, err = inmem.NewPriceDBFromSQL(snapshotPeriod, snapShotPath, sqlDBPath)
    if err != nil {
        return fmt.Errorf("failed to initialize PriceDB from SQL: %v", err)
    }
    
    log.Println("PriceDB initialized from SQL successfully")
    return nil
}
```

## Wrapper Functions

### Add Prices

Replace `db.addPrices()`:

```go
func addPricesToMemory(prices []Price) error {
    memPrices := make([]inmem.Price, len(prices))
    for i, p := range prices {
        timestamp, err := strconv.ParseInt(p.Date, 10, 64)
        if err != nil {
            log.Printf("Invalid timestamp for price: %v", err)
            continue
        }
        
        memPrices[i] = inmem.Price{
            Ticker: p.Ticker,
            Date:   uint32(timestamp),
            Open:   float32(p.Open),
            Close:  float32(p.Close),
            High:   float32(p.High),
            Low:    float32(p.Low),
            Volume: p.Volume,
        }
    }
    
    priceDB.AddPrices(memPrices)
    return nil
}
```

### Get Last Price Timestamp

Replace `db.getLastPriceTimestamp()`:

```go
func getLastPriceTimestamp(ticker string) (int64, error) {
    timestamp, err := priceDB.GetLastPriceTimestamp(ticker)
    if err != nil {
        return 0, err
    }
    return int64(timestamp), nil
}
```

### Get Latest Price

For portfolio value calculations:

```go
func getLatestPriceForTicker(ticker string) (float64, error) {
    price, found := priceDB.GetLatestPrice(ticker)
    if !found {
        return 0, fmt.Errorf("no price found for ticker %s", ticker)
    }
    return float64(price.Close), nil
}
```

### Get Prices by Range

For chart data and history:

```go
func getPricesInRange(ticker string, startTimestamp, endTimestamp int64) ([]Price, error) {
    memPrices := priceDB.GetPricesByTickerRange(ticker, uint32(startTimestamp), uint32(endTimestamp))
    
    prices := make([]Price, len(memPrices))
    for i, mp := range memPrices {
        prices[i] = Price{
            Ticker: mp.Ticker,
            Date:   fmt.Sprintf("%d", mp.Date),
            Open:   float64(mp.Open),
            Close:  float64(mp.Close),
            High:   float64(mp.High),
            Low:    float64(mp.Low),
            Volume: mp.Volume,
        }
    }
    
    return prices, nil
}
```

### Get All Prices for Ticker

```go
func getPricesByTicker(ticker string) ([]Price, error) {
    memPrices := priceDB.GetPricesByTicker(ticker)
    
    prices := make([]Price, len(memPrices))
    for i, mp := range memPrices {
        prices[i] = Price{
            Ticker: mp.Ticker,
            Date:   fmt.Sprintf("%d", mp.Date),
            Open:   float64(mp.Open),
            Close:  float64(mp.Close),
            High:   float64(mp.High),
            Low:    float64(mp.Low),
            Volume: mp.Volume,
        }
    }
    
    return prices, nil
}
```

## Update Existing Functions

### Update fetchPrices

```go
func fetchPrices(ticker string) error {
    lastTimestamp, err := getLastPriceTimestamp(ticker)
    if err != nil {
        log.Printf("Error getting last price timestamp for %s: %v", ticker, err)
        return err
    }

    now := time.Now().UTC().Unix()

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
        log.Printf("API returned non-JSON response for %s", ticker)
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
        log.Printf("Error decoding price data for %s: %v", ticker, err)
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
        err = addPricesToMemory(validPrices)
        if err != nil {
            log.Printf("Error adding prices to memory for %s: %v", ticker, err)
            return err
        }
    }

    log.Printf("Successfully processed %d valid price candles for %s", len(validPrices), ticker)
    return nil
}
```

### Update GetPortfolioValue

```go
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

    tickerQuantities := make(map[string]float64)
    tickerPurchasePrice := make(map[string]float64)

    for _, holding := range holdings {
        tickerQuantities[holding.Ticker] += holding.Quantity
        tickerPurchasePrice[holding.Ticker] = holding.PurchasePrice
    }

    totalValue := 0.0
    for ticker, quantity := range tickerQuantities {
        price, found := priceDB.GetLatestPrice(ticker)
        var closePrice float64
        if found {
            closePrice = float64(price.Close)
        } else {
            closePrice = tickerPurchasePrice[ticker]
        }
        totalValue += closePrice * quantity
    }

    return c.JSON(http.StatusOK, map[string]float64{
        "total_value": totalValue,
    })
}
```

### Update GetAssetPriceHistory

```go
func GetAssetPriceHistory(c echo.Context) error {
    ticker := c.QueryParam("ticker")
    if ticker == "" {
        return c.String(http.StatusBadRequest, "Ticker is required")
    }

    period := c.QueryParam("period")
    candleInterval := c.QueryParam("candle_interval")

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
        startTime = now.Add(-7 * 24 * time.Hour)
    }

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
        intervalSeconds = 300
    }

    startTimestamp := uint32(startTime.Unix())
    endTimestamp := uint32(now.Unix())

    memPrices := priceDB.GetPricesByTickerRange(ticker, startTimestamp, endTimestamp)

    type PriceData struct {
        Open   float64
        High   float64
        Low    float64
        Close  float64
        Volume int64
    }

    bucketData := make(map[int64]*PriceData)

    for _, price := range memPrices {
        timestamp := int64(price.Date)
        bucket := (timestamp / intervalSeconds) * intervalSeconds

        if bucketData[bucket] == nil {
            bucketData[bucket] = &PriceData{
                Open:   float64(price.Open),
                High:   float64(price.High),
                Low:    float64(price.Low),
                Close:  float64(price.Close),
                Volume: price.Volume,
            }
        } else {
            pd := bucketData[bucket]
            if float64(price.High) > pd.High {
                pd.High = float64(price.High)
            }
            if float64(price.Low) < pd.Low {
                pd.Low = float64(price.Low)
            }
            pd.Close = float64(price.Close)
            pd.Volume += price.Volume
        }
    }

    timestamps := make([]int64, 0, len(bucketData))
    for ts := range bucketData {
        timestamps = append(timestamps, ts)
    }
    sort.Slice(timestamps, func(i, j int) bool {
        return timestamps[i] < timestamps[j]
    })

    result := make([]map[string]interface{}, len(timestamps))
    for i, ts := range timestamps {
        pd := bucketData[ts]
        result[i] = map[string]interface{}{
            "timestamp": ts,
            "open":      pd.Open,
            "high":      pd.High,
            "low":       pd.Low,
            "close":     pd.Close,
            "volume":    pd.Volume,
        }
    }

    return c.JSON(http.StatusOK, result)
}
```

## Performance Benefits

The new PriceDB provides:

1. **Per-ticker mutexes** - Different tickers can be read/written concurrently
2. **Sorted arrays** - Binary search for O(log n) lookups
3. **Memory efficiency** - Columnar storage using float32 and uint32
4. **No SQL overhead** - Direct memory access
5. **Automatic snapshots** - Periodic persistence to disk

## Migration Steps

1. Keep existing SQL database for other data (users, holdings, etc.)
2. Initialize PriceDB from SQL database on first run
3. Route all price operations through PriceDB
4. Keep SQL database as backup, periodically sync from PriceDB snapshots if needed
