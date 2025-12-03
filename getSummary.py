import ollama

def summarize_daily_news(news_list, sentiment_list, max_tokens=1024):
    if len(news_list) != len(sentiment_list):
        raise ValueError("news_list and sentiment_list must have equal length")

    combined_items = []
    for summary, sentiment in zip(news_list, sentiment_list):
        combined_items.append(f"- Summary: {summary}\n  Sentiment: {sentiment}")

    news_block = "\n".join(combined_items)

    system_prompt = (
        "You are an analytical summarization model. Your task is to read multiple news summaries "
        "from the same day and produce a concise, factual daily briefing.\n"
        "Your goals:\n"
        "- Identify events with the highest real-world importance.\n"
        "- Use sentiment scores only to understand tone.\n"
        "- Combine related events into unified insights.\n"
        "- Remove redundancy and subjective language.\n"
        "- Output a structured summary with sections: Key Events, Market/Business, Technology, "
        "Politics/Geopolitics, Other Notable Items.\n"
        "- Never invent events not present in the input."
    )

    user_prompt = f"Here are today's collected news items:\n\n{news_block}\n\nGenerate the daily briefing."

    response = ollama.generate(
        model="nidumai/nidum-gemma-3-4b-it-uncensored:q3_k_m",
        prompt=f"{system_prompt}\n\n{user_prompt}",
        options={
            "num_predict": max_tokens,
            "temperature": 0.0
        }
    )

    return response['response']

if __name__ == '__main__':
    # Sample test data
    test_news = [
        "Apple announces new iPhone with advanced AI capabilities, stock rises 3%",
        "Tech giant Apple unveils latest smartphone featuring breakthrough artificial intelligence",
        "Federal Reserve hints at possible rate cuts in Q2 2025",
        "Ukraine and Russia agree to temporary ceasefire for humanitarian aid",
        "Tesla recalls 50,000 vehicles due to software glitch in autopilot system",
        "Major data breach at healthcare provider affects 2 million patients"
    ]
    
    test_sentiments = [
        "positive (0.85)",
        "positive (0.78)",
        "neutral (0.52)",
        "positive (0.65)",
        "negative (-0.72)",
        "negative (-0.88)"
    ]
    
    print(f"\nInput: {len(test_news)} news items")
    print("\nGenerating summary...\n")
    
    try:
        summary = summarize_daily_news(test_news, test_sentiments, max_tokens=512)
        print("-" * 80)
        print(summary)
        print("-" * 80)
    except Exception as e:
        print(f"Error during summarization: {e}")
        import traceback
        traceback.print_exc()
