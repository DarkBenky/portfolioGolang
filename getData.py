import yfinance as yf
import flask
from functools import lru_cache
from getAssets import get_etf_data
import asyncio
from concurrent.futures import ThreadPoolExecutor
from bs4 import BeautifulSoup
import re
from getNews import creteSentimentAnalyzer, getSentiment, getNews
from getPrice import getPrice

app = flask.Flask(__name__)

model = creteSentimentAnalyzer()

executor = ThreadPoolExecutor(max_workers=10)

@lru_cache(maxsize=256)
def get_etf_ter_and_policy(ticker, isin):
    """
    Get TER and distribution policy from multiple sources.
    Returns tuple: (ter, distribution_policy)
    """
    ter = None
    dist_policy = None
    
    # Try yfinance first
    try:
        ticker_obj = yf.Ticker(ticker)
        info = ticker_obj.info
        
        # Get TER
        ter_value = info.get('expenseRatio') or info.get('annualReportExpenseRatio') or info.get('fundOperatingExpense')
        if ter_value:
            if ter_value < 1:
                ter = f"{ter_value * 100:.2f}%"
            else:
                ter = f"{ter_value:.2f}%"
        
        # Get distribution policy
        dividend_type = info.get('dividendType', '').lower()
        if 'accumulat' in dividend_type:
            dist_policy = 'Accumulating'
        elif 'distribut' in dividend_type or 'dividend' in dividend_type:
            dist_policy = 'Distributing'
    except Exception as e:
        print(f"Warning: yfinance ETF info lookup failed for {ticker}: {e}")
    
    # If still missing data and we have ISIN, try JustETF
    if (not ter or not dist_policy) and isin and isin != 'N/A':
        try:
            import requests
            
            url = f"https://www.justetf.com/en/etf-profile.html?isin={isin}"
            headers = {'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'}
            
            response = requests.get(url, headers=headers, timeout=5)
            if response.status_code == 200:
                soup = BeautifulSoup(response.content, 'html.parser')
                text = soup.get_text().lower()
                
                # Get TER if not found yet
                if not ter:
                    ter_match = soup.find(string=lambda t: t and 'total expense ratio' in t.lower())
                    if ter_match:
                        parent = ter_match.find_parent()
                        if parent:
                            ter_text = parent.get_text()
                            ter_pattern = re.search(r'(\d+\.?\d*)\s*%', ter_text)
                            if ter_pattern:
                                ter = f"{ter_pattern.group(1)}%"
                
                # Get distribution policy if not found yet
                if not dist_policy:
                    if 'distributing' in text and 'distribution policy' in text:
                        dist_policy = 'Distributing'
                    elif 'accumulating' in text or 'reinvesting' in text:
                        dist_policy = 'Accumulating'
        except Exception as e:
            print(f"Warning: JustETF lookup failed for {isin}: {e}")
    
    # If still missing data, try ETFDB
    if not ter or not dist_policy:
        try:
            import requests
            
            # Clean ticker for ETFDB (remove exchange suffix)
            clean_ticker = ticker.split('.')[0]
            url = f"https://etfdb.com/etf/{clean_ticker}/"
            headers = {'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'}
            
            response = requests.get(url, headers=headers, timeout=5)
            if response.status_code == 200:
                soup = BeautifulSoup(response.content, 'html.parser')
                text = soup.get_text().lower()
                
                # Get TER if not found yet
                if not ter:
                    expense_match = soup.find(string=lambda t: t and 'expense ratio' in t.lower())
                    if expense_match:
                        parent = expense_match.find_parent()
                        if parent:
                            ter_pattern = re.search(r'(\d+\.?\d*)\s*%', parent.get_text())
                            if ter_pattern:
                                ter = f"{ter_pattern.group(1)}%"
                
                # Get distribution policy if not found yet
                if not dist_policy:
                    if 'dividend' in text and ('distributing' in text or 'paying' in text):
                        dist_policy = 'Distributing'
                    elif 'accumulating' in text or 'reinvesting' in text:
                        dist_policy = 'Accumulating'
        except Exception as e:
            print(f"Warning: ETFDB lookup failed for {clean_ticker}: {e}")
    
    return ter, dist_policy

@lru_cache(maxsize=256)
def search_ticker_info(identifier, search_type="ticker"):
    try:
        if search_type == "ticker":
            exchanges = ["", ".L", ".TO", ".SW", ".PA", ".DE", ".HK", ".AX", ".T"]
            results = []
            
            for exchange_suffix in exchanges:
                ticker_symbol = identifier.upper() + exchange_suffix
                try:
                    ticker_obj = yf.Ticker(ticker_symbol)
                    info = ticker_obj.info
                    
                    if info and info.get('symbol'):
                        quote_type = info.get('quoteType', 'Unknown')
                        is_etf = quote_type == 'ETF'
                        isin = info.get('isin', 'N/A')
                        
                        ter = None
                        dist_policy = None
                        
                        if is_etf:
                            ter, dist_policy = get_etf_ter_and_policy(ticker_symbol, isin)
                        
                        results.append({
                            'ticker': ticker_symbol,
                            'name': info.get('shortName', info.get('longName', 'Unknown')),
                            'exchange': info.get('exchange', 'Unknown'),
                            'isin': isin,
                            'currency': info.get('currency', 'USD'),
                            'type': quote_type,
                            'price': info.get('regularMarketPrice', 'N/A'),
                            'ter': ter,
                            'distribution_policy': dist_policy
                        })
                except:
                    continue
            
            return results
        else:
            try:
                ticker_obj = yf.Ticker(identifier)
                info = ticker_obj.info
                
                if info and info.get('symbol'):
                    quote_type = info.get('quoteType', 'Unknown')
                    is_etf = quote_type == 'ETF'
                    ticker_symbol = info.get('symbol', identifier)
                    
                    ter = None
                    dist_policy = None
                    
                    if is_etf:
                        ter, dist_policy = get_etf_ter_and_policy(ticker_symbol, identifier)
                    
                    return [{
                        'ticker': ticker_symbol,
                        'name': info.get('shortName', info.get('longName', 'Unknown')),
                        'exchange': info.get('exchange', 'Unknown'),
                        'isin': identifier,
                        'currency': info.get('currency', 'USD'),
                        'type': quote_type,
                        'price': info.get('regularMarketPrice', 'N/A'),
                        'ter': ter,
                        'distribution_policy': dist_policy
                    }]
            except:
                pass
            
            return []
    except Exception as e:
        print(f"Error searching for {identifier}: {e}")
        return []

# curl "http://localhost:5123/api/search?identifier=AAPL"
# curl "http://localhost:5123/api/search?identifier=US0378331005&search_type=isin"
@app.route('/api/search', methods=['GET'])
async def api_search():
    """Async route with thread pool execution"""
    identifier = flask.request.args.get('identifier', '')
    search_type = flask.request.args.get('search_type', 'ticker')
    
    if not identifier:
        return flask.jsonify({'error': 'Identifier parameter is required.'}), 400
    
    # Run blocking function in thread pool for true concurrency
    loop = asyncio.get_event_loop()
    results = await loop.run_in_executor(executor, search_ticker_info, identifier, search_type)
    return flask.jsonify(results)

# curl "http://127.0.0.1:5123/api/etf_data?ticker=VWCE.DE&isin=IE00BK5BQT80&etf_name=Vanguard%20FTSE%20All-World"
@app.route('/api/etf_data', methods=['GET'])
async def api_etf_data():
    """Async route for ETF data with thread pool execution"""
    ticker = flask.request.args.get('ticker', '')
    isin = flask.request.args.get('isin', '')
    etf_name = flask.request.args.get('etf_name', '')
    
    # Run blocking function in thread pool for true concurrency
    loop = asyncio.get_event_loop()
    data = await loop.run_in_executor(executor, get_etf_data, ticker, isin, etf_name)
    return flask.jsonify(data.to_dict())

# curl "http://127.0.0.1:5123/api/fetch_news?ticker=AAPL&num_articles=5"
@app.route('/api/fetch_news', methods=['GET'])
async def fetch_news():
    """Async route for fetching news articles with sentiment analysis"""
    ticker = flask.request.args.get('ticker', '')
    num_articles = int(flask.request.args.get('num_articles', '3'))
    
    # Run blocking function in thread pool for true concurrency
    loop = asyncio.get_event_loop()
    data = await loop.run_in_executor(executor, getNews, ticker, num_articles, model)
    return flask.jsonify(data)

# curl "http://localhost:5123/api/get_price?ticker=AAPL&last_updates_unix_timestamp=1700000000&interval=1m"
@app.route('/api/get_price', methods=['GET'])
async def api_get_price():
    ticker = flask.request.args.get('ticker', '')
    last_updates_unix_timestamp = int(flask.request.args.get('last_updates_unix_timestamp', '0'))
    interval = flask.request.args.get('interval', '1m')
    loop = asyncio.get_event_loop()
    data = await loop.run_in_executor(executor, getPrice, ticker, last_updates_unix_timestamp, interval)
    return flask.jsonify(data)

if __name__ == '__main__':
    # For production, use: gunicorn -w 1 --threads 10 -b 0.0.0.0:5123 app:app
    app.run(debug=True, host='0.0.0.0', port=5123, threaded=True)