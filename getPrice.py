import yfinance as yf
from datetime import datetime, timedelta, timezone

def getPrice(Ticker: str, lastUpdatesUnixTimeStamp: int, interval: str = "1m"):
    now = datetime.now(timezone.utc)
    Ticker  = Ticker.strip('$')
    
    # Convert milliseconds to seconds if necessary
    if lastUpdatesUnixTimeStamp > 10**10:
        lastUpdatesUnixTimeStamp = lastUpdatesUnixTimeStamp // 1000
    
    # yfinance limits for different intervals:
    # 1m: max 7 days
    # 5m, 15m: max 60 days
    # 1h: max 730 days
    # 1d: unlimited
    interval_max_days = {
        "1m": 7,
        "5m": 60,
        "15m": 60,
        "1h": 730,
        "1d": 365 * 10,
    }
    max_days = interval_max_days.get(interval, 7)
    earliest_allowed = now - timedelta(days=max_days)
    
    try:
        timestamp_dt = datetime.fromtimestamp(lastUpdatesUnixTimeStamp, tz=timezone.utc)
    except (ValueError, OSError):
        timestamp_dt = earliest_allowed
    
    # Ensure timestamp is valid (not in future, not before max allowed)
    if timestamp_dt > now:
        timestamp_dt = earliest_allowed
    elif timestamp_dt < earliest_allowed:
        timestamp_dt = earliest_allowed
    
    # Debug logging
    print(f"[{Ticker}] Request: from {timestamp_dt} to {now} (interval: {interval})")
    print(f"[{Ticker}] Unix timestamps: {int(timestamp_dt.timestamp())} -> {int(now.timestamp())}")
    
    try:
        ticker = yf.Ticker(Ticker)
        data = ticker.history(start=timestamp_dt, end=now, interval=interval)
    except Exception as e:
        print(f"Error fetching data for {Ticker}: {e}")
        return []
    
    if data.empty:
        print(f"[{Ticker}] No data returned from yfinance (likely market closed or invalid ticker)")
    else:
        print(f"[{Ticker}] Retrieved {len(data)} candles")
    
    candles = []
    for timestamp, row in data.iterrows():
        candles.append({
            'timestamp': int(timestamp.timestamp()),
            'open': row['Open'],
            'high': row['High'],
            'low': row['Low'],
            'close': row['Close'],
            'volume': row['Volume']
        })
    
    return candles

def getPriceDataOld(Ticker: str):
    Ticker  = Ticker.strip('$')
    # load as much data as possible with interval 1h
    ticker = yf.Ticker(Ticker)
    data = ticker.history(period="max", interval="1h")
    candles = []
    for timestamp, row in data.iterrows():
        candles.append({
            'timestamp': int(timestamp.timestamp()),
            'open': row['Open'],
            'high': row['High'],
            'low': row['Low'],
            'close': row['Close'],
            'volume': row['Volume'] / 60  # convert to per minute volume
        })
    return candles

def convertCurrency(amount: float, from_currency: str, to_currency: str) -> float:
    if from_currency == to_currency:
        return amount

    pair = f"{from_currency}{to_currency}=X"
    ticker = yf.Ticker(pair)
    data = ticker.history(period="1d")

    if data.empty:
        raise ValueError(f"Exchange rate not available for {pair}")

    rate = data["Close"].iloc[-1]
    return amount * rate

if __name__ == "__main__":
    # Example usage
    Ticker = "EDFS.DE"
    print(getPriceDataOld(Ticker))
    