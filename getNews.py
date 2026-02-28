import yfinance as yf
from newspaper import Article
from transformers import BertTokenizer, BertForSequenceClassification
from transformers import pipeline
from env import BACKEND_PYTHON, BACKEND_PYTHON_PORT, BACKEND_GO, BACKEND_GO_PORT
import requests
from datetime import datetime, timezone

USER_AGENTS = [
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36',
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:89.0) Gecko/20100101 Firefox/89.0',
]

def creteSentimentAnalyzer():
    finbert_model = BertForSequenceClassification.from_pretrained('yiyanghkust/finbert-tone',num_labels=3)
    finbert_tokenizer = BertTokenizer.from_pretrained('yiyanghkust/finbert-tone')
    nlp = pipeline('sentiment-analysis', model=finbert_model, tokenizer=finbert_tokenizer)
    return nlp

def getSentiment(model: pipeline, title: str, summary: str, text: str, weights: list):    
    # Process title
    title_sentiment = model(title)
    title_sentiment_score = 0
    if title_sentiment[0]['label'] == 'Positive':
        title_sentiment_score = 1
    elif title_sentiment[0]['label'] == 'Negative':
        title_sentiment_score = -1
    
    # Process summary
    summary_sentiment = model(summary)
    summary_sentiment_score = 0
    if summary_sentiment[0]['label'] == 'Positive':
        summary_sentiment_score = 1
    elif summary_sentiment[0]['label'] == 'Negative':
        summary_sentiment_score = -1
    
    # Process text in chunks
    text_sentiment = 0
    count = len(text)//512 + 1
    for i in range(len(text)//512 + 1):
        text_chunk = text[i*512:(i+1)*512]
        if text_chunk.strip():  # Only process non-empty chunks
            result = model(text_chunk)[0]  # Get first result from list
            if result['label'] == 'Positive':
                text_sentiment += 1
            elif result['label'] == 'Negative':
                text_sentiment -= 1
    
    text_sentiment_score = text_sentiment / count if count > 0 else 0
    
    # Calculate weighted sentiment
    weighted_sentiment = (weights[0] * title_sentiment_score +
                          weights[1] * summary_sentiment_score +
                          weights[2] * text_sentiment_score) / sum(weights)
    
    return weighted_sentiment

def convert_pubdate_to_unix(pub_date_str):
    """
    Convert Yahoo Finance pubDate string to Unix timestamp in UTC.
    Yahoo typically returns timestamps as Unix seconds.
    """
    if not pub_date_str:
        return int(datetime.now(timezone.utc).timestamp())
    
    try:
        # Try to parse as Unix timestamp (seconds)
        if isinstance(pub_date_str, (int, float)):
            return int(pub_date_str)
        
        # If it's a string, try to convert to int
        if isinstance(pub_date_str, str):
            try:
                return int(pub_date_str)
            except ValueError:
                pass
            
            # Try parsing as ISO format or common date formats
            for fmt in ["%Y-%m-%dT%H:%M:%SZ", "%Y-%m-%d %H:%M:%S", "%Y-%m-%d"]:
                try:
                    dt = datetime.strptime(pub_date_str, fmt)
                    # Assume UTC if no timezone info
                    if dt.tzinfo is None:
                        dt = dt.replace(tzinfo=timezone.utc)
                    return int(dt.timestamp())
                except ValueError:
                    continue
    except Exception as e:
        print(f"Error converting pubDate '{pub_date_str}': {e}")
    
    # Fallback to current time
    return int(datetime.now(timezone.utc).timestamp())

# BEFORE FETCHING NEW OR CLASIFIENG THEM CHECK IF WE HAVE THIS NEWS ALREADY IN DB IF SO WE CAN SKIP IT

def getNews(Ticker: str, num_articles:int, model: pipeline):
    asset = yf.Ticker(Ticker)
    articles = []
    try:
        news_items = asset.news
        for item in news_items:
            if len(articles) >= num_articles:
                break
            content = item.get('content', item)
            canonicalUrl = content.get('canonicalUrl', '')
            title = content.get('title', '')
            summary = content.get('summary', '')

            # Handle canonicalUrl - it can be a dict or string
            if isinstance(canonicalUrl, dict):
                url = canonicalUrl.get('url', '')
            elif isinstance(canonicalUrl, str):
                url = canonicalUrl
            else:
                url = ''
            
            # Fetch article text first
            text = ''
            if url != '':
                try:
                    article = Article(url)
                    article.download()
                    article.parse()
                    text = article.text
                except requests.exceptions.HTTPError as e:
                    if hasattr(e, 'response') and e.response is not None and e.response.status_code == 403:
                        for user_agent in USER_AGENTS:
                            try:
                                headers = {'User-Agent': user_agent}
                                with requests.get(url, headers=headers, timeout=10) as response:
                                    response.raise_for_status()
                                    article = Article(url)
                                    article.html = response.text
                                    article.parse()
                                    text = article.text
                                break
                            except Exception:
                                continue
                    else:
                        print(f"Error fetching article from {url}: {e}")
                except Exception as e:
                    if '403' in str(e) or 'Forbidden' in str(e):
                        for user_agent in USER_AGENTS:
                            try:
                                headers = {'User-Agent': user_agent}
                                with requests.get(url, headers=headers, timeout=10) as response:
                                    response.raise_for_status()
                                    article = Article(url)
                                    article.html = response.text
                                    article.parse()
                                    text = article.text
                                break
                            except Exception:
                                continue
                        else:
                            print(f"Error fetching article from {url}: {e}")
                    else:
                        print(f"Error fetching article from {url}: {e}")

            # Check if news already exists in DB (now including text)
            try:
                with requests.get(
                    f"{BACKEND_GO}/news_exists",
                    params={"title": title, "summary": summary, "text": text[:500] if text else ""},
                    timeout=5
                ) as response:
                    if response.status_code == 200:
                        exists = response.json().get("exists", False)
                        if exists:
                            print(f"News already exists in DB: {title}")
                            continue
                    else:
                        print(f"Error checking news existence for {title}: {response.status_code}")
            except Exception as e:
                print(f"Error checking news existence: {e}")
            
            published_at_unix = convert_pubdate_to_unix(content.get('pubDate', ''))
            
            news = {
                'ticker': Ticker,  # Add ticker to the news item
                'title': title,
                'summary': summary,
                'text': text,
                'url': url,
                'published_at': published_at_unix,
                'author': content.get('provider', {}).get('displayName', '') if isinstance(content.get('provider'), dict) else '',
                'img_url': content.get('thumbnail', {}).get('originalUrl', ''),
                'sentiment': getSentiment(model, content.get('title', ''), content.get('summary', ''), text, [0.4, 0.35, 0.25])
            }
            articles.append(news)
    except Exception as e:
        print(f"Error retrieving news for {Ticker}: {e}")
    return articles        
                

