package market

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type openFigiRequest struct {
	IDType  string `json:"idType"`
	IDValue string `json:"idValue"`
}

type openFigiResult struct {
	Data []struct {
		Ticker string `json:"ticker"`
		ISIN   string `json:"isin"`
		Name   string `json:"name"`
	} `json:"data"`
}

func (c *Client) figiLookup(idType string, idValue string) (string, string) {
	payload, err := json.Marshal([]openFigiRequest{{IDType: idType, IDValue: idValue}})
	if err != nil {
		return "", ""
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.openfigi.com/v3/mapping", bytes.NewReader(payload))
	if err != nil {
		return "", ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", ""
	}
	var results []openFigiResult
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 || len(results[0].Data) == 0 {
		return "", ""
	}
	return results[0].Data[0].Ticker, results[0].Data[0].ISIN
}

func (c *Client) TickerToISIN(ticker string) string {
	if ticker == "" || ticker == "N/A" {
		return ""
	}
	_, isin := c.figiLookup("TICKER", strings.ToUpper(ticker))
	return isin
}

func (c *Client) ISINToTicker(isin string) string {
	if isin == "" || isin == "N/A" {
		return ""
	}
	ticker, _ := c.figiLookup("ID_ISIN", strings.ToUpper(isin))
	return ticker
}

func (c *Client) ResolveTicker(identifier string) string {
	identifier = strings.TrimSpace(strings.TrimPrefix(identifier, "$"))
	if identifier == "" {
		return ""
	}
	if len(identifier) == 12 && isISIN(identifier) {
		if ticker := c.ISINToTicker(identifier); ticker != "" {
			return ticker
		}
	}
	return identifier
}

func isISIN(value string) bool {
	if len(value) != 12 {
		return false
	}
	for i, r := range value {
		if i < 2 {
			if r < 'A' || r > 'Z' {
				return false
			}
			continue
		}
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func (c *Client) CurrencyConversion(amount float64, from string, to string) (float64, error) {
	if strings.EqualFold(from, to) {
		return amount, nil
	}
	rate, err := c.Rate(strings.ToUpper(from), strings.ToUpper(to))
	if err != nil {
		return 0, fmt.Errorf("rate %s/%s: %w", from, to, err)
	}
	return amount * rate, nil
}
