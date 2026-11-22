package llm

import "errors"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type reasoning struct {
	Enabled bool   `json:"enabled"`
	Effort  string `json:"effort,omitempty"`
}

// Handlers receives the two output streams of a chat request. Reasoning is kept
// separate from the answer so the UI can render it collapsed.

type request struct {
	Model       string     `json:"model"`
	Messages    []Message  `json:"messages"`
	Stream      bool       `json:"stream,omitempty"`
	Tools       []Tool     `json:"tools,omitempty"`
	ToolChoice  string     `json:"tool_choice,omitempty"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Temperature *float64   `json:"temperature,omitempty"`
	Reasoning   *reasoning `json:"reasoning,omitempty"`
}

type response struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Cost             float64 `json:"cost"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type Completion struct {
	Content          string
	Reasoning        string
	ToolCalls        []ToolCall
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	CostUSD          float64
	Model            string
	FinishReason     string
}

type Kind string

const (
	KindDailySummary     Kind = "daily_summary"
	KindPortfolioSummary Kind = "portfolio_summary"
	KindRunningSummary   Kind = "running_summary"
	KindSituation        Kind = "situation"
	KindChat             Kind = "chat"
	KindClassify         Kind = "classify"
	KindExpenseReport    Kind = "expense_report"
)

func (k Kind) maxTokens() int {
	switch k {
	case KindChat:
		// Reasoning tokens are drawn from the same budget, so the cap must leave
		// room for both the thinking and the answer.
		return 2500
	case KindSituation:
		return 2600
	case KindRunningSummary:
		return 2600
	case KindPortfolioSummary:
		return 900
	case KindDailySummary:
		return 700
	case KindExpenseReport:
		return 900
	default:
		return 40
	}
}

func (k Kind) useCheapModel() bool {
	return k == KindClassify || k == KindDailySummary
}

// useReasoning enables thinking output for the interactive assistant only. Summary
// and report jobs stay deterministic and cheap.
func (k Kind) useReasoning() bool {
	return k == KindChat
}

var ErrBudgetExceeded = errors.New("llm: daily spend budget reached")

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	CostUSD          float64
	Model            string
}

// Handlers receives the two output streams of a chat request. Reasoning is kept
// separate from the answer so the UI can render it collapsed.
type Handlers struct {
	OnDelta     func(string)
	OnReasoning func(string)
}

type streamDelta struct {
	Content   string `json:"content"`
	Reasoning string `json:"reasoning"`
	ToolCalls []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

type streamChunk struct {
	Choices []struct {
		Delta        streamDelta `json:"delta"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int     `json:"prompt_tokens"`
		CompletionTokens    int     `json:"completion_tokens"`
		Cost                float64 `json:"cost"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func Truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func TrimContext(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[truncated]"
}
