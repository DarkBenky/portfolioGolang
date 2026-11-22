package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

type Client struct {
	http     *http.Client
	fast     *http.Client
	cacheMu  sync.Mutex
	cache    map[string]cacheEntry
	maxBytes int64
}

type cacheEntry struct {
	body    []byte
	status  int
	expires time.Time
}

func NewClient() *Client {
	return &Client{
		http:     &http.Client{Timeout: 20 * time.Second},
		fast:     &http.Client{Timeout: 8 * time.Second},
		cache:    map[string]cacheEntry{},
		maxBytes: 4 << 20,
	}
}

func (c *Client) getJSON(url string, ttl time.Duration, out any) (int, error) {
	body, status, err := c.get(url, ttl)
	if err != nil {
		return status, err
	}
	if status != http.StatusOK {
		return status, fmt.Errorf("http %d", status)
	}
	return status, json.Unmarshal(body, out)
}

func (c *Client) get(url string, ttl time.Duration) ([]byte, int, error) {
	if ttl > 0 {
		c.cacheMu.Lock()
		entry, ok := c.cache[url]
		c.cacheMu.Unlock()
		if ok && time.Now().Before(entry.expires) {
			return entry.body, entry.status, nil
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json,text/html;q=0.9,*/*;q=0.8")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if ttl > 0 && resp.StatusCode == http.StatusOK {
		c.cacheMu.Lock()
		if len(c.cache) > 512 {
			c.cache = map[string]cacheEntry{}
		}
		c.cache[url] = cacheEntry{body: body, status: resp.StatusCode, expires: time.Now().Add(ttl)}
		c.cacheMu.Unlock()
	}
	return body, resp.StatusCode, nil
}

func (c *Client) getText(url string, ttl time.Duration) (string, error) {
	body, status, err := c.get(url, ttl)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("http %d", status)
	}
	return string(body), nil
}

func newRequest(target string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json,text/html;q=0.9,*/*;q=0.8")
	return req, nil
}

// getWithCookie is needed for Yahoo quoteSummary, which rejects requests without the
// session cookie and matching crumb.
func (c *Client) getWithCookie(target string) ([]byte, error) {
	req, err := newRequest(target)
	if err != nil {
		return nil, err
	}
	if cookie := cookieHeader(); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	return body, nil
}
