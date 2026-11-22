package market

import (
	"strings"
	"testing"
)

func TestScoreSentimentPositive(t *testing.T) {
	score := ScoreSentiment("Apple beats estimates as profit surges", "Strong growth and record revenue", "")
	if score <= 0.2 {
		t.Errorf("expected clearly positive sentiment, got %v", score)
	}
}

func TestScoreSentimentNegative(t *testing.T) {
	score := ScoreSentiment("Company plunges after profit warning", "Lawsuit and investigation announced", "")
	if score >= -0.2 {
		t.Errorf("expected clearly negative sentiment, got %v", score)
	}
}

func TestScoreSentimentNeutral(t *testing.T) {
	if score := ScoreSentiment("Company hires a new office manager", "", ""); score != 0 {
		t.Errorf("expected neutral, got %v", score)
	}
}

func TestScoreSentimentNegation(t *testing.T) {
	positive := ScoreSentiment("growth", "", "")
	negated := ScoreSentiment("no growth", "", "")
	if negated >= positive {
		t.Errorf("negation must reduce the score: %v vs %v", negated, positive)
	}
}

func TestParsePercent(t *testing.T) {
	cases := map[string]float64{"5.01%": 5.01, "24,73%": 24.73, "": 0, "n/a": 0}
	for input, expected := range cases {
		if actual := parsePercent(input); actual != expected {
			t.Errorf("%q: expected %v, got %v", input, expected, actual)
		}
	}
}

const etfFixture = `<html><body>
<div data-testid="etf-holdings_top-holdings_container"><table><tbody>
<tr data-testid="etf-holdings_top-holdings_row">
  <td><a data-testid="tl_etf-holdings_top-holdings_link_name" href="/en/stock-profiles/US67066G1040" title="NVIDIA Corp."><span>NVIDIA Corp.</span></a></td>
  <td><span data-testid="tl_etf-holdings_top-holdings_value_percentage">5.01%</span></td>
</tr>
<tr data-testid="etf-holdings_top-holdings_row">
  <td><a data-testid="tl_etf-holdings_top-holdings_link_name" href="/en/stock-profiles/US0378331005" title="Apple"><span>Apple</span></a></td>
  <td><span data-testid="tl_etf-holdings_top-holdings_value_percentage">4.52%</span></td>
</tr>
</tbody></table></div>
<table><tbody>
<tr data-testid="etf-holdings_sectors_row">
  <td data-testid="tl_etf-holdings_sectors_value_name">Technology</td>
  <td><span data-testid="tl_etf-holdings_sectors_value_percentage">38.27%</span></td>
</tr>
<tr data-testid="etf-holdings_countries_row">
  <td data-testid="tl_etf-holdings_countries_value_name">United States</td>
  <td><span data-testid="tl_etf-holdings_countries_value_percentage">61.10%</span></td>
</tr>
</tbody></table>
<p>Total expense ratio 0.22%</p>
<p>Distribution policy Accumulating</p>
</body></html>`

func TestParseETFPage(t *testing.T) {
	holdings, err := parseETFPage(etfFixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(holdings.Holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(holdings.Holdings))
	}
	if holdings.Holdings[0].Name != "NVIDIA Corp." || holdings.Holdings[0].ISIN != "US67066G1040" {
		t.Errorf("unexpected first holding %+v", holdings.Holdings[0])
	}
	if holdings.Holdings[0].Percentage != 5.01 {
		t.Errorf("expected 5.01, got %v", holdings.Holdings[0].Percentage)
	}
	if holdings.Sectors["Technology"] != 38.27 {
		t.Errorf("unexpected sector split %+v", holdings.Sectors)
	}
	if holdings.Regions["United States"] != 61.10 {
		t.Errorf("unexpected region split %+v", holdings.Regions)
	}
	if holdings.TER != "0.22%" {
		t.Errorf("unexpected TER %q", holdings.TER)
	}
	if holdings.Policy != "Accumulating" {
		t.Errorf("unexpected policy %q", holdings.Policy)
	}
}

func TestISINDetection(t *testing.T) {
	if !isISIN("US0378331005") {
		t.Error("valid ISIN rejected")
	}
	if isISIN("AAPL") {
		t.Error("ticker accepted as ISIN")
	}
	if !strings.HasPrefix("https://example.com", "https") {
		t.Error("sanity")
	}
}

func BenchmarkScoreSentiment(b *testing.B) {
	title := "Apple beats estimates as revenue surges to a record"
	summary := "Strong growth in services and record profit."
	text := strings.Repeat("The company reported healthy demand and strong growth. ", 40)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if ScoreSentiment(title, summary, text) == 0 {
			b.Fatal("expected a non zero score")
		}
	}
}
