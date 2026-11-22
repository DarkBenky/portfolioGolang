package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portfolio/internal/bills"
	"portfolio/internal/chat"
	"portfolio/internal/config"
	"portfolio/internal/jobs"
	"portfolio/internal/llm"
	"portfolio/internal/market"
	"portfolio/internal/prices"
	"portfolio/internal/rag"
	"portfolio/internal/store"
)

type harness struct {
	server *httptest.Server
	token  string
}

// newHarness wires the real handlers against temporary databases with background
// fetch work disabled, so the integration test never touches the network.
func newHarness(t testing.TB) *harness {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := db.EnsurePinsTable(); err != nil {
		t.Fatalf("pins: %v", err)
	}
	if err := db.CreateSituationTables(); err != nil {
		t.Fatalf("situation: %v", err)
	}
	if err := bills.InitBillDB(db.DB); err != nil {
		t.Fatalf("bills: %v", err)
	}
	priceStore, err := prices.Init(filepath.Join(dir, "prices.duckdb"), db.DB, 500)
	if err != nil {
		t.Fatalf("prices: %v", err)
	}
	marketClient := market.NewClient()
	llmClient := llm.New("", "test-chat", "test-cheap", "", db, 0)
	indexer := rag.New(db, llmClient, marketClient, false)
	chatService := chat.New(llmClient, db, marketClient, priceStore, indexer)
	manager := jobs.New(db, priceStore, marketClient, llmClient, indexer, jobs.Settings{
		Retention5mDays: 14, Retention1hDays: 120, Lookback5mDays: 14, SummaryMaxChars: 1000,
	})
	cfg := config.Config{Salt: "test-salt", JWTSecret: "test-secret", JWTExpiry: time.Hour,
		ChatModel: "test-chat", CheapModel: "test-cheap"}
	apiServer := New(cfg, db, priceStore, marketClient, llmClient, indexer, chatService, manager, nil)
	apiServer.Background = false
	httpServer := httptest.NewServer(apiServer.Router())

	t.Cleanup(func() {
		httpServer.Close()
		priceStore.Close()
		db.Close()
	})
	return &harness{server: httpServer}
}

func (h *harness) do(t testing.TB, method string, path string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return h.send(t, request)
}

func (h *harness) send(t testing.TB, request *http.Request) (int, []byte) {
	t.Helper()
	if h.token != "" {
		request.Header.Set("Authorization", "Bearer "+h.token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, raw
}

func (h *harness) register(t testing.TB, email string, password string) {
	t.Helper()
	status, _ := h.do(t, http.MethodPost, "/register", map[string]string{
		"user_name": "Tester", "email": email, "password": password,
	})
	if status != http.StatusOK {
		t.Fatalf("register returned %d", status)
	}
	status, body := h.do(t, http.MethodPost, "/login", map[string]string{"email": email, "password": password})
	if status != http.StatusOK {
		t.Fatalf("login returned %d: %s", status, body)
	}
	var response struct{ Token string }
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	h.token = response.Token
}

func (h *harness) addHolding(t testing.TB, ticker string, quantity string, price string) {
	t.Helper()
	form := &bytes.Buffer{}
	writer := multipart.NewWriter(form)
	fields := map[string]string{
		"Name": ticker + " Inc", "Ticker": ticker, "ISIN": "", "Exchange": "NASDAQ",
		"Quantity": quantity, "PurchasePrice": price, "TER": "0", "Policy": "N/A",
		"Currency": "USD", "ETF": "false",
	}
	for key, value := range fields {
		writer.WriteField(key, value)
	}
	writer.Close()
	request, err := http.NewRequest(http.MethodPost, h.server.URL+"/api/asset/holdings", form)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	status, body := h.send(t, request)
	if status != http.StatusOK {
		t.Fatalf("add holding returned %d: %s", status, body)
	}
}

func TestPublicHealth(t *testing.T) {
	h := newHarness(t)
	status, body := h.do(t, http.MethodGet, "/health", nil)
	if status != http.StatusOK || !strings.Contains(string(body), "healthy") {
		t.Fatalf("health returned %d %s", status, body)
	}
}

func TestProtectedRoutesRequireToken(t *testing.T) {
	h := newHarness(t)
	status, _ := h.do(t, http.MethodGet, "/api/portfolio/holdings", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
}

func TestHoldingLifecycle(t *testing.T) {
	h := newHarness(t)
	h.register(t, "lifecycle@example.com", "password123")

	status, body := h.do(t, http.MethodGet, "/api/portfolio/holdings", nil)
	if status != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("expected an empty list, got %d %s", status, body)
	}

	h.addHolding(t, "AAPL", "10", "180")

	status, body = h.do(t, http.MethodGet, "/api/portfolio/holdings", nil)
	if status != http.StatusOK {
		t.Fatalf("holdings returned %d", status)
	}
	var holdings []holdingDetail
	if err := json.Unmarshal(body, &holdings); err != nil {
		t.Fatalf("decode holdings: %v", err)
	}
	if len(holdings) != 1 || holdings[0].Ticker != "AAPL" || holdings[0].Quantity != 10 {
		t.Fatalf("unexpected holdings %+v", holdings)
	}

	status, body = h.do(t, http.MethodGet, "/api/portfolio/value", nil)
	if status != http.StatusOK {
		t.Fatalf("value returned %d", status)
	}
	var value map[string]float64
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	if value["total_value"] != 1800 {
		t.Errorf("expected the purchase price to be used when no bars exist, got %v", value["total_value"])
	}

	status, body = h.do(t, http.MethodGet, "/api/portfolio/history?interval=1d", nil)
	if status != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("expected an empty history, got %d %s", status, body)
	}

	status, _ = h.do(t, http.MethodGet, "/api/portfolio/allocation", nil)
	if status != http.StatusOK {
		t.Errorf("allocation returned %d", status)
	}
	status, _ = h.do(t, http.MethodGet, "/api/portfolio/stats", nil)
	if status != http.StatusOK {
		t.Errorf("stats returned %d", status)
	}
}

func TestExpensesAndPins(t *testing.T) {
	h := newHarness(t)
	h.register(t, "money@example.com", "password123")

	status, _ := h.do(t, http.MethodPost, "/api/expenses", map[string]any{
		"description": "Lunch", "amount": 12.5, "category": "Dining", "date": "2026-01-15",
	})
	if status != http.StatusCreated {
		t.Fatalf("add expense returned %d", status)
	}
	status, body := h.do(t, http.MethodGet, "/api/expenses/stats/category", nil)
	var categories map[string]float64
	json.Unmarshal(body, &categories)
	if status != http.StatusOK || categories["Dining"] != 12.5 {
		t.Errorf("unexpected category stats %d %s", status, body)
	}
	status, body = h.do(t, http.MethodGet, "/api/expenses", nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Lunch") {
		t.Errorf("unexpected expenses %d %s", status, body)
	}

	status, _ = h.do(t, http.MethodPost, "/api/pins", map[string]string{"symbol": "nvda", "name": "Nvidia"})
	if status != http.StatusOK {
		t.Fatalf("add pin returned %d", status)
	}
	status, body = h.do(t, http.MethodGet, "/api/pins", nil)
	if status != http.StatusOK || !strings.Contains(string(body), "NVDA") {
		t.Errorf("pins must be upper cased: %d %s", status, body)
	}
	status, _ = h.do(t, http.MethodDelete, "/api/pins?symbol=NVDA", nil)
	if status != http.StatusOK {
		t.Fatalf("remove pin returned %d", status)
	}
	status, body = h.do(t, http.MethodGet, "/api/pins", nil)
	if strings.Contains(string(body), "NVDA") {
		t.Errorf("pin was not removed: %s", body)
	}
}

func TestSituationTaskLifecycle(t *testing.T) {
	h := newHarness(t)
	h.register(t, "situation@example.com", "password123")

	status, body := h.do(t, http.MethodPost, "/api/situation/tasks", map[string]any{
		"subject": "Semiconductor supply chain", "sub_topics": []string{"foundries"}, "daily_hour": 9,
	})
	if status != http.StatusOK {
		t.Fatalf("create task returned %d: %s", status, body)
	}
	var task store.SituationTask
	json.Unmarshal(body, &task)

	status, body = h.do(t, http.MethodGet, "/api/situation/tasks", nil)
	if status != http.StatusOK || !strings.Contains(string(body), task.ID) {
		t.Fatalf("task missing from list: %d %s", status, body)
	}
	status, _ = h.do(t, http.MethodPost, "/api/situation/tasks/"+task.ID+"/pause", nil)
	if status != http.StatusOK {
		t.Fatalf("pause returned %d", status)
	}
	status, _ = h.do(t, http.MethodDelete, "/api/situation/tasks/"+task.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("delete returned %d", status)
	}
	status, body = h.do(t, http.MethodGet, "/api/situation/tasks", nil)
	if strings.Contains(string(body), task.ID) {
		t.Errorf("soft deleted task still listed: %s", body)
	}
}

func TestUsageEndpoint(t *testing.T) {
	h := newHarness(t)
	h.register(t, "usage@example.com", "password123")
	status, body := h.do(t, http.MethodGet, "/api/usage", nil)
	if status != http.StatusOK {
		t.Fatalf("usage returned %d", status)
	}
	var usage map[string]any
	if err := json.Unmarshal(body, &usage); err != nil {
		t.Fatalf("decode usage: %v", err)
	}
	if _, ok := usage["chat_model"]; !ok {
		t.Errorf("usage must report the model in use: %s", body)
	}
}

func BenchmarkGetHoldings(b *testing.B) {
	h := newHarness(b)
	h.register(b, "bench@example.com", "password123")
	for i := 0; i < 20; i++ {
		h.addHolding(b, fmt.Sprintf("T%02d", i), "10", "100")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		status, _ := h.do(b, http.MethodGet, "/api/portfolio/holdings", nil)
		if status != http.StatusOK {
			b.Fatalf("status %d", status)
		}
	}
}
