import ollama

def summarize_daily_news(news_list, sentiment_list, max_tokens=2048, ticker: str = "", date: str = "", full_text_list=None):
    if not news_list:
        return {
            'ticker': ticker,
            'date': date,
            'summary': f"No news available for {ticker} on {date}.",
            'sentiment': 0.0
        }
    
    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")
    
    if full_text_list and len(full_text_list) != len(news_list):
        raise ValueError("full_text_list must have same length as news_list")
    
    average_sentiment = sum(sentiment_list) / len(sentiment_list) if sentiment_list else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"

    # Estimate token count (rough: 1 token ≈ 4 chars)
    # Reserve ~1500 tokens for prompt structure and response
    max_context_chars = (32000 - 1500 - max_tokens) * 4
    
    combined_items = []
    current_chars = 0
    
    for i, (summary, sentiment) in enumerate(zip(news_list, sentiment_list), 1):
        sent_label = "+" if sentiment > 0.2 else "-" if sentiment < -0.2 else "~"
        
        # Try to use full text if available and fits in context window
        content = summary
        if full_text_list and full_text_list[i-1]:
            full_text = full_text_list[i-1].strip()
            # Only use full text if it's significantly longer and we have room
            if len(full_text) > len(summary) * 1.5:
                estimated_item_chars = len(full_text) + 50  # +50 for formatting
                if current_chars + estimated_item_chars <= max_context_chars:
                    content = full_text
                    current_chars += estimated_item_chars
                else:
                    # Not enough room for full text, use summary
                    content = summary
                    current_chars += len(summary) + 50
            else:
                content = summary
                current_chars += len(summary) + 50
        else:
            current_chars += len(summary) + 50
        
        combined_items.append(f"Article {i} [{sent_label} {sentiment:.2f}]:\n{content}")

    news_block = "\n\n".join(combined_items)

    prompt = f"""You are a financial analyst. Create a detailed daily market summary for {ticker} on {date}.

ARTICLES ({len(news_list)} total):
{news_block}

Overall Sentiment: {sentiment_label} (avg score: {average_sentiment:.2f})

Write a comprehensive summary (8-12 sentences) in markdown format covering:

## TL;DR
[2-3 sentences: What happened today and why it matters]

## Key Developments
[3-5 bullet points of most important news with specific details, numbers, dates]

## Market Impact
[2-3 sentences: How this affects stock price, investor sentiment, fundamentals]

## What to Watch
[1-2 sentences: Risks, opportunities, or upcoming events]

Be factual and specific. Include company names, numbers, percentages, dates, analyst targets, earnings figures, and other concrete details from the articles. Use markdown formatting (headers, bold, lists)."""

    response = ollama.generate(
        model="nidumai/nidum-gemma-3-4b-it-uncensored:q3_k_m",
        prompt=prompt,
        options={
            "num_predict": max_tokens,
            "temperature": 0.4,
            "repeat_penalty": 1.15,
            "top_k": 40,
            "top_p": 0.9,
        }
    )

    summary_text = response['response'].strip()
    
    # Clean up repetitive content while preserving structure
    lines = summary_text.split('\n')
    seen_content = set()
    clean_lines = []
    
    for line in lines:
        line_stripped = line.strip()
        # Keep headers and formatting even if similar
        if line_stripped.startswith('#') or line_stripped.startswith('-') or line_stripped.startswith('*'):
            clean_lines.append(line)
        elif line_stripped and line_stripped not in seen_content:
            seen_content.add(line_stripped)
            clean_lines.append(line)
        elif not line_stripped:  # Keep blank lines for formatting
            clean_lines.append(line)
    
    summary_text = '\n'.join(clean_lines)

    res = {
        'ticker': ticker,
        'date': date,
        'summary': summary_text,
        'sentiment': average_sentiment
    }
    return res

def summarize_daily_portfolio_news(news_list, sentiment_list, tickers_list, max_tokens=2048, user_id: str = "", date: str = "", full_text_list=None):
    if not news_list:
        return {
            'user_id': user_id,
            'date': date,
            'summary': f"No news available for your portfolio on {date}.",
            'sentiment': 0.0
        }
    
    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")
    
    if full_text_list and len(full_text_list) != len(news_list):
        raise ValueError("full_text_list must have same length as news_list")
    
    average_sentiment = sum(sentiment_list) / len(sentiment_list) if sentiment_list else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"
    
    # Token budget management (32k context window)
    max_context_chars = (32000 - 1500 - max_tokens) * 4
    
    # Group news by ticker with full text when available
    ticker_news = {}
    current_chars = 0
    
    for i, (summary, sentiment, ticker) in enumerate(zip(news_list, sentiment_list, tickers_list)):
        if ticker not in ticker_news:
            ticker_news[ticker] = []
        
        sent_label = "+" if sentiment > 0.2 else "-" if sentiment < -0.2 else "~"
        
        # Use full text if available and fits
        content = summary
        if full_text_list and full_text_list[i]:
            full_text = full_text_list[i].strip()
            if len(full_text) > len(summary) * 1.5:
                estimated_chars = len(full_text) + 100
                if current_chars + estimated_chars <= max_context_chars:
                    content = full_text
                    current_chars += estimated_chars
                else:
                    content = summary
                    current_chars += len(summary) + 100
            else:
                content = summary
                current_chars += len(summary) + 100
        else:
            current_chars += len(summary) + 100
        
        ticker_news[ticker].append({
            'label': sent_label,
            'sentiment': sentiment,
            'content': content
        })
    
    # Build news block grouped by ticker
    news_sections = []
    for ticker, items in ticker_news.items():
        ticker_section = f"### {ticker}\n"
        for idx, item in enumerate(items[:5], 1):  # Max 5 articles per ticker
            ticker_section += f"Article {idx} [{item['label']} {item['sentiment']:.2f}]:\n{item['content']}\n\n"
        news_sections.append(ticker_section)
    
    news_block = "\n".join(news_sections[:10])  # Max 10 tickers to avoid overflow
    unique_tickers = list(set(tickers_list))

    prompt = f"""You are a portfolio analyst. Create a comprehensive daily portfolio update for {date}.

PORTFOLIO HOLDINGS: {', '.join(unique_tickers[:15])}
Overall Sentiment: {sentiment_label} (score: {average_sentiment:.2f})

NEWS BY HOLDING:
{news_block}

Write a detailed portfolio summary (10-15 sentences) in markdown format:

## TL;DR
[3-4 sentences: Overall market day, biggest movers, key themes]

## Top Movers
[3-5 bullet points: Which holdings had significant news, what happened, specific numbers/percentages]

## Earnings & Fundamentals
[2-3 sentences: Any earnings reports, revenue/EPS beats/misses, guidance changes]

## Catalysts & Events
[2-3 sentences: Product launches, upgrades/downgrades, regulatory news, partnerships]

## Risk & Opportunity
[2-3 sentences: What to watch, upcoming events, potential concerns]

Be specific with numbers, dates, company names, analyst targets, and financial metrics. Use markdown formatting."""

    response = ollama.generate(
        model="nidumai/nidum-gemma-3-4b-it-uncensored:q3_k_m",
        prompt=prompt,
        options={
            "num_predict": max_tokens,
            "temperature": 0.4,
            "repeat_penalty": 1.15,
            "top_k": 40,
            "top_p": 0.9,
        }
    )

    summary_text = response['response'].strip()
    
    # Clean up repetitive content while preserving markdown structure
    lines = summary_text.split('\n')
    seen_content = set()
    clean_lines = []
    
    for line in lines:
        line_stripped = line.strip()
        # Keep markdown formatting
        if line_stripped.startswith('#') or line_stripped.startswith('-') or line_stripped.startswith('*'):
            clean_lines.append(line)
        elif line_stripped and line_stripped not in seen_content:
            seen_content.add(line_stripped)
            clean_lines.append(line)
        elif not line_stripped:
            clean_lines.append(line)
    
    summary_text = '\n'.join(clean_lines)
    
    # Fallback if model produced garbage
    if len(summary_text) < 100:
        summary_parts = []
        for ticker, items in list(ticker_news.items())[:5]:
            if items:
                summary_parts.append(f"**{ticker}**: {items[0]['content'][:200]}")
        summary_text = f"## Portfolio Update - {sentiment_label}\n\n" + "\n\n".join(summary_parts) if summary_parts else f"No significant news for your holdings on {date}."

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
