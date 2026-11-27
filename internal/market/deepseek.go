package market

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
)

const deepSeekSearchURL = "https://api.deepseek.com/v1/tools/search"

// The API rejects queries above 128 Unicode characters, so long model queries are trimmed.
const deepSeekQueryRunes = 128

type deepSeekSearchRequest struct {
	Query string `json:"query"`
	Count int    `json:"count,omitempty"`
}

type deepSeekSearchResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
		Summary string `json:"summary"`
	} `json:"results"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// searchDeepSeek queries the DeepSeek Search API and returns nil when
// DEEPSEEK_API_KEY is not set, so the provider costs nothing otherwise.
// The long summary is kept as the result text, which feeds the chat prompt
// without downloading every page.
func (c *Client) searchDeepSeek(query string, maxResults int) []SearchResult {
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" || maxResults <= 0 {
		return nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if runes := []rune(query); len(runes) > deepSeekQueryRunes {
		query = string(runes[:deepSeekQueryRunes])
	}

	payload, err := json.Marshal(deepSeekSearchRequest{Query: query, Count: maxResults})
	if err != nil {
		return nil
	}
	req, err := http.NewRequest(http.MethodPost, deepSeekSearchURL, bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}

	var parsed deepSeekSearchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	if parsed.Error != nil {
		return nil
	}
	results := make([]SearchResult, 0, len(parsed.Results))
	for _, item := range parsed.Results {
		if item.URL == "" {
			continue
		}
		snippet := strings.TrimSpace(item.Snippet)
		if snippet == "" {
			snippet = strings.TrimSpace(item.Summary)
		}
		results = append(results, SearchResult{
			Title:   stripTags(item.Title),
			URL:     item.URL,
			Snippet: stripTags(snippet),
			Text:    stripTags(item.Summary),
		})
	}
	return results
}
