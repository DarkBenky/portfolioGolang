import yfinance as yf
import pprint

ticker = yf.Ticker("AAPL")
news = ticker.news  # or ticker.get_news(count=10)

for item in news:
    pprint.pprint(item)