import yfinance as yf
from datetime import datetime

def getPrice(Ticker: str, lastUpdatesUnixTimeStamp: int, interval: str = "1m"):
    ticker = yf.Ticker(Ticker)
    start_date = datetime.fromtimestamp(lastUpdatesUnixTimeStamp)
    data = ticker.history(start=start_date, interval=interval)
    
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