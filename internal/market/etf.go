package market

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type ETFHolding struct {
	Name       string  `json:"name"`
	Ticker     string  `json:"ticker"`
	ISIN       string  `json:"isin"`
	Exchange   string  `json:"exchange"`
	Sector     string  `json:"sector"`
	Region     string  `json:"region"`
	Percentage float64 `json:"percentage"`
}

type ETFData struct {
	Holdings []ETFHolding       `json:"holdings"`
	Sectors  map[string]float64 `json:"sectors"`
	Regions  map[string]float64 `json:"regions"`
	ISIN     string             `json:"isin"`
	Ticker   string             `json:"ticker"`
	TER      string             `json:"ter"`
	Policy   string             `json:"distribution_policy"`
}

var percentPattern = regexp.MustCompile(`([0-9]+(?:[.,][0-9]+)?)\s*%`)

// ETFComposition reads the public JustETF profile. The page ships the top holdings
// plus the sector and country splits, which is everything the allocation view needs.
func (c *Client) ETFComposition(ticker string, isin string, name string) (*ETFData, error) {
	data := &ETFData{Ticker: ticker, ISIN: isin, Sectors: map[string]float64{}, Regions: map[string]float64{}}
	if isin == "" || isin == "N/A" {
		return data, nil
	}
	page, err := c.getText("https://www.justetf.com/en/etf-profile.html?isin="+isin, 12*time.Hour)
	if err != nil {
		return data, err
	}
	parsed, err := parseETFPage(page)
	if err != nil {
		return data, err
	}
	parsed.Ticker = ticker
	parsed.ISIN = isin
	return parsed, nil
}

// parseETFPage extracts the composition from a JustETF profile document.
func parseETFPage(page string) (*ETFData, error) {
	data := &ETFData{Sectors: map[string]float64{}, Regions: map[string]float64{}}
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return data, err
	}

	for _, row := range rowsWithTestID(doc, "etf-holdings_top-holdings_row") {
		name := row.values["tl_etf-holdings_top-holdings_value_name"]
		if name == "" {
			name = row.values["tl_etf-holdings_top-holdings_link_name"]
		}
		holding := ETFHolding{Name: name}
		holding.Percentage = parsePercent(row.values["tl_etf-holdings_top-holdings_value_percentage"])
		holding.ISIN = profileISIN(row.href)
		if holding.Name == "" {
			continue
		}
		data.Holdings = append(data.Holdings, holding)
	}
	for _, row := range rowsWithTestID(doc, "etf-holdings_sectors_row") {
		if label := row.values["tl_etf-holdings_sectors_value_name"]; label != "" {
			data.Sectors[label] = parsePercent(row.values["tl_etf-holdings_sectors_value_percentage"])
		}
	}
	for _, row := range rowsWithTestID(doc, "etf-holdings_countries_row") {
		if label := row.values["tl_etf-holdings_countries_value_name"]; label != "" {
			data.Regions[label] = parsePercent(row.values["tl_etf-holdings_countries_value_percentage"])
		}
	}

	text := textOf(doc)
	if index := strings.Index(strings.ToLower(text), "total expense ratio"); index >= 0 {
		window := text[index:]
		if len(window) > 200 {
			window = window[:200]
		}
		if match := percentPattern.FindStringSubmatch(window); len(match) > 1 {
			data.TER = match[1] + "%"
		}
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "distribution policy") && strings.Contains(lower, "accumulating"):
		data.Policy = "Accumulating"
	case strings.Contains(lower, "distribution policy") && strings.Contains(lower, "distributing"):
		data.Policy = "Distributing"
	}
	return data, nil
}

type testIDRow struct {
	values map[string]string
	href   string
}

func rowsWithTestID(root *html.Node, testID string) []testIDRow {
	var rows []testIDRow
	var visit func(node *html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && attr(node, "data-testid") == testID {
			rows = append(rows, testIDRow{values: collectTestIDs(node), href: firstLink(node)})
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return rows
}

func collectTestIDs(node *html.Node) map[string]string {
	values := map[string]string{}
	var visit func(current *html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.ElementNode {
			if id := attr(current, "data-testid"); strings.HasPrefix(id, "tl_") {
				values[id] = strings.TrimSpace(textOf(current))
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return values
}

func firstLink(node *html.Node) string {
	var href string
	var visit func(current *html.Node) bool
	visit = func(current *html.Node) bool {
		if current.Type == html.ElementNode && current.Data == "a" {
			href = attr(current, "href")
			return true
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if visit(child) {
				return true
			}
		}
		return false
	}
	visit(node)
	return href
}

func profileISIN(href string) string {
	const marker = "/stock-profiles/"
	if index := strings.Index(href, marker); index >= 0 {
		return strings.TrimSpace(href[index+len(marker):])
	}
	return ""
}

func attr(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func textOf(node *html.Node) string {
	var sb strings.Builder
	var visit func(current *html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			sb.WriteString(current.Data)
			sb.WriteString(" ")
		}
		if current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style") {
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return sb.String()
}

func parsePercent(value string) float64 {
	match := percentPattern.FindStringSubmatch(value)
	if len(match) < 2 {
		return 0
	}
	number, err := strconv.ParseFloat(strings.ReplaceAll(strings.ReplaceAll(match[1], ",", "."), " ", ""), 64)
	if err != nil {
		return 0
	}
	return number
}
