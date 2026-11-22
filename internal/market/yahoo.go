package market

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Quote struct {
	Ticker             string  `json:"ticker"`
	Name               string  `json:"name"`
	Exchange           string  `json:"exchange"`
	ISIN               string  `json:"isin"`
	Currency           string  `json:"currency"`
	Type               string  `json:"type"`
	Price              float64 `json:"price"`
	TER                string  `json:"ter,omitempty"`
	DistributionPolicy string  `json:"distribution_policy,omitempty"`
}

type ChartBar struct {
	Timestamp int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    int64
}

type AssetDetails struct {
	Ticker        string  `json:"ticker"`
	ISIN          string  `json:"isin"`
	MarketCap     string  `json:"market_cap"`
	MarketCapEur  string  `json:"market_cap_eur"`
	Country       string  `json:"country"`
	Sector        string  `json:"sector"`
	Eps           float64 `json:"eps"`
	PbRatio       float64 `json:"pb_ratio"`
	PeRatio       float64 `json:"pe_ratio"`
	DividendYield float64 `json:"dividend_yield"`
	Revenue       string  `json:"revenue"`
	NetIncome     string  `json:"net_income"`
	ProfitMargin  float64 `json:"profit_margin"`
}

type NewsItem struct {
	Ticker      string
	Title       string
	Summary     string
	Text        string
	URL         string
	PublishedAt int64
	Author      string
	ImageURL    string
	Sentiment   float64
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency         string `json:"currency"`
				Symbol           string `json:"symbol"`
				FullExchangeName string `json:"fullExchangeName"`
				ExchangeName     string `json:"exchangeName"`
				InstrumentType   string `json:"instrumentType"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*float64 `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type yahooSearchResponse struct {
	Quotes []struct {
		Symbol    string  `json:"symbol"`
		ShortName string  `json:"shortname"`
		LongName  string  `json:"longname"`
		Exchange  string  `json:"exchange"`
		QuoteType string  `json:"quoteType"`
		Price     float64 `json:"regularMarketPrice"`
		ISIN      string  `json:"isin"`
	} `json:"quotes"`
	News []struct {
		Title               string `json:"title"`
		Publisher           string `json:"publisher"`
		ProviderPublishTime int64  `json:"providerPublishTime"`
		Summary             string `json:"summary"`
		Link                string `json:"link"`
		Content             struct {
			Title    string `json:"title"`
			Summary  string `json:"summary"`
			PubDate  string `json:"pubDate"`
			Provider struct {
				DisplayName string `json:"displayName"`
			} `json:"provider"`
			CanonicalURL struct {
				URL string `json:"url"`
			} `json:"canonicalUrl"`
			Thumbnail struct {
				OriginalURL string `json:"originalUrl"`
			} `json:"thumbnail"`
		} `json:"content"`
	} `json:"news"`
}

type yahooQuoteSummary struct {
	QuoteSummary struct {
		Result []struct {
			Price struct {
				MarketCap struct {
					Raw float64 `json:"raw"`
				} `json:"marketCap"`
				Currency string `json:"currency"`
			} `json:"price"`
			SummaryDetail struct {
				DividendYield struct {
					Raw float64 `json:"raw"`
				} `json:"dividendYield"`
				TrailingPE struct {
					Raw float64 `json:"raw"`
				} `json:"trailingPE"`
			} `json:"summaryDetail"`
			DefaultKeyStatistics struct {
				TrailingEps struct {
					Raw float64 `json:"raw"`
				} `json:"trailingEps"`
				PriceToBook struct {
					Raw float64 `json:"raw"`
				} `json:"priceToBook"`
			} `json:"defaultKeyStatistics"`
			FinancialData struct {
				ProfitMargins struct {
					Raw float64 `json:"raw"`
				} `json:"profitMargins"`
				TotalRevenue struct {
					Raw float64 `json:"raw"`
				} `json:"totalRevenue"`
				NetIncome struct {
					Raw float64 `json:"raw"`
				} `json:"netIncomeToCommon"`
			} `json:"financialData"`
			AssetProfile struct {
				Country string `json:"country"`
				Sector  string `json:"sector"`
			} `json:"assetProfile"`
		} `json:"result"`
	} `json:"quoteSummary"`
}

var (
	crumbMu     sync.Mutex
	crumbValue  string
	crumbExpiry time.Time
	cookieJar   = map[string]string{}
	cookieMu    sync.Mutex
)

// crumb returns the token Yahoo requires for quoteSummary. Without it the endpoint
// answers 401 Invalid Crumb for every request.
func (c *Client) crumb() string {
	crumbMu.Lock()
	defer crumbMu.Unlock()
	if crumbValue != "" && time.Now().Before(crumbExpiry) {
		return crumbValue
	}
	req, err := newRequest("https://fc.yahoo.com")
	if err != nil {
		return ""
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	cookieMu.Lock()
	for _, cookie := range resp.Cookies() {
		cookieJar[cookie.Name] = cookie.Value
	}
	cookieMu.Unlock()
	resp.Body.Close()

	req, err = newRequest("https://query1.finance.yahoo.com/v1/test/getcrumb")
	if err != nil {
		return ""
	}
	req.Header.Set("Cookie", cookieHeader())
	resp, err = c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 128)
	n, _ := resp.Body.Read(buf)
	sb.Write(buf[:n])
	crumbValue = strings.TrimSpace(sb.String())
	crumbExpiry = time.Now().Add(30 * time.Minute)
	return crumbValue
}

func cookieHeader() string {
	cookieMu.Lock()
	defer cookieMu.Unlock()
	parts := make([]string, 0, len(cookieJar))
	for name, value := range cookieJar {
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

func (c *Client) Chart(ticker string, period1, period2 int64, interval string) ([]ChartBar, string, error) {
	target := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=%s&includePrePost=false&events=div%%2Csplit",
		url.PathEscape(ticker), period1, period2, interval)
	var payload yahooChartResponse
	status, err := c.getJSON(target, 0, &payload)
	if err != nil {
		return nil, "", err
	}
	if status != 200 {
		return nil, "", fmt.Errorf("chart http %d", status)
	}
	if payload.Chart.Error != nil {
		return nil, "", fmt.Errorf("%s: %s", payload.Chart.Error.Code, payload.Chart.Error.Description)
	}
	if len(payload.Chart.Result) == 0 {
		return nil, "", nil
	}
	result := payload.Chart.Result[0]
	quote := result.Indicators.Quote
	bars := make([]ChartBar, 0, len(result.Timestamp))
	if len(quote) == 0 {
		return bars, result.Meta.Currency, nil
	}
	source := quote[0]
	for i, ts := range result.Timestamp {
		if ts <= 0 || i >= len(source.Close) || source.Close[i] == nil {
			continue
		}
		open, high, low, closePrice := 0.0, 0.0, 0.0, *source.Close[i]
		if i < len(source.Open) && source.Open[i] != nil {
			open = *source.Open[i]
		}
		if i < len(source.High) && source.High[i] != nil {
			high = *source.High[i]
		}
		if i < len(source.Low) && source.Low[i] != nil {
			low = *source.Low[i]
		}
		volume := int64(0)
		if i < len(source.Volume) && source.Volume[i] != nil {
			volume = int64(*source.Volume[i])
		}
		bars = append(bars, ChartBar{Timestamp: ts, Open: open, High: high, Low: low, Close: closePrice, Volume: volume})
	}
	return bars, result.Meta.Currency, nil
}

func (c *Client) TickerSearch(query string, maxResults int) ([]Quote, error) {
	target := "https://query2.finance.yahoo.com/v1/finance/search?q=" + url.QueryEscape(query) +
		"&quotesCount=" + strconv.Itoa(maxResults) + "&newsCount=0&listsCount=0&enableFuzzyQuery=true"
	var payload yahooSearchResponse
	if _, err := c.getJSON(target, 10*time.Minute, &payload); err != nil {
		return nil, err
	}
	quotes := make([]Quote, 0, len(payload.Quotes))
	for _, item := range payload.Quotes {
		switch item.QuoteType {
		case "EQUITY", "ETF", "MUTUALFUND", "CRYPTOCURRENCY", "INDEX":
		default:
			continue
		}
		name := item.LongName
		if name == "" {
			name = item.ShortName
		}
		quotes = append(quotes, Quote{
			Ticker:   item.Symbol,
			Name:     name,
			Exchange: item.Exchange,
			ISIN:     item.ISIN,
			Type:     item.QuoteType,
			Price:    item.Price,
		})
	}
	return quotes, nil
}

func (c *Client) Details(ticker string, isin string) (*AssetDetails, error) {
	target := fmt.Sprintf("https://query1.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=price,summaryDetail,defaultKeyStatistics,financialData,assetProfile",
		url.PathEscape(ticker))
	if crumb := c.crumb(); crumb != "" {
		target += "&crumb=" + url.QueryEscape(crumb)
	}
	body, err := c.getWithCookie(target)
	if err != nil {
		return nil, err
	}
	var payload yahooQuoteSummary
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if len(payload.QuoteSummary.Result) == 0 {
		return nil, fmt.Errorf("no summary for %s", ticker)
	}
	result := payload.QuoteSummary.Result[0]
	details := &AssetDetails{
		Ticker:        ticker,
		ISIN:          isin,
		MarketCap:     formatMoney(result.Price.MarketCap.Raw, result.Price.Currency),
		Country:       result.AssetProfile.Country,
		Sector:        result.AssetProfile.Sector,
		Eps:           result.DefaultKeyStatistics.TrailingEps.Raw,
		PbRatio:       result.DefaultKeyStatistics.PriceToBook.Raw,
		PeRatio:       result.SummaryDetail.TrailingPE.Raw,
		DividendYield: result.SummaryDetail.DividendYield.Raw,
		Revenue:       formatMoney(result.FinancialData.TotalRevenue.Raw, result.Price.Currency),
		NetIncome:     formatMoney(result.FinancialData.NetIncome.Raw, result.Price.Currency),
		ProfitMargin:  result.FinancialData.ProfitMargins.Raw,
	}
	if result.Price.Currency != "EUR" && result.Price.MarketCap.Raw > 0 {
		if rate, err := c.Rate(result.Price.Currency, "EUR"); err == nil && rate > 0 {
			details.MarketCapEur = formatMoney(result.Price.MarketCap.Raw*rate, "EUR")
		}
	}
	return details, nil
}

type rateEntry struct {
	value   float64
	expires time.Time
}

var (
	rateMu    sync.Mutex
	rateCache = map[string]rateEntry{}
)

func (c *Client) Rate(from string, to string) (float64, error) {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == "" || from == to {
		return 1, nil
	}
	key := from + "/" + to
	rateMu.Lock()
	entry, ok := rateCache[key]
	rateMu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.value, nil
	}
	bars, _, err := c.Chart(from+to+"=X", time.Now().Add(-48*time.Hour).Unix(), time.Now().Unix(), "1h")
	if err != nil {
		return 0, err
	}
	if len(bars) == 0 {
		return 0, fmt.Errorf("no fx rate for %s%s", from, to)
	}
	value := bars[len(bars)-1].Close
	rateMu.Lock()
	rateCache[key] = rateEntry{value: value, expires: time.Now().Add(10 * time.Minute)}
	rateMu.Unlock()
	return value, nil
}

func (c *Client) News(ticker string, limit int) ([]NewsItem, error) {
	target := "https://query2.finance.yahoo.com/v1/finance/search?q=" + url.QueryEscape(ticker) +
		"&quotesCount=0&newsCount=" + strconv.Itoa(max(limit*2, 10)) + "&listsCount=0"
	var payload yahooSearchResponse
	if _, err := c.getJSON(target, 15*time.Minute, &payload); err != nil {
		return nil, err
	}
	items := make([]NewsItem, 0, len(payload.News))
	for _, entry := range payload.News {
		title := entry.Content.Title
		if title == "" {
			title = entry.Title
		}
		if title == "" {
			continue
		}
		summary := entry.Content.Summary
		if summary == "" {
			summary = entry.Summary
		}
		link := entry.Content.CanonicalURL.URL
		if link == "" {
			link = entry.Link
		}
		published := entry.ProviderPublishTime
		if published == 0 && entry.Content.PubDate != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, entry.Content.PubDate); err == nil {
				published = parsed.Unix()
			}
		}
		author := entry.Content.Provider.DisplayName
		if author == "" {
			author = entry.Publisher
		}
		items = append(items, NewsItem{
			Ticker:      ticker,
			Title:       title,
			Summary:     summary,
			URL:         link,
			PublishedAt: published,
			Author:      author,
			ImageURL:    entry.Content.Thumbnail.OriginalURL,
		})
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func formatMoney(value float64, currency string) string {
	if value == 0 {
		return ""
	}
	symbol := currency
	if symbol == "" {
		symbol = "USD"
	}
	switch {
	case value >= 1e12:
		return fmt.Sprintf("%.2fT %s", value/1e12, symbol)
	case value >= 1e9:
		return fmt.Sprintf("%.2fB %s", value/1e9, symbol)
	case value >= 1e6:
		return fmt.Sprintf("%.2fM %s", value/1e6, symbol)
	default:
		return fmt.Sprintf("%.2f %s", value, symbol)
	}
}

func (c *Client) FetchJSON(url string, ttl time.Duration, out any) error {
	_, err := c.getJSON(url, ttl, out)
	return err
}
