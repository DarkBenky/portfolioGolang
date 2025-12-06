import ollama

def summarize_daily_news(news_list, sentiment_list, max_tokens=1024, ticker: str = "", date: str = ""):
    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")
    
    average_sentiment = sum(sentiment_list) / len(sentiment_list) if sentiment_list else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"

    combined_items = []
    for i, (summary, sentiment) in enumerate(zip(news_list, sentiment_list), 1):
        sent_label = "+" if sentiment > 0.2 else "-" if sentiment < -0.2 else "~"
        combined_items.append(f"{i}. [{sent_label}] {summary}")

    news_block = "\n".join(combined_items)

    system_prompt = f"""You are a financial analyst. Summarize news for {ticker} into a brief investment update.

RULES:
- Focus only on {ticker}-relevant information
- Use numbers from the news (e.g., "up 3%", "revenue +15%")
- Do NOT invent data
- Be concise

FORMAT:
## {ticker} ({date}) - {sentiment_label}

**Key News:**
- [Top 2-3 developments]

**Impact:** [1 sentence on stock implications]

**Risk:** [1 sentence if any concerns, else "None noted"]"""

    user_prompt = f"""NEWS [{sentiment_label}, score: {average_sentiment:.2f}]:
{news_block}

Summarize."""


    response = ollama.generate(
        model="nidumai/nidum-gemma-3-4b-it-uncensored:q3_k_m",
        prompt=f"{system_prompt}\n\n{user_prompt}",
        options={
            "num_predict": max_tokens,
            "temperature": 0.015,
        }
    )

    res = {
        'ticker': ticker,
        'date': date,
        'summary': response['response'].strip(),
        'sentiment': average_sentiment
    }
    return res

def summarize_daily_portfolio_news(news_list, sentiment_list, tickers_list, max_tokens=1024, user_id: str = "", date: str = ""):
    """Summarize news across entire portfolio"""
    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")
    
    average_sentiment = sum(sentiment_list) / len(sentiment_list) if sentiment_list else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"
    
    # Group news by ticker
    ticker_news = {}
    for summary, sentiment, ticker in zip(news_list, sentiment_list, tickers_list):
        if ticker not in ticker_news:
            ticker_news[ticker] = []
        sent_label = "+" if sentiment > 0.2 else "-" if sentiment < -0.2 else "~"
        ticker_news[ticker].append(f"[{sent_label}] {summary}")
    
    news_block = ""
    for ticker, items in ticker_news.items():
        news_block += f"{ticker}: " + " | ".join(items) + "\n"

    unique_tickers = list(set(tickers_list))
    
    system_prompt = f"""You are a portfolio analyst. Summarize news for holdings: {', '.join(unique_tickers)}.

RULES:
- [+] = positive, [-] = negative, [~] = neutral
- Prioritize by impact
- Do NOT invent data
- Be concise

FORMAT:
## Portfolio ({date}) - {sentiment_label}

**Headlines:** [Top 2-3 items across holdings]

**Risks:** [Any concerns, or "None"]

**Opportunities:** [Any positive catalysts, or "None"]"""

    user_prompt = f"""NEWS [Overall: {sentiment_label}, {average_sentiment:.2f}]:
{news_block}

Summarize."""

    response = ollama.generate(
        model="nidumai/nidum-gemma-3-4b-it-uncensored:q3_k_m",
        prompt=f"{system_prompt}\n\n{user_prompt}",
        options={
            "num_predict": max_tokens,
            "temperature": 0.015,
        }
    )

    res = {
        'user_id': user_id,
        'date': date,
        'summary': response['response'].strip(),
        'sentiment': average_sentiment
    }
    return res

if __name__ == '__main__':
    test_news = [
        "Apple Q4 earnings beat: EPS $1.64 vs $1.58, revenue +8% YoY",
        "Apple announces $90B buyback program",
        "iPhone 16 gains 3% market share in China",
        "Analyst upgrades to Strong Buy, PT $250",
        "Key supplier reports 15% production delays"
    ]
    
    test_sentiments = [0.85, 0.78, 0.72, 0.88, -0.45]
    
    print(f"\nInput: {len(test_news)} news items for AAPL")
    print("\nGenerating summary...\n")
    
    try:
        summary = summarize_daily_news(
            test_news, 
            test_sentiments, 
            max_tokens=512,
            ticker="AAPL",
            date="2025-12-06"
        )
        print("-" * 80)
        print(summary)
        print("-" * 80)
    except Exception as e:
        print(f"Error during summarization: {e}")
        import traceback
        traceback.print_exc()
