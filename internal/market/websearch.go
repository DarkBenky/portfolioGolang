package market

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Text    string `json:"text"`
	Date    string `json:"date,omitempty"`
}

type searchCache struct {
	mu      sync.Mutex
	entries map[string]searchCacheEntry
}

type searchCacheEntry struct {
	results []SearchResult
	status  string
	expires time.Time
}

var webCache = searchCache{entries: map[string]searchCacheEntry{}}

type rssFeed struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Search queries available providers in order: the DeepSeek Search API when
// DEEPSEEK_API_KEY is set, then a configured SearXNG, then Bing's RSS feed,
// then Yahoo news and Wikipedia as topical fallbacks.
func (c *Client) Search(query string, maxResults int) ([]SearchResult, string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, "empty query"
	}
	key := strings.ToLower(query)

	webCache.mu.Lock()
	if entry, ok := webCache.entries[key]; ok && time.Now().Before(entry.expires) {
		webCache.mu.Unlock()
		return entry.results, entry.status
	}
	webCache.mu.Unlock()

	var statuses []string
	providers := []struct {
		name string
		run  func(string, int) []SearchResult
	}{
		{"deepseek", c.searchDeepSeek},
		{"searxng", c.searchSearXNG},
		{"bing", c.searchBing},
		{"yahoo", c.searchYahoo},
		{"wikipedia", c.searchWikipedia},
	}
	var results []SearchResult
	for _, provider := range providers {
		found := provider.run(query, maxResults)
		if len(found) > 0 {
			results = found
			statuses = append(statuses, fmt.Sprintf("%s:ok(%d)", provider.name, len(found)))
			break
		}
		statuses = append(statuses, provider.name+":empty")
	}

	status := strings.Join(statuses, ";")
	// An empty result is not cached, so one provider outage does not block retries.
	if len(results) > 0 {
		webCache.mu.Lock()
		if len(webCache.entries) > 256 {
			webCache.entries = map[string]searchCacheEntry{}
		}
		webCache.entries[key] = searchCacheEntry{results: results, status: status, expires: time.Now().Add(time.Hour)}
		webCache.mu.Unlock()
	}
	return results, status
}

// SearchWithContent also downloads the readable text of the top results, which gives
// the report writer concrete facts instead of only headlines.
func (c *Client) SearchWithContent(query string, maxResults int) ([]SearchResult, string) {
	results, status := c.Search(query, maxResults)
	for i := range results {
		if i >= 3 || results[i].URL == "" || results[i].Text != "" {
			continue
		}
		results[i].Text = c.ArticleText(results[i].URL)
	}
	return results, status
}

func (c *Client) searchBing(query string, maxResults int) []SearchResult {
	body, err := c.getText("https://www.bing.com/search?format=rss&q="+url.QueryEscape(query), 30*time.Minute)
	if err != nil {
		return nil
	}
	var feed rssFeed
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		return nil
	}
	results := make([]SearchResult, 0, maxResults)
	for _, item := range feed.Channel.Items {
		if len(results) >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:   stripTags(item.Title),
			URL:     item.Link,
			Snippet: stripTags(item.Description),
		})
	}
	return results
}

func (c *Client) searchSearXNG(query string, maxResults int) []SearchResult {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("SEARXNG_URL")), "/")
	if base == "" {
		return nil
	}
	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := c.FetchJSON(base+"/search?format=json&q="+url.QueryEscape(query), 30*time.Minute, &payload); err != nil {
		return nil
	}
	results := make([]SearchResult, 0, maxResults)
	for _, item := range payload.Results {
		if len(results) >= maxResults {
			break
		}
		results = append(results, SearchResult{Title: item.Title, URL: item.URL, Snippet: item.Content})
	}
	return results
}

func (c *Client) searchYahoo(query string, maxResults int) []SearchResult {
	items, err := c.News(query, maxResults)
	if err != nil {
		return nil
	}
	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		results = append(results, SearchResult{Title: item.Title, URL: item.URL, Snippet: item.Summary})
	}
	return results
}

func (c *Client) searchWikipedia(query string, maxResults int) []SearchResult {
	var payload struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	target := "https://en.wikipedia.org/w/api.php?action=query&list=search&srlimit=" +
		fmt.Sprint(maxResults) + "&format=json&srsearch=" + url.QueryEscape(query)
	if err := c.FetchJSON(target, 6*time.Hour, &payload); err != nil {
		return nil
	}
	results := make([]SearchResult, 0, len(payload.Query.Search))
	for _, item := range payload.Query.Search {
		results = append(results, SearchResult{
			Title:   item.Title,
			URL:     "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(item.Title, " ", "_"),
			Snippet: stripTags(item.Snippet),
		})
	}
	return results
}

var tagPattern = regexp.MustCompile(`<[^>]+>`)
var entityPattern = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'", "&nbsp;", " ")

func stripTags(value string) string {
	value = tagPattern.ReplaceAllString(value, "")
	value = entityPattern.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}
