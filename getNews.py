import yfinance as yf
from newspaper import Article
from transformers import BertTokenizer, BertForSequenceClassification
from transformers import pipeline

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

def getNews(Ticker: str, num_articles:int, model: pipeline):
    asset = yf.Ticker(Ticker)
    articles = []
    try:
        news_items = asset.news[:num_articles]
        for item in news_items:
            content = item.get('content', item)
            canonicalUrl = content.get('canonicalUrl', '')
            url = canonicalUrl.get('url', '')
            text = ''
            if url != '':
                try:
                    article = Article(url)
                    article.download()
                    article.parse()
                    text = article.text
                except Exception as e:
                    print(f"Error fetching article from {url}: {e}")
            news = {
                'title': content.get('title', ''),
                'summary': content.get('summary', ''),
                'text': text,
                'url': url,
                'published_at': content.get('pubDate', ''),
                'author': content.get('provider', {}).get('displayName', '') if isinstance(content.get('provider'), dict) else '',
                'img_url': content.get('thumbnail', {}).get('originalUrl', ''),
                'sentiment': getSentiment(model, content.get('title', ''), content.get('summary', ''), text, [0.4, 0.35, 0.25])
            }
            articles.append(news)
    except Exception as e:
        print(f"Error retrieving news for {Ticker}: {e}")
    return articles        
                

