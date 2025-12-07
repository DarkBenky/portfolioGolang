import ollama

def summarize_daily_news(news_list, sentiment_list, max_tokens=1024, ticker: str = "", date: str = ""):
    if not news_list:
        return {
            'ticker': ticker,
            'date': date,
            'summary': f"No news available for {ticker} on {date}.",
            'sentiment': 0.0
        }
    
    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")
    
    average_sentiment = sum(sentiment_list) / len(sentiment_list) if sentiment_list else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"

    combined_items = []
    for i, (summary, sentiment) in enumerate(zip(news_list, sentiment_list), 1):
        sent_label = "+" if sentiment > 0.2 else "-" if sentiment < -0.2 else "~"
        combined_items.append(f"{i}. [{sent_label}] {summary}")

    news_block = "\n".join(combined_items)

    prompt = f"""Summarize today's news for {ticker} ({date}).

NEWS:
{news_block}

Overall sentiment: {sentiment_label} (score: {average_sentiment:.2f})

Write a brief 3-4 sentence summary covering:
1. The most important development today
2. How it might affect the stock price
3. Any risks to watch

Be specific - use actual numbers and facts from the news. Do not use bullet points or headers."""

    response = ollama.generate(
        model="nidumai/nidum-gemma-3-4b-it-uncensored:q3_k_m",
        prompt=prompt,
        options={
            "num_predict": max_tokens,
            "temperature": 0.3,
            "repeat_penalty": 1.2,
            "stop": ["\n\n\n", "---", "NEWS:", "##"]
        }
    )

    summary_text = response['response'].strip()
    
    # Clean up any repetitive content
    lines = summary_text.split('\n')
    seen = set()
    clean_lines = []
    for line in lines:
        line_stripped = line.strip()
        if line_stripped and line_stripped not in seen:
            seen.add(line_stripped)
            clean_lines.append(line)
    summary_text = '\n'.join(clean_lines)

    res = {
        'ticker': ticker,
        'date': date,
        'summary': summary_text,
        'sentiment': average_sentiment
    }
    return res

def summarize_daily_portfolio_news(news_list, sentiment_list, tickers_list, max_tokens=1024, user_id: str = "", date: str = ""):
    """Summarize news across entire portfolio"""
    if not news_list:
        return {
            'user_id': user_id,
            'date': date,
            'summary': f"No news available for your portfolio on {date}.",
            'sentiment': 0.0
        }
    
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
    
    # Build concise news block
    news_items = []
    for ticker, items in ticker_news.items():
        # Limit to top 3 news per ticker to avoid overwhelming the model
        for item in items[:3]:
            news_items.append(f"{ticker}: {item}")
    
    news_block = "\n".join(news_items[:15])  # Limit total items
    unique_tickers = list(set(tickers_list))

    prompt = f"""Portfolio Update for {date}

Holdings: {', '.join(unique_tickers)}
Overall Sentiment: {sentiment_label} (score: {average_sentiment:.2f})

Today's News:
{news_block}

Write a 4-5 sentence portfolio summary:
1. Which holdings had significant news today and what happened
2. The biggest mover (positive or negative) and why
3. Any earnings, upgrades/downgrades, or major announcements
4. Key risk or opportunity to watch

Use specific facts and numbers. Write in plain paragraphs, no bullet points or headers."""

    response = ollama.generate(
        model="nidumai/nidum-gemma-3-4b-it-uncensored:q3_k_m",
        prompt=prompt,
        options={
            "num_predict": max_tokens,
            "temperature": 0.3,
            "repeat_penalty": 1.3,
            "stop": ["\n\n\n", "---", "Today's News:", "Holdings:", "##", "**"]
        }
    )

    summary_text = response['response'].strip()
    
    # Clean up any repetitive content
    lines = summary_text.split('\n')
    seen = set()
    clean_lines = []
    for line in lines:
        line_stripped = line.strip()
        if line_stripped and line_stripped not in seen:
            seen.add(line_stripped)
            clean_lines.append(line)
    summary_text = '\n'.join(clean_lines)
    
    # Fallback if model produced garbage
    if len(summary_text) < 50 or summary_text.count('**') > 4:
        # Generate a simple factual summary
        summary_parts = []
        for ticker, items in list(ticker_news.items())[:3]:
            if items:
                summary_parts.append(f"{ticker}: {items[0].replace('[+]', '').replace('[-]', '').replace('[~]', '').strip()}")
        summary_text = f"Portfolio update ({sentiment_label}): " + " | ".join(summary_parts) if summary_parts else f"No significant news for your holdings on {date}."

    res = {
        'user_id': user_id,
        'date': date,
        'summary': summary_text,
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
