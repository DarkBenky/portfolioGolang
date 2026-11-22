package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

func (d *DB) CreateConversation(userID string, title string) (*Conversation, error) {
	now := Now()
	conversation := Conversation{ID: NewID(), UserID: userID, Title: title, CreatedAt: now, UpdatedAt: now}
	_, err := d.Exec(`INSERT INTO conversations (id, user_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		conversation.ID, userID, title, now, now)
	if err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (d *DB) ListConversations(userID string) ([]Conversation, error) {
	rows, err := d.Query(`SELECT id, user_id, title, created_at, updated_at FROM conversations
		WHERE user_id = ? ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conversations []Conversation
	for rows.Next() {
		var conversation Conversation
		if err := rows.Scan(&conversation.ID, &conversation.UserID, &conversation.Title,
			&conversation.CreatedAt, &conversation.UpdatedAt); err == nil {
			conversations = append(conversations, conversation)
		}
	}
	return conversations, rows.Err()
}

func (d *DB) GetConversation(id string, userID string) (*Conversation, error) {
	var conversation Conversation
	err := d.QueryRow(`SELECT id, user_id, title, created_at, updated_at FROM conversations
		WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&conversation.ID, &conversation.UserID, &conversation.Title, &conversation.CreatedAt, &conversation.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (d *DB) DeleteConversation(id string, userID string) error {
	if _, err := d.Exec(`DELETE FROM chat_messages WHERE conversation_id IN
		(SELECT id FROM conversations WHERE id = ? AND user_id = ?)`, id, userID); err != nil {
		return err
	}
	_, err := d.Exec(`DELETE FROM conversations WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// The inner select needs aliases: the outer query resolves names against the subquery
// result, not the table, so COALESCE(...) without an alias cannot be found again.
const messageColumns = `id, conversation_id, role, content, COALESCE(search_results,'') AS search_results, COALESCE(status,'') AS status, COALESCE(trace,'') AS trace, created_at`

func (d *DB) ConversationMessages(conversationID string, limit int) ([]ChatMessage, error) {
	rows, err := d.Query(`SELECT `+messageColumns+` FROM (
		SELECT `+messageColumns+` FROM chat_messages WHERE conversation_id = ? ORDER BY created_at DESC LIMIT ?
	) ORDER BY created_at ASC`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []ChatMessage
	for rows.Next() {
		var message ChatMessage
		if err := rows.Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content,
			&message.SearchResults, &message.Status, &message.Trace, &message.CreatedAt); err == nil {
			messages = append(messages, message)
		}
	}
	return messages, rows.Err()
}

func (d *DB) AddChatMessage(conversationID string, role string, content string, searchResults string) (string, error) {
	id := NewID()
	_, err := d.Exec(`INSERT INTO chat_messages (id, conversation_id, role, content, search_results, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, conversationID, role, content, searchResults, "", Now())
	if err != nil {
		return "", err
	}
	d.Exec(`UPDATE conversations SET updated_at = ? WHERE id = ?`, Now(), conversationID)
	return id, nil
}

func (d *DB) CreatePendingAssistantMessage(conversationID string) (string, error) {
	return d.AddChatMessage(conversationID, "assistant", "", "")
}

func (d *DB) UpdateChatMessage(id string, content string, searchResults string, status string, trace string) error {
	_, err := d.Exec(`UPDATE chat_messages SET content = ?, search_results = ?, status = ?, trace = ? WHERE id = ?`,
		content, searchResults, status, trace, id)
	return err
}

func (d *DB) UpdateConversationTitleIfDefault(conversationID string, title string) {
	d.Exec(`UPDATE conversations SET title = ? WHERE id = ? AND title = ''`, title, conversationID)
}

// CompactChatHistory returns the last keep messages and collapses older turns into a
// short digest. Sending every old message back to the model is the single largest
// avoidable chat cost.
func (d *DB) CompactChatHistory(conversationID string, keep int) ([]ChatMessage, error) {
	all, err := d.ConversationMessages(conversationID, 200)
	if err != nil {
		return nil, err
	}
	if len(all) <= keep {
		return all, nil
	}
	return all[len(all)-keep:], nil
}

func (d *DB) UpsertRagChunk(chunk RagChunk) error {
	embedding := ""
	if len(chunk.Embedding) > 0 {
		payload, err := json.Marshal(chunk.Embedding)
		if err != nil {
			return err
		}
		embedding = string(payload)
	}
	_, err := d.Exec(`INSERT INTO rag_chunks (id, source, ticker, user_id, date, content, embedding, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(content_hash) DO UPDATE SET
		content = excluded.content, embedding = excluded.embedding`,
		chunk.ID, chunk.Source, chunk.Ticker, chunk.UserID, chunk.Date, chunk.Content, embedding, chunk.ID, Now())
	return err
}

func (d *DB) RagChunkHashes() (map[string]bool, error) {
	rows, err := d.Query(`SELECT content_hash FROM rag_chunks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hashes := map[string]bool{}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err == nil {
			hashes[hash] = true
		}
	}
	return hashes, rows.Err()
}

func (d *DB) CachedLLMResponse(key string) (string, bool) {
	var response string
	err := d.QueryRow(`SELECT response FROM llm_cache WHERE cache_key = ?`, key).Scan(&response)
	return response, err == nil
}

func (d *DB) SaveLLMResponse(key string, model string, response string, promptTokens int, completionTokens int, cost float64) {
	d.Exec(`INSERT INTO llm_cache (cache_key, model, response, prompt_tokens, completion_tokens, cost_usd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(cache_key) DO UPDATE SET response = excluded.response,
		created_at = excluded.created_at`, key, model, response, promptTokens, completionTokens, cost, Now())
}

func (d *DB) RecordLLMUsage(userID string, kind string, model string, promptTokens int, completionTokens int, cost float64) {
	d.Exec(`INSERT INTO llm_usage (user_id, kind, model, prompt_tokens, completion_tokens, cost_usd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, userID, kind, model, promptTokens, completionTokens, cost, Now())
}

func (d *DB) LLMSpendSince(userID string, since int64) (float64, error) {
	var spend float64
	err := d.QueryRow(`SELECT COALESCE(SUM(cost_usd), 0) FROM llm_usage WHERE user_id = ? AND created_at >= ?`,
		userID, since).Scan(&spend)
	return spend, err
}

func (d *DB) LLMSpendToday(userID string) (float64, error) {
	start := Now() - 86400
	return d.LLMSpendSince(userID, start)
}

type UsageRow struct {
	Kind             string  `json:"kind"`
	Model            string  `json:"model"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Calls            int64   `json:"calls"`
}

func (d *DB) LLMUsageSummary(userID string, since int64) ([]UsageRow, error) {
	rows, err := d.Query(`SELECT kind, model, SUM(prompt_tokens), SUM(completion_tokens), SUM(cost_usd), COUNT(*)
		FROM llm_usage WHERE user_id = ? AND created_at >= ? GROUP BY kind, model ORDER BY 5 DESC`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summary []UsageRow
	for rows.Next() {
		var row UsageRow
		if err := rows.Scan(&row.Kind, &row.Model, &row.PromptTokens, &row.CompletionTokens, &row.CostUSD, &row.Calls); err == nil {
			summary = append(summary, row)
		}
	}
	return summary, rows.Err()
}

// AllRagChunks loads the newest chunks for the in-memory index. Embeddings are only
// selected when asked for: on a legacy database they are tens of gigabytes of JSON
// that the lexical index must not read.
func (d *DB) AllRagChunks(withEmbeddings bool, limit int) ([]RagChunk, error) {
	embeddingColumn := "''"
	if withEmbeddings {
		embeddingColumn = "COALESCE(embedding,'')"
	}
	if limit <= 0 {
		limit = 80000
	}
	rows, err := d.Query(`SELECT id, source, COALESCE(ticker,''), COALESCE(user_id,''), COALESCE(date,''),
		content, `+embeddingColumn+` FROM rag_chunks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []RagChunk
	for rows.Next() {
		var chunk RagChunk
		var embedding string
		if err := rows.Scan(&chunk.ID, &chunk.Source, &chunk.Ticker, &chunk.UserID, &chunk.Date,
			&chunk.Content, &embedding); err != nil {
			return nil, err
		}
		if embedding != "" {
			_ = json.Unmarshal([]byte(embedding), &chunk.Embedding)
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func (d *DB) RagVersion() (int64, int64, error) {
	var count, latest int64
	err := d.QueryRow(`SELECT COUNT(*), COALESCE(MAX(created_at),0) FROM rag_chunks`).Scan(&count, &latest)
	return count, latest, err
}

// RagChunksMissingEmbedding returns chunks without a vector, or with a vector of a
// different size than the current model produces, newest first.
func (d *DB) RagChunksMissingEmbedding(limit int, dimensions int) ([]RagChunk, error) {
	where := `length(COALESCE(embedding,'')) <= 2`
	args := []any{}
	if dimensions > 0 {
		where += ` OR json_array_length(embedding) <> ?`
		args = append(args, dimensions)
	}
	args = append(args, limit)
	rows, err := d.Query(`SELECT id, source, COALESCE(ticker,''), COALESCE(user_id,''), COALESCE(date,''), content
		FROM rag_chunks WHERE `+where+` ORDER BY created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []RagChunk
	for rows.Next() {
		var chunk RagChunk
		if err := rows.Scan(&chunk.ID, &chunk.Source, &chunk.Ticker, &chunk.UserID,
			&chunk.Date, &chunk.Content); err == nil {
			chunks = append(chunks, chunk)
		}
	}
	return chunks, rows.Err()
}

func (d *DB) SetRagChunkEmbedding(id string, vector []float32) error {
	payload, err := json.Marshal(vector)
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE rag_chunks SET embedding = ? WHERE id = ?`, string(payload), id)
	return err
}

// RagEmbeddingCounts reports how much of the corpus carries a vector and how many
// vectors do not have the expected size. Older vectors keep working; they are simply
// skipped by search, so a dimension change only means re-embedding.
func (d *DB) RagEmbeddingCounts(dimensions int) (embedded int64, total int64, wrongSize int64, err error) {
	if err = d.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN length(COALESCE(embedding,'')) > 2 THEN 1 ELSE 0 END),0),
		COUNT(*) FROM rag_chunks`).Scan(&embedded, &total); err != nil {
		return 0, 0, 0, err
	}
	if dimensions <= 0 {
		return embedded, total, 0, nil
	}
	// json_array_length is present in the bundled SQLite; a parse failure only
	// means the counter stays at zero.
	d.QueryRow(`SELECT COUNT(*) FROM rag_chunks
		WHERE length(COALESCE(embedding,'')) > 2 AND json_array_length(embedding) <> ?`,
		dimensions).Scan(&wrongSize)
	return embedded, total, wrongSize, nil
}
