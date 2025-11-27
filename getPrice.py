import yfinance as yf
from datetime import datetime, timedelta

def getPrice(Ticker: str, lastUpdatesUnixTimeStamp: int, interval: str = "1m"):
    now = datetime.now()
    four_hours_ago = now - timedelta(hours=4)
    timestamp_dt = datetime.fromtimestamp(lastUpdatesUnixTimeStamp)
    
    if timestamp_dt < four_hours_ago:
        timestamp_dt = four_hours_ago

    ticker = yf.Ticker(Ticker)
    data = ticker.history(start=timestamp_dt, interval=interval)
    
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
