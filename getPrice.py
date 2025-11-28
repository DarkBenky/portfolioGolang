import yfinance as yf
from datetime import datetime, timedelta, timezone

def getPrice(Ticker: str, lastUpdatesUnixTimeStamp: int, interval: str = "1m"):
    now = datetime.now(timezone.utc)
    Ticker  = Ticker.strip('$')
    
    # Convert milliseconds to seconds if necessary
    if lastUpdatesUnixTimeStamp > 10**10:
        lastUpdatesUnixTimeStamp = lastUpdatesUnixTimeStamp // 1000
    
    four_hours_ago = now - timedelta(hours=4)
    
    try:
        timestamp_dt = datetime.fromtimestamp(lastUpdatesUnixTimeStamp, tz=timezone.utc)
    except (ValueError, OSError):
        timestamp_dt = four_hours_ago
    
    # Ensure timestamp is valid (not in future, not too old)
    if timestamp_dt > now or timestamp_dt < four_hours_ago:
        timestamp_dt = four_hours_ago
    
    try:
        ticker = yf.Ticker(Ticker)
        data = ticker.history(start=timestamp_dt, end=now, interval=interval)
    except Exception as e:
        print(f"Error fetching data for {Ticker}: {e}")
        return []
    
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