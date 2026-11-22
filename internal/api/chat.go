package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"portfolio/internal/llm"
	"portfolio/internal/store"
)

func (s *Server) chatHandler(c echo.Context) error {
	userID := s.userID(c)
	if !s.Cfg.DevMode {
		if ok, retry := s.allow("chat:"+userID, 15*time.Second); !ok {
			return c.JSON(http.StatusTooManyRequests, map[string]any{
				"error": "Please wait before sending another message", "retry_after_sec": retry,
			})
		}
	}

	var request struct {
		ConversationID string `json:"conversation_id"`
		Message        string `json:"message"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	request.Message = strings.TrimSpace(request.Message)
	if request.Message == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Message is required"})
	}

	conversation, err := s.DB.GetConversation(request.ConversationID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Conversation not found"})
	}
	if conversation == nil {
		conversation, err = s.DB.CreateConversation(userID, llm.Truncate(request.Message, 50))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create conversation"})
		}
	} else if conversation.Title == "" {
		s.DB.UpdateConversationTitleIfDefault(conversation.ID, llm.Truncate(request.Message, 50))
	}

	if _, err := s.DB.AddChatMessage(conversation.ID, "user", request.Message, ""); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save message"})
	}

	response := c.Response()
	response.Header().Set(echo.HeaderContentType, "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("X-Accel-Buffering", "no")
	response.WriteHeader(http.StatusOK)
	response.Flush()

	assistantID, _ := s.DB.CreatePendingAssistantMessage(conversation.ID)
	streamed := ""
	lastSaved := 0
	emit := func(event map[string]any) {
		if delta, ok := event["delta"].(string); ok {
			streamed += delta
			if len(streamed)-lastSaved >= 120 {
				lastSaved = len(streamed)
				_ = s.DB.UpdateChatMessage(assistantID, streamed, "", "streaming", "")
			}
		}
		payload, err := json.Marshal(event)
		if err != nil {
			return
		}
		_, _ = response.Write([]byte("data: " + string(payload) + "\n\n"))
		response.Flush()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	answer, sources, trace := s.Chat.Run(ctx, userID, conversation.ID, request.Message, emit)
	traceJSON := ""
	if trace != nil {
		if encoded, err := json.Marshal(trace); err == nil {
			traceJSON = string(encoded)
		}
	}
	if answer == "" {
		_ = s.DB.UpdateChatMessage(assistantID, streamed, "", "failed", traceJSON)
		return nil
	}
	searchJSON, _ := json.Marshal(sources)
	_ = s.DB.UpdateChatMessage(assistantID, answer, string(searchJSON), "done", traceJSON)
	_, _ = response.Write([]byte("data: [DONE]\n\n"))
	response.Flush()
	return nil
}

func (s *Server) listConversations(c echo.Context) error {
	conversations, err := s.DB.ListConversations(s.userID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to list conversations"})
	}
	if conversations == nil {
		conversations = []store.Conversation{}
	}
	return c.JSON(http.StatusOK, conversations)
}

func (s *Server) getConversation(c echo.Context) error {
	conversation, err := s.DB.GetConversation(c.Param("id"), s.userID(c))
	if err != nil || conversation == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Conversation not found"})
	}
	messages, err := s.DB.ConversationMessages(conversation.ID, 200)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load messages"})
	}
	return c.JSON(http.StatusOK, map[string]any{"conversation": conversation, "messages": messages})
}

func (s *Server) deleteConversation(c echo.Context) error {
	if err := s.DB.DeleteConversation(c.Param("id"), s.userID(c)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete conversation"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Conversation deleted"})
}

func (s *Server) reindexRAG(c echo.Context) error {
	s.RAG.Trigger()
	return c.JSON(http.StatusOK, map[string]string{"message": "RAG reindex started"})
}

// usageSummary exposes what the LLM layer spent, which makes cost tuning measurable.
func (s *Server) usageSummary(c echo.Context) error {
	userID := s.userID(c)
	today := startOfToday()
	rows, err := s.DB.LLMUsageSummary(userID, today)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load usage"})
	}
	total := 0.0
	for _, row := range rows {
		total += row.CostUSD
	}
	systemRows, _ := s.DB.LLMUsageSummary("system", today)
	systemTotal := 0.0
	for _, row := range systemRows {
		systemTotal += row.CostUSD
	}
	return c.JSON(http.StatusOK, map[string]any{
		"date":        store.Today(),
		"total_usd":   total + systemTotal,
		"user_usd":    total,
		"system_usd":  systemTotal,
		"daily_limit": s.Cfg.LLMDailyBudgetUSD,
		"by_kind":     append(rows, systemRows...),
		"chat_model":  s.Cfg.SessionModel,
		"cheap_model": s.Cfg.CheapModel,
	})
}
