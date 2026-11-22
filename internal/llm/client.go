package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"portfolio/internal/store"
)

const (
	chatURL      = "https://openrouter.ai/api/v1/chat/completions"
	embeddingURL = "https://openrouter.ai/api/v1/embeddings"
	modelsURL    = "https://openrouter.ai/api/v1/models"
)

type Client struct {
	apiKey       string
	chatModel    string
	sessionModel string
	cheapModel   string
	embedModel   string
	db           *store.DB
	dailyBudget  float64
	http         *http.Client
	priceMu      sync.Mutex
	prices       map[string][2]float64
	pricesLoaded time.Time
}

func New(apiKey string, chatModel string, sessionModel string, cheapModel string, embedModel string, db *store.DB, dailyBudget float64) *Client {
	if sessionModel == "" {
		sessionModel = chatModel
	}
	return &Client{
		apiKey:       apiKey,
		chatModel:    chatModel,
		sessionModel: sessionModel,
		cheapModel:   cheapModel,
		embedModel:   embedModel,
		db:           db,
		dailyBudget:  dailyBudget,
		http:         &http.Client{Timeout: 180 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c.apiKey != ""
}

// SessionModel is the model used for interactive chat, which is not the same as the
// cheaper model used for summary jobs.
func (c *Client) SessionModel() string {
	return c.Model(KindChat)
}

func (c *Client) Model(kind Kind) string {
	if kind.useCheapModel() {
		return c.cheapModel
	}
	if kind == KindChat {
		return c.sessionModel
	}
	return c.chatModel
}

// Complete is the non streaming path used by summaries and classification. The cache
// key makes repeated identical prompts free, which is the cheapest possible saving.
func (c *Client) Complete(ctx context.Context, kind Kind, userID string, system string, user string, cacheKey string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("llm: OPENROUTER_API_KEY not configured")
	}
	if cacheKey != "" {
		if cached, ok := c.db.CachedLLMResponse(cacheKey); ok {
			return cached, nil
		}
	}
	if err := c.checkBudget(userID); err != nil {
		return "", err
	}

	messages := make([]Message, 0, 2)
	if system != "" {
		messages = append(messages, Message{Role: "system", Content: system})
	}
	messages = append(messages, Message{Role: "user", Content: user})

	completion, err := c.do(ctx, kind, messages, nil, "")
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(completion.Content)
	c.record(userID, kind, completion)
	if cacheKey != "" && text != "" {
		c.db.SaveLLMResponse(cacheKey, completion.Model, text, completion.PromptTokens, completion.CompletionTokens, completion.CostUSD)
	}
	return text, nil
}

// Chat streams tokens to onDelta and accumulates tool calls for the agent loop.
func (c *Client) Chat(ctx context.Context, kind Kind, userID string, messages []Message, tools []Tool, handlers Handlers) (*Completion, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("llm: OPENROUTER_API_KEY not configured")
	}
	if err := c.checkBudget(userID); err != nil {
		return nil, err
	}
	return c.stream(ctx, kind, userID, messages, tools, handlers)
}

func (c *Client) do(ctx context.Context, kind Kind, messages []Message, tools []Tool, toolChoice string) (*Completion, error) {
	body := request{
		Model:      c.Model(kind),
		Messages:   messages,
		Tools:      tools,
		ToolChoice: toolChoice,
		MaxTokens:  kind.maxTokens(),
		Reasoning:  &reasoning{Enabled: false},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		c.headers(req)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("llm: http %d %s", resp.StatusCode, Truncate(string(raw), 200))
			time.Sleep(time.Duration(attempt+1) * 3 * time.Second)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("llm: http %d %s", resp.StatusCode, Truncate(string(raw), 300))
		}
		var parsed response
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		if parsed.Error != nil {
			return nil, fmt.Errorf("llm: %s", parsed.Error.Message)
		}
		if len(parsed.Choices) == 0 {
			return nil, fmt.Errorf("llm: empty response")
		}
		return &Completion{
			Content:          parsed.Choices[0].Message.Content,
			ToolCalls:        parsed.Choices[0].Message.ToolCalls,
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			CostUSD:          c.cost(parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, parsed.Usage.Cost, c.Model(kind)),
			Model:            c.Model(kind),
			FinishReason:     parsed.Choices[0].FinishReason,
		}, nil
	}
	return nil, lastErr
}

func (c *Client) stream(ctx context.Context, kind Kind, userID string, messages []Message, tools []Tool, handlers Handlers) (*Completion, error) {
	body := request{
		Model:     c.Model(kind),
		Messages:  messages,
		Stream:    true,
		Tools:     tools,
		MaxTokens: kind.maxTokens(),
		// Low effort keeps thinking short: this model spends the whole token budget
		// on reasoning at the default level, which truncates the answer.
		Reasoning: &reasoning{Enabled: kind.useReasoning(), Effort: "low"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.headers(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("llm: http %d %s", resp.StatusCode, Truncate(string(raw), 300))
	}

	completion := &Completion{Model: c.Model(kind)}
	accumulated := map[int]*ToolCall{}
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		if data == "" {
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return nil, fmt.Errorf("llm: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			completion.PromptTokens = chunk.Usage.PromptTokens
			completion.CompletionTokens = chunk.Usage.CompletionTokens
			completion.CachedTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			if chunk.Usage.Cost > 0 {
				completion.CostUSD = chunk.Usage.Cost
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if chunk.Choices[0].FinishReason != "" {
			completion.FinishReason = chunk.Choices[0].FinishReason
		}
		if delta.Reasoning != "" {
			completion.Reasoning += delta.Reasoning
			if handlers.OnReasoning != nil {
				handlers.OnReasoning(delta.Reasoning)
			}
		}
		if delta.Content != "" {
			completion.Content += delta.Content
			if handlers.OnDelta != nil {
				handlers.OnDelta(delta.Content)
			}
		}
		for _, call := range delta.ToolCalls {
			entry, ok := accumulated[call.Index]
			if !ok {
				entry = &ToolCall{Type: "function"}
				accumulated[call.Index] = entry
			}
			if call.ID != "" {
				entry.ID = call.ID
			}
			if call.Function.Name != "" {
				entry.Function.Name += call.Function.Name
			}
			if call.Function.Arguments != "" {
				entry.Function.Arguments += call.Function.Arguments
			}
		}
	}

	for i := 0; i < len(accumulated); i++ {
		if call, ok := accumulated[i]; ok {
			completion.ToolCalls = append(completion.ToolCalls, *call)
		}
	}
	if completion.CostUSD == 0 {
		completion.CostUSD = c.cost(completion.PromptTokens, completion.CompletionTokens, 0, completion.Model)
	}
	c.record(userID, kind, completion)
	return completion, nil
}

func (c *Client) headers(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://portfolio.local")
	req.Header.Set("X-Title", "portfolio")
}

func (c *Client) record(userID string, kind Kind, completion *Completion) {
	if c.db == nil || completion == nil {
		return
	}
	if userID == "" {
		userID = "system"
	}
	c.db.RecordLLMUsage(userID, string(kind), completion.Model, completion.PromptTokens, completion.CompletionTokens, completion.CostUSD)
}

func (c *Client) checkBudget(userID string) error {
	if c.dailyBudget <= 0 || c.db == nil {
		return nil
	}
	if userID == "" {
		userID = "system"
	}
	spend, err := c.db.LLMSpendToday(userID)
	if err != nil {
		return nil
	}
	if spend >= c.dailyBudget {
		log.Printf("llm: %s reached the daily budget (%.4f USD)", userID, spend)
		return ErrBudgetExceeded
	}
	return nil
}

// cost falls back to the published per million token prices when the provider does
// not report a charge, so the budget guard still works across providers.
func (c *Client) cost(promptTokens int, completionTokens int, reported float64, model string) float64 {
	if reported > 0 {
		return reported
	}
	prices := c.modelPrices()
	price, ok := prices[model]
	if !ok {
		return 0
	}
	return float64(promptTokens)/1e6*price[0] + float64(completionTokens)/1e6*price[1]
}

func (c *Client) modelPrices() map[string][2]float64 {
	c.priceMu.Lock()
	defer c.priceMu.Unlock()
	if c.prices != nil && time.Since(c.pricesLoaded) < 6*time.Hour {
		return c.prices
	}
	prices := map[string][2]float64{}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(modelsURL)
	if err != nil {
		if c.prices != nil {
			return c.prices
		}
		return prices
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return prices
	}
	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return prices
	}
	for _, model := range payload.Data {
		prompt := parseFloat(model.Pricing.Prompt) * 1e6
		completion := parseFloat(model.Pricing.Completion) * 1e6
		prices[model.ID] = [2]float64{prompt, completion}
	}
	c.prices = prices
	c.pricesLoaded = time.Now()
	return prices
}

func parseFloat(value string) float64 {
	var parsed float64
	fmt.Sscanf(value, "%g", &parsed)
	return parsed
}

// Embed requests vectors for the given texts. dimensions trims the model output when
// the provider supports it, which cuts both storage and the memory needed for search.
func (c *Client) Embed(ctx context.Context, texts []string, dimensions int) ([][]float32, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("llm: OPENROUTER_API_KEY not configured")
	}
	vectors, err := c.embed(ctx, texts, dimensions)
	if err != nil && dimensions > 0 {
		// Not every embedding model accepts dimensions; retry at the model default
		// instead of failing the whole batch.
		return c.embed(ctx, texts, 0)
	}
	return vectors, err
}

func (c *Client) embed(ctx context.Context, texts []string, dimensions int) ([][]float32, error) {
	body := map[string]any{"model": c.embedModel, "input": texts}
	if dimensions > 0 {
		body["dimensions"] = dimensions
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embeddingURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.headers(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: embeddings http %d %s", resp.StatusCode, Truncate(string(raw), 200))
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	embeddings := make([][]float32, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		embeddings = append(embeddings, item.Embedding)
	}
	return embeddings, nil
}

func (c *Client) EmbeddingModel() string {
	return c.embedModel
}
