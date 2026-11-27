package market

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func stubResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)),
		Header: http.Header{"Content-Type": []string{"application/json"}}}
}

func stubClient(t *testing.T, handler roundTripFunc) *Client {
	t.Helper()
	client := NewClient()
	client.http = &http.Client{Transport: handler}
	return client
}

func withDeepSeekKey(t *testing.T, key string) {
	t.Helper()
	t.Setenv("DEEPSEEK_API_KEY", key)
}

func TestSearchDeepSeekMapsDateAndSummary(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	var body map[string]any
	var auth string
	client := stubClient(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != deepSeekSearchURL {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
		}
		auth = request.Header.Get("Authorization")
		raw, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("request body: %v", err)
		}
		return stubResponse(200, `{"results":[
			{"title":"<b>Tariffs</b>","url":"https://example.com/a","snippet":"short","summary":"long summary","date":"2026-11-27T16:38:22Z"}]}`), nil
	})

	results := client.searchDeepSeek("tariffs", 5)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	result := results[0]
	if result.Title != "Tariffs" || result.Snippet != "short" || result.Text != "long summary" {
		t.Fatalf("mapping wrong: %+v", result)
	}
	if result.Date != "2026-11-27T16:38:22Z" {
		t.Fatalf("date not carried: %q", result.Date)
	}
	if auth != "Bearer test-key" {
		t.Fatalf("authorization = %q", auth)
	}
	if body["count"] != float64(5) {
		t.Fatalf("count = %v", body["count"])
	}
}

func TestSearchDeepSeekFallsBackToSummaryForSnippet(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	client := stubClient(t, func(*http.Request) (*http.Response, error) {
		return stubResponse(200, `{"results":[{"title":"t","url":"https://example.com/b","snippet":"  ","summary":"only summary"}]}`), nil
	})
	results := client.searchDeepSeek("q", 5)
	if len(results) != 1 || results[0].Snippet != "only summary" || results[0].Text != "only summary" {
		t.Fatalf("results = %+v", results)
	}
}

func TestSearchDeepSeekTrimsQueryToAPILimit(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	var query string
	client := stubClient(t, func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		raw, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(raw, &body)
		query, _ = body["query"].(string)
		return stubResponse(200, `{"results":[]}`), nil
	})
	client.searchDeepSeek(strings.Repeat("z", 300), 5)
	if len([]rune(query)) != deepSeekQueryRunes {
		t.Fatalf("query runes = %d, want %d", len([]rune(query)), deepSeekQueryRunes)
	}
}

func TestSearchDeepSeekNeedsKey(t *testing.T) {
	withDeepSeekKey(t, "")
	client := stubClient(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("no request should be sent without a key")
		return nil, nil
	})
	if results := client.searchDeepSeek("q", 5); results != nil {
		t.Fatalf("results = %+v, want nil", results)
	}
}

func TestSearchDeepSeekDeduplicatesTrackingVariants(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	client := stubClient(t, func(*http.Request) (*http.Response, error) {
		return stubResponse(200, `{"results":[
			{"title":"live","url":"https://finance.yahoo.com/?guccounter=1&sig=aaa#1","snippet":"a","summary":"a"},
			{"title":"live again","url":"https://finance.yahoo.com/?guccounter=1&sig=bbb#1","snippet":"b","summary":"b"},
			{"title":"other page","url":"https://finance.yahoo.com/news/story","snippet":"c","summary":"c"}]}`), nil
	})
	results := client.searchDeepSeek("q", 5)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(results), results)
	}
	if results[0].Title != "live" || results[1].Title != "other page" {
		t.Fatalf("results = %+v", results)
	}
}

func TestSearchDeepSeekRejectsBrokenURLs(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	client := stubClient(t, func(*http.Request) (*http.Response, error) {
		return stubResponse(200, `{"results":[
			{"title":"nested","url":"https://m.economictimes.com/a/articleshow/https:/economictimes.indiatimes.com/x.cms","snippet":"s","summary":"m"},
			{"title":"empty","url":"","snippet":"s","summary":"m"},
			{"title":"relative","url":"/news/story","snippet":"s","summary":"m"},
			{"title":"ftp","url":"ftp://example.com/file","snippet":"s","summary":"m"},
			{"title":"good","url":"https://example.com/ok","snippet":"s","summary":"m"}]}`), nil
	})
	results := client.searchDeepSeek("q", 5)
	if len(results) != 1 || results[0].Title != "good" {
		t.Fatalf("results = %+v", results)
	}
}

func TestSearchDeepSeekErrorsFallThroughToNextProvider(t *testing.T) {
	cases := map[string]*http.Response{
		"unauthorized":    stubResponse(401, `{"error":{"message":"Authentication Fails"}}`),
		"api error field": stubResponse(200, `{"error":{"message":"quota exceeded"}}`),
		"invalid json":    stubResponse(200, `not json`),
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			withDeepSeekKey(t, "bad-key")
			client := stubClient(t, func(request *http.Request) (*http.Response, error) {
				if strings.Contains(request.URL.Host, "deepseek") {
					return failure, nil
				}
				return stubResponse(200, `<rss><channel><item><title>fallback</title><link>https://example.com/f</link><description>d</description></item></channel></rss>`), nil
			})
			results, status := client.Search("fallthrough "+name, 5)
			if len(results) != 1 || results[0].Title != "fallback" {
				t.Fatalf("results = %+v status = %s", results, status)
			}
			if !strings.Contains(status, "deepseek:empty") || !strings.Contains(status, "bing:ok(1)") {
				t.Fatalf("status = %s", status)
			}
		})
	}
}

func TestSearchPrefersDeepSeekWhenKeyed(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	client := stubClient(t, func(*http.Request) (*http.Response, error) {
		return stubResponse(200, `{"results":[{"title":"ds","url":"https://example.com/ds","snippet":"s","summary":"m","date":"2026-11-27T10:00:00Z"}]}`), nil
	})
	results, status := client.Search("deepseek first query", 5)
	if len(results) != 1 || results[0].Title != "ds" || results[0].Date == "" {
		t.Fatalf("results = %+v", results)
	}
	if !strings.HasPrefix(status, "deepseek:ok(1)") {
		t.Fatalf("status = %s", status)
	}
}

func TestSearchDoesNotCacheEmptyResults(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	var calls int32
	client := stubClient(t, func(*http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return stubResponse(500, `server error`), nil
		}
		return stubResponse(200, `{"results":[{"title":"recovered","url":"https://example.com/r","snippet":"s","summary":"m"}]}`), nil
	})
	first, firstStatus := client.Search("uncached empty query", 5)
	if len(first) != 0 || !strings.Contains(firstStatus, "deepseek:empty") {
		t.Fatalf("first = %+v status = %s", first, firstStatus)
	}
	second, secondStatus := client.Search("uncached empty query", 5)
	if len(second) != 1 || second[0].Title != "recovered" {
		t.Fatalf("second call did not retry the provider: %+v status = %s", second, secondStatus)
	}
}

func TestSearchCachesNonEmptyResults(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	var calls int32
	client := stubClient(t, func(*http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return stubResponse(200, `{"results":[{"title":"cached","url":"https://example.com/c","snippet":"s","summary":"m"}]}`), nil
	})
	client.Search("cached query", 5)
	client.Search("cached query", 5)
	if calls != 1 {
		t.Fatalf("provider called %d times, want 1", calls)
	}
}

func TestSearchWithContentKeepsProviderText(t *testing.T) {
	withDeepSeekKey(t, "test-key")
	var downloads int32
	client := stubClient(t, func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.URL.Host, "deepseek") {
			return stubResponse(200, `{"results":[
				{"title":"has text","url":"https://example.com/text","snippet":"s","summary":"provider summary"},
				{"title":"no text","url":"https://example.com/plain","snippet":"s","summary":""}]}`), nil
		}
		atomic.AddInt32(&downloads, 1)
		return stubResponse(200, `<html><body><p>`+strings.Repeat("page body ", 40)+`</p></body></html>`), nil
	})
	results, _ := client.SearchWithContent("content query", 5)
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Text != "provider summary" {
		t.Fatalf("provider text overwritten: %q", results[0].Text)
	}
	if results[1].Text == "" {
		t.Fatalf("missing text was not downloaded")
	}
	if downloads != 1 {
		t.Fatalf("downloads = %d, want 1", downloads)
	}
}

func TestResultKey(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"https://example.com/a?b=1#x", "example.com/a", true},
		{"http://example.com/a", "example.com/a", true},
		{"https://example.com/nested/https:/other.com/x", "", false},
		{"https:///nohost", "", false},
		{"ftp://example.com/a", "", false},
		{"", "", false},
	}
	for _, test := range cases {
		key, ok := resultKey(test.raw)
		if ok != test.ok || key != test.want {
			t.Errorf("%q: got (%q,%v), want (%q,%v)", test.raw, key, ok, test.want, test.ok)
		}
	}
}
