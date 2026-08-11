import resource
import yfinance as yf
import flask
from functools import lru_cache
from getAssets import get_etf_data
from concurrent.futures import ThreadPoolExecutor
from bs4 import BeautifulSoup
import re
from getNews import creteSentimentAnalyzer, getSentiment, getNews
from getPrice import convertCurrency, getPrice, getPriceDataOld
from getSummary import summarize_daily_news, summarize_daily_portfolio_news, summarize_portfolio_from_holdings, generate_running_summary
from getStock import get_stock_data
from env import BACKEND_PYTHON, BACKEND_PYTHON_PORT, PYTHON_API_KEY
from datetime import datetime, timezone
from collections import deque
from flask_cors import CORS
import atexit

try:
    _soft, _hard = resource.getrlimit(resource.RLIMIT_NOFILE)
    resource.setrlimit(resource.RLIMIT_NOFILE, (min(65536, _hard), _hard))
except Exception:
    pass

app = flask.Flask(__name__)

CORS(app, resources={r"/api/*": {"origins": ["http://localhost:5173", "http://127.0.0.1:5173"]}})

@app.before_request
def require_api_key():
    if flask.request.path.startswith('/api') and flask.request.path != '/api/health':
        provided = flask.request.headers.get('X-API-Key', '')
        if not PYTHON_API_KEY or provided != PYTHON_API_KEY:
            return flask.jsonify({'error': 'Unauthorized'}), 401

model = creteSentimentAnalyzer()

executor = ThreadPoolExecutor(max_workers=10)

def cleanup_executor():
    executor.shutdown(wait=True, cancel_futures=False)

atexit.register(cleanup_executor)

request_logs = deque(maxlen=512)

@app.before_request
def log_request():
    """Log every request before it's processed"""
    log_entry = {
        'timestamp': datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC'),
        'method': flask.request.method,
        'path': flask.request.path,
        # 'ip': flask.request.remote_addr,
        'user_agent': flask.request.headers.get('User-Agent', 'Unknown')[:50]  # Truncate UA
    }
    request_logs.append(log_entry)

@app.route('/api/logs', methods=['GET'])
def view_logs():
    """Display recent request logs"""
    return flask.jsonify({
        'total_logs': len(request_logs),
        'logs': list(request_logs)
    })

@app.route('/api/health', methods=['GET'])
def health_check():
    """Simple health check endpoint"""
    return flask.jsonify({
        'status': 'ok',
        'timestamp': datetime.now(timezone.utc).isoformat(),
        'service': 'portfolio-python-api'
    })


@app.route('/')
def index():
    return f"OK - Backend Python running at {BACKEND_PYTHON}:{BACKEND_PYTHON_PORT}" 

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
            
            with requests.get(url, headers=headers, timeout=5) as response:
                if response.status_code == 200:
                    soup = BeautifulSoup(response.content, 'html.parser')
                    text = soup.get_text().lower()

                    if not ter:
                        ter_match = soup.find(string=lambda t: t and 'total expense ratio' in t.lower())
                        if ter_match:
                            parent = ter_match.find_parent()
                            if parent:
                                ter_text = parent.get_text()
                                ter_pattern = re.search(r'(\d+\.?\d*)\s*%', ter_text)
                                if ter_pattern:
                                    ter = f"{ter_pattern.group(1)}%"

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
            
            with requests.get(url, headers=headers, timeout=5) as response:
                if response.status_code == 200:
                    soup = BeautifulSoup(response.content, 'html.parser')
                    text = soup.get_text().lower()

                    if not ter:
                        expense_match = soup.find(string=lambda t: t and 'expense ratio' in t.lower())
                        if expense_match:
                            parent = expense_match.find_parent()
                            if parent:
                                ter_pattern = re.search(r'(\d+\.?\d*)\s*%', parent.get_text())
                                if ter_pattern:
                                    ter = f"{ter_pattern.group(1)}%"

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
                    
                    if info and info.get('symbol') and info.get('regularMarketPrice') is not None:
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
                except Exception as e:
                    print(f"Search error for {ticker_symbol}: {e}")
                    continue
            
            return results
        elif search_type == "isin":
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
        elif search_type == "name":
            try:
                search = yf.Search(identifier, max_results=10, enable_fuzzy_query=True)
                quotes = search.quotes
                results = []
                for q in quotes[:8]:
                    ticker_sym = q.get('symbol', '')
                    if not ticker_sym:
                        continue
                    quote_type = q.get('quoteType', 'Unknown')
                    results.append({
                        'ticker': ticker_sym,
                        'name': q.get('shortName', q.get('longName', 'Unknown')),
                        'exchange': q.get('exchange', 'Unknown'),
                        'isin': q.get('isin', 'N/A'),
                        'currency': q.get('currency', 'USD'),
                        'type': quote_type,
                        'price': q.get('regularMarketPrice', 'N/A'),
                        'ter': None,
                        'distribution_policy': None
                    })
                return results
            except Exception as e:
                print(f"Name search error for '{identifier}': {e}")
                return []

    except Exception as e:
        print(f"Error searching for {identifier}: {e}")
        return []

def convert_ticker_to_isin(ticker):
    """Convert ticker to ISIN using OpenFIGI API with caching"""
    if not ticker or ticker == 'N/A':
        return None
    
    try:
        import requests
        url = 'https://api.openfigi.com/v3/mapping'
        headers = {'Content-Type': 'application/json'}
        payload = [{"idType": "TICKER", "idValue": ticker}]
        
        with requests.post(url, json=payload, headers=headers, timeout=5) as response:
            if response.status_code == 200:
                data = response.json()
                if data and len(data) > 0 and 'data' in data[0]:
                    results = data[0]['data']
                    if results and len(results) > 0:
                        isin = results[0].get('isin')
                        if isin:
                            return isin
    except Exception as e:
        print(f"OpenFIGI lookup failed for {ticker}: {e}")

    return None

# curl "http://localhost:5123/api/search?identifier=AAPL"
# curl "http://localhost:5123/api/search?identifier=US0378331005&search_type=isin"
@app.route('/api/search', methods=['GET'])
def api_search():
    identifier = flask.request.args.get('identifier', '')
    search_type = flask.request.args.get('search_type', 'ticker')
    
    if not identifier:
        return flask.jsonify({'error': 'Identifier parameter is required.'}), 400
    
    results = search_ticker_info(identifier, search_type)
    return flask.jsonify(results)

# curl "http://127.0.0.1:5123/api/etf_data?ticker=VWCE.DE&isin=IE00BK5BQT80&etf_name=Vanguard%20FTSE%20All-World"
@app.route('/api/etf_data', methods=['GET'])
def api_etf_data():
    ticker = flask.request.args.get('ticker', '')
    isin = flask.request.args.get('isin', '')
    etf_name = flask.request.args.get('etf_name', '')
    
    future = executor.submit(get_etf_data, ticker, isin, etf_name)
    data = future.result()
    return flask.jsonify(data.to_dict())

# curl "http://127.0.0.1:5123/api/fetch_news?ticker=AAPL&num_articles=5"
@app.route('/api/fetch_news', methods=['GET'])
def fetch_news():
    ticker = flask.request.args.get('ticker', '')
    num_articles = int(flask.request.args.get('num_articles', '3'))
    
    future = executor.submit(getNews, ticker, num_articles, model)
    data = future.result()
    return flask.jsonify(data)

# curl "http://localhost:5123/api/get_price?ticker=AAPL&last_updates_unix_timestamp=1700000000&interval=1m"
@app.route('/api/get_price', methods=['GET'])
def api_get_price():
    ticker = flask.request.args.get('ticker', '')
    last_updates_unix_timestamp = int(flask.request.args.get('last_updates_unix_timestamp', '0'))
    interval = flask.request.args.get('interval', '1m')
    future = executor.submit(getPrice, ticker, last_updates_unix_timestamp, interval)
    data = future.result()
    return flask.jsonify(data)

# curl -X POST "http://localhost:5123/api/summarize_ticker" -H "Content-Type: application/json" -d '{"ticker": "AAPL", "date": "2025-12-04", "news_list": ["news1", "news2"], "sentiment_list": [0.5, -0.2]}'
@app.route('/api/summarize_ticker', methods=['POST'])
def api_summarize_ticker():
    data = flask.request.get_json()
    ticker = data.get('ticker', '')
    date = data.get('date', '')
    news_list = data.get('news_list', [])
    sentiment_list = data.get('sentiment_list', [])
    full_text_list = data.get('full_text_list', None)
    max_tokens = data.get('max_tokens', 2048)
    
    if not ticker or not news_list:
        return flask.jsonify({'error': 'ticker and news_list are required'}), 400
    
    future = executor.submit(
        summarize_daily_news, 
        news_list, 
        sentiment_list, 
        max_tokens,
        ticker,
        date,
        full_text_list
    )
    result = future.result()
    return flask.jsonify(result)

# curl -X POST "http://localhost:5123/api/summarize_portfolio" -H "Content-Type: application/json" -d '{"user_id": "123", "date": "2025-12-04", "news_list": ["news1", "news2"], "sentiment_list": [0.5, -0.2], "tickers_list": ["AAPL", "TSLA"]}'
@app.route('/api/summarize_portfolio', methods=['POST'])
def api_summarize_portfolio():
    data = flask.request.get_json()
    user_id = data.get('user_id', '')
    date = data.get('date', '')
    holding_summaries = data.get('holding_summaries', [])
    max_tokens = data.get('max_tokens', 4096)

    if not holding_summaries:
        return flask.jsonify({'error': 'holding_summaries is required'}), 400

    future = executor.submit(
        summarize_portfolio_from_holdings,
        holding_summaries,
        max_tokens,
        user_id,
        date
    )
    result = future.result()
    return flask.jsonify(result)

@app.route('/api/running_summary', methods=['POST'])
def api_running_summary():
    data = flask.request.get_json()
    user_id = data.get('user_id', '')
    date = data.get('date', '')
    holding_summaries = data.get('holding_summaries', [])
    sector_data = data.get('sector_data', {})
    window_days = data.get('window_days', 30)
    max_tokens = data.get('max_tokens', 8192)

    if not holding_summaries:
        return flask.jsonify({'error': 'holding_summaries is required'}), 400

    future = executor.submit(
        generate_running_summary,
        holding_summaries,
        sector_data,
        date,
        window_days,
        max_tokens,
    )
    result = future.result()
    return flask.jsonify(result)

@lru_cache(maxsize=2048)
def convert_isin_to_ticker(isin):
    """Convert ISIN to ticker using OpenFIGI API with caching"""
    if not isin or isin == 'N/A':
        return None
    
    try:
        import requests
        url = 'https://api.openfigi.com/v3/mapping'
        headers = {'Content-Type': 'application/json'}
        payload = [{"idType": "ID_ISIN", "idValue": isin}]
        
        with requests.post(url, json=payload, headers=headers, timeout=5) as response:
            if response.status_code == 200:
                data = response.json()
                if data and len(data) > 0 and 'data' in data[0]:
                    results = data[0]['data']
                    if results and len(results) > 0:
                        ticker = results[0].get('ticker')
                        if ticker:
                            return ticker
    except Exception as e:
        print(f"OpenFIGI lookup failed for {isin}: {e}")

    return None

# curl "http://localhost:5123/api/isin_to_ticker?isin=US0378331005"
@app.route('/api/isin_to_ticker', methods=['GET'])
def api_isin_to_ticker():
    isin = flask.request.args.get('isin', '')
    
    if not isin:
        return flask.jsonify({'error': 'ISIN parameter is required'}), 400
    
    ticker = convert_isin_to_ticker(isin)
    
    if ticker:
        return flask.jsonify({'isin': isin, 'ticker': ticker})
    else:
        return flask.jsonify({'error': 'Could not convert ISIN to ticker'}), 404
    
@app.route('/api/ticker_to_isin', methods=['GET'])
def api_ticker_to_isin():
    ticker = flask.request.args.get('ticker', '')
    
    if not ticker:
        return flask.jsonify({'error': 'Ticker parameter is required'}), 400
    
    isin = convert_ticker_to_isin(ticker)
    
    if isin:
        return flask.jsonify({'ticker': ticker, 'isin': isin})
    else:
        return flask.jsonify({'error': 'Could not convert ticker to ISIN'}), 404

# curl "http://localhost:5123/api/stock/US0378331005"
@app.route('/api/stock/<isin>', methods=['GET'])
def api_stock_data(isin):
    try:
        future = executor.submit(get_stock_data, isin)
        result = future.result()
        if result:
            return flask.jsonify({
                'isin': isin,
                'metrics': result.metrics.__dict__ if hasattr(result.metrics, '__dict__') else result.metrics,
                'financials': result.financials.__dict__ if hasattr(result.financials, '__dict__') else result.financials
            })
        return flask.jsonify({'error': 'Could not fetch stock data for this ISIN'}), 404
    except Exception as e:
        return flask.jsonify({'error': str(e)}), 500
    
@app.route('/api/stock/history/<ticker>', methods=['GET'])
def api_stock_history(ticker):
    try:
        future = executor.submit(getPriceDataOld, ticker)
        result = future.result()
        if result:
            return flask.jsonify({
                'ticker': ticker,
                'history': result
            })
        return flask.jsonify({'error': 'Could not fetch stock history for this ticker'}), 404
    except Exception as e:
        return flask.jsonify({'error': str(e)}), 500
    
@app.route('/api/convert_currency', methods=['GET'])
def api_convert_currency():
    amount = float(flask.request.args.get('amount', '0'))
    from_currency = flask.request.args.get('from_currency', 'USD')
    to_currency = flask.request.args.get('to_currency', 'USD')
    
    future = executor.submit(
        convertCurrency, 
        amount, 
        from_currency, 
        to_currency
    )
    converted_amount = future.result()
    
    return flask.jsonify({
        'amount': amount,
        'from_currency': from_currency,
        'to_currency': to_currency,
        'converted_amount': converted_amount
    })

if __name__ == '__main__':
    app.run(debug=False, host='127.0.0.1', port=BACKEND_PYTHON_PORT, threaded=True)