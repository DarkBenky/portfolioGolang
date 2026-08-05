import requests
import json
from env import DEEPSEEK_API_KEY, DEEPSEEK_MODEL

DEEPSEEK_URL = "https://api.deepseek.com/chat/completions"

def _call_deepseek(prompt, max_tokens=2048):
    if not DEEPSEEK_API_KEY:
        raise RuntimeError("DEEPSEEK_API_KEY not configured")

    payload = {
        "model": DEEPSEEK_MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": max_tokens,
    }

    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {DEEPSEEK_API_KEY}",
    }

    try:
        resp = requests.post(DEEPSEEK_URL, json=payload, headers=headers, timeout=90)
        if not resp.ok:
            print(f"DeepSeek error body: {resp.text[:500]}")
        resp.raise_for_status()
        data = resp.json()
        if data.get("choices") and len(data["choices"]) > 0:
            return data["choices"][0]["message"]["content"].strip()
        print(f"DeepSeek empty choices, full response: {json.dumps(data)[:500]}")
        return ""
    except Exception as e:
        print(f"DeepSeek API error: {e}")
        raise


def summarize_daily_news(news_list, sentiment_list, max_tokens=2048, ticker="", date="", full_text_list=None):
    if not news_list:
        return {
            "ticker": ticker,
            "date": date,
            "summary": f"No news available for {ticker} on {date}.",
            "sentiment": 0.0,
        }

    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")

    if full_text_list and len(full_text_list) != len(news_list):
        raise ValueError("full_text_list must have same length as news_list")

    average_sentiment = sum(sentiment_list) / len(sentiment_list) if sentiment_list else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"

    max_context_chars = (32000 - 1500 - max_tokens) * 4
    combined_items = []
    current_chars = 0

    for i, (summary, sentiment) in enumerate(zip(news_list, sentiment_list), 1):
        sent_label = "+" if sentiment > 0.2 else "-" if sentiment < -0.2 else "~"
        content = summary
        if full_text_list and full_text_list[i - 1]:
            full_text = full_text_list[i - 1].strip()
            if len(full_text) > len(summary) * 1.5:
                estimated_item_chars = len(full_text) + 50
                if current_chars + estimated_item_chars <= max_context_chars:
                    content = full_text
                    current_chars += estimated_item_chars
                else:
                    current_chars += len(summary) + 50
            else:
                current_chars += len(summary) + 50
        else:
            current_chars += len(summary) + 50
        combined_items.append(f"Article {i} [{sent_label} {sentiment:.2f}]:\n{content}")

    news_block = "\n\n".join(combined_items)

    prompt = f"""Analyze these {len(news_list)} news articles about {ticker} from {date} and create a clear, factual summary.

Articles:
{news_block}

Overall market sentiment: {sentiment_label} ({average_sentiment:.2f})

Write a concise summary in markdown with these sections:

## Summary
Write 3-4 sentences summarizing the most important developments with specific facts, numbers, and dates from the articles.

## Key Points
- List 3-5 bullet points with concrete details (earnings numbers, percentage changes, product launches, analyst ratings, etc.)

## Outlook
Write 1-2 sentences about potential impact on stock price or what investors should monitor.

Focus only on facts from the articles. Include specific numbers, percentages, dates, and company names. Do not add speculation or make up information."""

    try:
        summary_text = _call_deepseek(prompt, max_tokens)
        if not summary_text or len(summary_text) < 50:
            print(f"Warning: Model generated empty or short response. Using fallback.")
            summary_text = f"## Summary\n{ticker} had {len(news_list)} news items on {date}. Sentiment: {sentiment_label} ({average_sentiment:.2f})\n\n"
            summary_text += "\n".join([f"- {s}" for s in news_list[:5]])
    except Exception as e:
        print(f"Error generating summary with DeepSeek: {e}")
        summary_text = f"## Summary\n{ticker} had {len(news_list)} news items on {date}. Sentiment: {sentiment_label} ({average_sentiment:.2f})\n\n"
        summary_text += "\n".join([f"- {s}" for s in news_list[:5]])

    return {
        "ticker": ticker,
        "date": date,
        "summary": summary_text,
        "sentiment": average_sentiment,
    }


def summarize_portfolio_from_holdings(holding_summaries, max_tokens=4096, user_id="", date=""):
    if not holding_summaries:
        return {
            "user_id": user_id,
            "date": date,
            "summary": f"No holding summaries available for your portfolio on {date}.",
            "sentiment": 0.0,
        }

    sentiments = [hs.get("sentiment", 0.0) for hs in holding_summaries]
    average_sentiment = sum(sentiments) / len(sentiments) if sentiments else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"

    per_ticker_blocks = []
    ticker_briefs = []

    for hs in holding_summaries:
        ticker = hs.get("ticker", "???")
        summary = hs.get("summary", "")
        sentiment = hs.get("sentiment", 0.0)

        per_ticker_blocks.append(f"{ticker}\n{summary}\n")

        ticker_label = "Bullish" if sentiment > 0.2 else "Bearish" if sentiment < -0.2 else "Neutral"
        summary_snippet = summary[:400].replace("\n", " ").strip()
        ticker_briefs.append(f"{ticker}: {ticker_label} ({sentiment:.2f}) - {summary_snippet}")

    per_ticker_detail = "\n".join(per_ticker_blocks)
    ticker_brief_block = "\n\n".join(ticker_briefs[:30])

    unique_tickers = list(set(hs.get("ticker", "???") for hs in holding_summaries))

    prompt = f"""You are analyzing a portfolio with these holdings on {date}: {', '.join(unique_tickers[:20])}

Overall portfolio sentiment: {sentiment_label} ({average_sentiment:.2f})

Here are pre-generated summaries for each holding:

{ticker_brief_block}

Write a concise portfolio overview in markdown with exactly these sections:

## Portfolio Summary
Write 3-4 sentences highlighting the most impactful news across the portfolio. Mention which holdings had the most significant developments and the overall tone. Include specific facts and numbers from the summaries.

## Market Impact
Write 2-3 sentences about how these developments may affect the portfolio and what investors should monitor going forward.

Keep it focused on the big picture. The detailed per-holding summaries will be appended separately, so do not repeat individual article lists. Only include facts from the provided summaries. Do not speculate or invent information."""

    try:
        ai_overview = _call_deepseek(prompt, max_tokens)
        if not ai_overview or len(ai_overview) < 50:
            ai_overview = f"## Portfolio Summary\nYour portfolio had summarized news across {len(unique_tickers)} holdings on {date}. Overall sentiment: {sentiment_label} ({average_sentiment:.2f}).\n\n## Market Impact\nReview individual holding details below for specific developments affecting your positions."
    except Exception as e:
        print(f"Error generating portfolio overview with DeepSeek: {e}")
        ai_overview = f"## Portfolio Summary\nYour portfolio had summarized news across {len(unique_tickers)} holdings on {date}. Overall sentiment: {sentiment_label} ({average_sentiment:.2f}).\n\n## Market Impact\nReview individual holding details below for specific developments affecting your positions."

    combined_summary = f"Portfolio Update - {sentiment_label}\n\n{ai_overview}\n\n---\n\n## Individual Holdings Detail\n\n{per_ticker_detail}"

    return {"user_id": user_id, "date": date, "summary": combined_summary, "sentiment": average_sentiment}


def _build_portfolio_fallback(ticker_news, sentiment_label, date):
    summary_parts = []
    for ticker, items in ticker_news.items():
        if not items:
            continue
        ticker_sent = sum(it["sentiment"] for it in items) / len(items)
        ticker_label = "Bullish" if ticker_sent > 0.2 else "Bearish" if ticker_sent < -0.2 else "Neutral"
        block = f"{ticker} had {len(items)} news items on {date}. Sentiment: {ticker_label} ({ticker_sent:.2f})\n"
        for it in items:
            block += f"{it['content']}\n"
        summary_parts.append(block)
    if summary_parts:
        return f"Portfolio Update - {sentiment_label}\n\n" + "\n".join(summary_parts)
    return f"No significant news for your holdings on {date}."


def summarize_daily_portfolio_news(news_list, sentiment_list, tickers_list, max_tokens=4096, user_id="", date="", full_text_list=None):
    if not news_list:
        return {
            "user_id": user_id,
            "date": date,
            "summary": f"No news available for your portfolio on {date}.",
            "sentiment": 0.0,
        }

    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")

    if full_text_list and len(full_text_list) != len(news_list):
        raise ValueError("full_text_list must have same length as news_list")

    average_sentiment = sum(sentiment_list) / len(sentiment_list) if sentiment_list else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"

    ticker_news = {}
    for i, (summary, sentiment, ticker) in enumerate(zip(news_list, sentiment_list, tickers_list)):
        if ticker not in ticker_news:
            ticker_news[ticker] = []
        content = summary
        if full_text_list and full_text_list[i]:
            full_text = full_text_list[i].strip()
            if len(full_text) > len(summary) * 1.5:
                content = full_text
        ticker_news[ticker].append({"sentiment": sentiment, "content": content})

    per_ticker_blocks = []
    for ticker, items in ticker_news.items():
        ticker_sent = sum(it["sentiment"] for it in items) / len(items)
        ticker_label = "Bullish" if ticker_sent > 0.2 else "Bearish" if ticker_sent < -0.2 else "Neutral"
        block = f"{ticker} had {len(items)} news items on {date}. Sentiment: {ticker_label} ({ticker_sent:.2f})\n"
        for it in items:
            block += f"{it['content']}\n"
        per_ticker_blocks.append(block)

    per_ticker_detail = "\n".join(per_ticker_blocks)

    ticker_summaries_for_prompt = []
    for ticker, items in ticker_news.items():
        ticker_sent = sum(it["sentiment"] for it in items) / len(items)
        ticker_label = "Bullish" if ticker_sent > 0.2 else "Bearish" if ticker_sent < -0.2 else "Neutral"
        item_summaries = "\n".join([f"  - {it['content'][:300]}" for it in items[:5]])
        ticker_summaries_for_prompt.append(
            f"{ticker}: {len(items)} articles, {ticker_label} ({ticker_sent:.2f})\n{item_summaries}"
        )

    ticker_summary_block = "\n\n".join(ticker_summaries_for_prompt[:20])
    unique_tickers = list(set(tickers_list))

    prompt = f"""You are analyzing a portfolio with these holdings on {date}: {', '.join(unique_tickers[:20])}

Overall portfolio sentiment: {sentiment_label} ({average_sentiment:.2f})

Here are pre-summarized news for each holding:

{ticker_summary_block}

Write a concise portfolio overview in markdown with exactly these sections:

## Portfolio Summary
Write 3-4 sentences highlighting the most impactful news across the portfolio. Mention which holdings had the most significant developments and the overall tone. Include specific facts and numbers.

## Market Impact
Write 2-3 sentences about how these developments may affect the portfolio and what investors should monitor going forward.

Keep it focused on the big picture. The detailed per-holding news will be appended separately, so do not repeat individual article lists. Only include facts from the provided summaries. Do not speculate or invent information."""

    try:
        ai_overview = _call_deepseek(prompt, max_tokens)
        if not ai_overview or len(ai_overview) < 50:
            print(f"Warning: Model generated empty or short portfolio overview. Using fallback.")
            ai_overview = f"## Portfolio Summary\nYour portfolio had news across {len(unique_tickers)} holdings on {date}. Overall sentiment: {sentiment_label} ({average_sentiment:.2f}).\n\n## Market Impact\nReview individual holding details below for specific developments affecting your positions."
    except Exception as e:
        print(f"Error generating portfolio overview with DeepSeek: {e}")
        ai_overview = f"## Portfolio Summary\nYour portfolio had news across {len(unique_tickers)} holdings on {date}. Overall sentiment: {sentiment_label} ({average_sentiment:.2f}).\n\n## Market Impact\nReview individual holding details below for specific developments affecting your positions."

    combined_summary = f"Portfolio Update - {sentiment_label}\n\n{ai_overview}\n\n---\n\n## Individual Holdings Detail\n\n{per_ticker_detail}"

    return {"user_id": user_id, "date": date, "summary": combined_summary, "sentiment": average_sentiment}


def _extract_json_from_text(text):
    import re
    text = text.strip()
    if text.startswith('```'):
        text = re.sub(r'^```(?:json)?\s*\n?', '', text)
        text = re.sub(r'\n?```\s*$', '', text)
    first_brace = text.find('{')
    if first_brace == -1:
        return None
    depth = 0
    in_string = False
    escape_next = False
    for i in range(first_brace, len(text)):
        ch = text[i]
        if escape_next:
            escape_next = False
            continue
        if ch == '\\':
            escape_next = True
            continue
        if ch == '"' and not escape_next:
            in_string = not in_string
            continue
        if in_string:
            continue
        if ch == '{':
            depth += 1
        elif ch == '}':
            depth -= 1
            if depth == 0:
                return text[first_brace:i + 1]
    return None


def generate_running_summary(holding_summaries, sector_data, date, window_days, max_tokens=8192):
    if not holding_summaries:
        return {
            "user_id": "",
            "date": date,
            "window_days": window_days,
            "summary": f"No holding data available for the last {window_days} days.",
            "sentiment": 0.0,
            "sector_predictions": [],
            "theme_predictions": [],
        }

    sentiments = [hs.get("sentiment", 0.0) for hs in holding_summaries]
    average_sentiment = sum(sentiments) / len(sentiments) if sentiments else 0.0
    sentiment_label = "Bullish" if average_sentiment > 0.2 else "Bearish" if average_sentiment < -0.2 else "Neutral"

    ticker_briefs = []
    max_brief_chars = 12000
    current_chars = 0
    for hs in holding_summaries[:40]:
        ticker = hs.get("ticker", "???")
        summary = hs.get("summary", "")
        sent = hs.get("sentiment", 0.0)
        tlabel = "Bullish" if sent > 0.2 else "Bearish" if sent < -0.2 else "Neutral"
        snippet = summary[:400].replace("\n", " ").strip()
        brief = f"{ticker} [{tlabel} {sent:.2f}]: {snippet}"
        if current_chars + len(brief) > max_brief_chars:
            break
        ticker_briefs.append(brief)
        current_chars += len(brief) + 2

    ticker_brief_block = "\n\n".join(ticker_briefs)

    sectors_block = ""
    if sector_data:
        sector_lines = []
        for sector_name, sector_info in sector_data.items():
            tickers_list = sector_info.get("tickers", [])
            alloc = sector_info.get("allocation", 0)
            sector_lines.append(f"{sector_name} ({alloc:.1f}% of portfolio): {', '.join(tickers_list[:10])}")
        sectors_block = "\n".join(sector_lines)

    prompt = f"""You are a senior financial analyst producing a running summary report covering the last {window_days} days ending {date}.

## Ticker Summaries Across the Period
{ticker_brief_block}

## Portfolio Sector Allocation
{sectors_block if sectors_block else "No sector data available."}

Overall portfolio sentiment across the window: {sentiment_label} ({average_sentiment:.2f})

Produce a comprehensive report. You MUST respond ONLY with a valid JSON object using this exact schema:

{{
  "executive_summary": "3-4 sentence overview of the most impactful developments across the entire {window_days}-day window. Include specific facts, dates, numbers, and company names.",
  "key_themes": [
    {{ "theme": "Theme Name", "description": "2-3 sentences describing the theme with supporting evidence from the data.", "impact_sentiment": 0.5 }}
  ],
  "sector_predictions": [
    {{
      "sector": "Sector Name",
      "scenarios": [
        {{ "label": "Very Positive", "probability": 10, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Positive", "probability": 35, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Neutral", "probability": 30, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Negative", "probability": 20, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Very Negative", "probability": 5, "description": "What would drive this outcome and its impact." }}
      ]
    }}
  ],
  "theme_predictions": [
    {{
      "theme": "Theme Name",
      "scenarios": [
        {{ "label": "Very Positive", "probability": 10, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Positive", "probability": 30, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Neutral", "probability": 35, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Negative", "probability": 20, "description": "What would drive this outcome and its impact." }},
        {{ "label": "Very Negative", "probability": 5, "description": "What would drive this outcome and its impact." }}
      ]
    }}
  ],
  "portfolio_outlook": {{
    "scenarios": [
      {{ "label": "Very Positive", "probability": 10, "description": "What would drive this outcome." }},
      {{ "label": "Positive", "probability": 30, "description": "What would drive this outcome." }},
      {{ "label": "Neutral", "probability": 35, "description": "What would drive this outcome." }},
      {{ "label": "Negative", "probability": 20, "description": "What would drive this outcome." }},
      {{ "label": "Very Negative", "probability": 5, "description": "What would drive this outcome." }}
    ]
  }}
}}

CRITICAL RULES:
- Probabilities for each set of scenarios MUST sum to exactly 100.
- impact_sentiment for themes must be between -1.0 and 1.0.
- Identify 3-6 key themes. If no sector data is available, return empty array for sector_predictions.
- Only include facts from the provided data. Do not invent information.
- Respond ONLY with valid JSON. No markdown, no commentary, no code fences."""

    try:
        raw_response = _call_deepseek(prompt, max_tokens)
        json_str = _extract_json_from_text(raw_response)
        if not json_str:
            print(f"Could not extract JSON from AI response. Raw: {raw_response[:500]}")
            raise ValueError("No JSON found in response")

        parsed = json.loads(json_str)

        return {
            "user_id": "",
            "date": date,
            "window_days": window_days,
            "summary": parsed.get("executive_summary", ""),
            "sentiment": average_sentiment,
            "sector_predictions": parsed.get("sector_predictions", []),
            "theme_predictions": parsed.get("theme_predictions", []),
            "key_themes": parsed.get("key_themes", []),
            "portfolio_outlook": parsed.get("portfolio_outlook", {}),
        }

    except Exception as e:
        print(f"Error generating running summary: {e}")
        fallback_exec = f"Over the last {window_days} days, your portfolio showed {sentiment_label.lower()} sentiment ({average_sentiment:.2f}). Key holdings included {', '.join(list(set(hs.get('ticker', '?') for hs in holding_summaries))[:10])}."
        return {
            "user_id": "",
            "date": date,
            "window_days": window_days,
            "summary": fallback_exec,
            "sentiment": average_sentiment,
            "sector_predictions": [],
            "theme_predictions": [],
            "key_themes": [],
            "portfolio_outlook": {},
        }


if __name__ == "__main__":
    test_news = [
        "Apple Q4 earnings beat: EPS $1.64 vs $1.58, revenue +8% YoY",
        "Apple announces $90B buyback program",
        "iPhone 16 gains 3% market share in China",
        "Analyst upgrades to Strong Buy, PT $250",
        "Key supplier reports 15% production delays",
    ]
    test_sentiments = [0.85, 0.78, 0.72, 0.88, -0.45]
    print(f"\nInput: {len(test_news)} news items for AAPL")
    print("\nGenerating summary...\n")
    try:
        summary = summarize_daily_news(test_news, test_sentiments, max_tokens=512, ticker="AAPL", date="2025-12-06")
        print("-" * 80)
        print(summary["summary"])
        print("-" * 80)
    except Exception as e:
        print(f"Error during summarization: {e}")
