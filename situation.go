package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type SituationTask struct {
	Id                string   `json:"id"`
	UserID            string   `json:"user_id"`
	Subject           string   `json:"subject"`
	SubTopics         []string `json:"sub_topics"`
	Enabled           bool     `json:"enabled"`
	Deleted           bool     `json:"deleted"`
	DailyHour         int      `json:"daily_hour"`
	LastRunDate       string   `json:"last_run_date"`
	CreatedAt         int64    `json:"created_at"`
	UpdatedAt         int64    `json:"updated_at"`
	LastReportDate    string   `json:"last_report_date,omitempty"`
	LastReportStatus  string   `json:"last_report_status,omitempty"`
	LastReportSummary string   `json:"last_report_summary,omitempty"`
}

type SituationReport struct {
	Id            string `json:"id"`
	TaskID        string `json:"task_id"`
	UserID        string `json:"user_id"`
	Subject       string `json:"subject"`
	Date          string `json:"date"`
	Summary       string `json:"summary"`
	Content       string `json:"content"`
	Status        string `json:"status"`
	Iterations    int    `json:"iterations"`
	SearchResults string `json:"search_results"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

var (
	situationRunMu sync.Mutex
	situationRun   = map[string]bool{}
)

const (
	situationMaxSearches = 8
	situationRunTimeout  = 8 * time.Minute
)

func createSituationTables(sqlDB *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS situation_tasks (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			sub_topics TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			deleted INTEGER NOT NULL DEFAULT 0,
			daily_hour INTEGER NOT NULL DEFAULT 9,
			last_run_date TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_situation_tasks_user ON situation_tasks(user_id)`,
		`CREATE TABLE IF NOT EXISTS situation_reports (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			subject TEXT,
			date TEXT NOT NULL,
			summary TEXT,
			content TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			iterations INTEGER NOT NULL DEFAULT 0,
			search_results TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY (task_id) REFERENCES situation_tasks(id) ON DELETE CASCADE,
			UNIQUE(task_id, date)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_situation_reports_user_date ON situation_reports(user_id, date)`,
		`CREATE TABLE IF NOT EXISTS situation_news (
			id TEXT PRIMARY KEY,
			key TEXT NOT NULL UNIQUE,
			task_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			title TEXT,
			link TEXT,
			summary TEXT,
			text TEXT,
			published_at TEXT,
			date TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			FOREIGN KEY (task_id) REFERENCES situation_tasks(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_situation_news_user_date ON situation_news(user_id, date)`,
	}
	for _, s := range stmts {
		if _, err := sqlDB.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func (database *DB) createSituationTask(t *SituationTask) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	subJSON, _ := json.Marshal(t.SubTopics)
	_, err := database.Exec(`
		INSERT INTO situation_tasks (id, user_id, subject, sub_topics, enabled, deleted, daily_hour, last_run_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.Id, t.UserID, t.Subject, string(subJSON), b2i(t.Enabled), b2i(t.Deleted), t.DailyHour, t.LastRunDate, t.CreatedAt, t.UpdatedAt)
	return err
}

func (database *DB) getSituationTask(id string, userID string) (*SituationTask, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return scanSituationTask(database.QueryRow(`
		SELECT id, user_id, subject, sub_topics, enabled, deleted, daily_hour, COALESCE(last_run_date,''), created_at, updated_at
		FROM situation_tasks WHERE id = ? AND user_id = ?
	`, id, userID))
}

func (database *DB) listSituationTasks(userID string, includeDeleted bool) ([]SituationTask, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	query := `SELECT id, user_id, subject, sub_topics, enabled, deleted, daily_hour, COALESCE(last_run_date,''), created_at, updated_at FROM situation_tasks WHERE user_id = ?`
	args := []interface{}{userID}
	if !includeDeleted {
		query += ` AND deleted = 0`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []SituationTask
	for rows.Next() {
		t, err := scanSituationTask(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, *t)
	}
	return tasks, nil
}

func (database *DB) listEnabledSituationTasks() ([]SituationTask, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	rows, err := database.Query(`
		SELECT id, user_id, subject, sub_topics, enabled, deleted, daily_hour, COALESCE(last_run_date,''), created_at, updated_at
		FROM situation_tasks WHERE enabled = 1 AND deleted = 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []SituationTask
	for rows.Next() {
		t, err := scanSituationTask(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, *t)
	}
	return tasks, nil
}

func scanSituationTask(row interface {
	Scan(dest ...interface{}) error
}) (*SituationTask, error) {
	var t SituationTask
	var subJSON string
	var enabled, deleted int
	err := row.Scan(&t.Id, &t.UserID, &t.Subject, &subJSON, &enabled, &deleted, &t.DailyHour, &t.LastRunDate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	t.Deleted = deleted == 1
	_ = json.Unmarshal([]byte(subJSON), &t.SubTopics)
	if t.SubTopics == nil {
		t.SubTopics = []string{}
	}
	return &t, nil
}

func (database *DB) updateSituationTask(t *SituationTask) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	subJSON, _ := json.Marshal(t.SubTopics)
	_, err := database.Exec(`
		UPDATE situation_tasks SET subject = ?, sub_topics = ?, enabled = ?, daily_hour = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, t.Subject, string(subJSON), b2i(t.Enabled), t.DailyHour, time.Now().Unix(), t.Id, t.UserID)
	return err
}

func (database *DB) softDeleteSituationTask(id string, userID string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		UPDATE situation_tasks SET deleted = 1, updated_at = ? WHERE id = ? AND user_id = ?
	`, time.Now().Unix(), id, userID)
	return err
}

func (database *DB) setSituationTaskEnabled(id string, userID string, enabled bool) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		UPDATE situation_tasks SET enabled = ?, updated_at = ? WHERE id = ? AND user_id = ?
	`, b2i(enabled), time.Now().Unix(), id, userID)
	return err
}

func (database *DB) updateSituationTaskLastRun(id string, date string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		UPDATE situation_tasks SET last_run_date = ?, updated_at = ? WHERE id = ?
	`, date, time.Now().Unix(), id)
	return err
}

func (database *DB) upsertSituationReport(r *SituationReport) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		INSERT INTO situation_reports (id, task_id, user_id, subject, date, summary, content, status, iterations, search_results, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, date) DO UPDATE SET
			subject = excluded.subject,
			summary = excluded.summary,
			content = excluded.content,
			status = excluded.status,
			iterations = excluded.iterations,
			search_results = excluded.search_results,
			updated_at = excluded.updated_at
	`, r.Id, r.TaskID, r.UserID, r.Subject, r.Date, r.Summary, r.Content, r.Status, r.Iterations, r.SearchResults, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (database *DB) getSituationReport(taskID string, date string) (*SituationReport, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	var r SituationReport
	err := database.QueryRow(`
		SELECT id, task_id, user_id, COALESCE(subject,''), date, COALESCE(summary,''), COALESCE(content,''), status, COALESCE(iterations,0), COALESCE(search_results,''), created_at, updated_at
		FROM situation_reports WHERE task_id = ? AND date = ?
	`, taskID, date).Scan(&r.Id, &r.TaskID, &r.UserID, &r.Subject, &r.Date, &r.Summary, &r.Content, &r.Status, &r.Iterations, &r.SearchResults, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (database *DB) getLatestSituationReport(taskID string) (*SituationReport, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	var r SituationReport
	err := database.QueryRow(`
		SELECT id, task_id, user_id, COALESCE(subject,''), date, COALESCE(summary,''), COALESCE(content,''), status, COALESCE(iterations,0), COALESCE(search_results,''), created_at, updated_at
		FROM situation_reports WHERE task_id = ? ORDER BY date DESC LIMIT 1
	`, taskID).Scan(&r.Id, &r.TaskID, &r.UserID, &r.Subject, &r.Date, &r.Summary, &r.Content, &r.Status, &r.Iterations, &r.SearchResults, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (database *DB) getSituationReportsForDate(userID string, date string, taskID string) ([]SituationReport, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	query := `
		SELECT id, task_id, user_id, COALESCE(subject,''), date, COALESCE(summary,''), COALESCE(content,''), status, COALESCE(iterations,0), COALESCE(search_results,''), created_at, updated_at
		FROM situation_reports WHERE user_id = ? AND date = ?`
	args := []interface{}{userID, date}
	if taskID != "" {
		query += ` AND task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []SituationReport
	for rows.Next() {
		var r SituationReport
		if err := rows.Scan(&r.Id, &r.TaskID, &r.UserID, &r.Subject, &r.Date, &r.Summary, &r.Content, &r.Status, &r.Iterations, &r.SearchResults, &r.CreatedAt, &r.UpdatedAt); err == nil {
			reports = append(reports, r)
		}
	}
	return reports, nil
}

func (database *DB) getSituationReportsForTask(userID string, taskID string) ([]SituationReport, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	rows, err := database.Query(`
		SELECT id, task_id, user_id, COALESCE(subject,''), date, COALESCE(summary,''), COALESCE(content,''), status, COALESCE(iterations,0), COALESCE(search_results,''), created_at, updated_at
		FROM situation_reports WHERE user_id = ? AND task_id = ? ORDER BY date DESC
	`, userID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []SituationReport
	for rows.Next() {
		var r SituationReport
		if err := rows.Scan(&r.Id, &r.TaskID, &r.UserID, &r.Subject, &r.Date, &r.Summary, &r.Content, &r.Status, &r.Iterations, &r.SearchResults, &r.CreatedAt, &r.UpdatedAt); err == nil {
			reports = append(reports, r)
		}
	}
	return reports, nil
}

func (database *DB) getSituationReportDates(userID string, taskID string) ([]string, error) {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	query := `SELECT DISTINCT date FROM situation_reports WHERE user_id = ?`
	args := []interface{}{userID}
	if taskID != "" {
		query += ` AND task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY date DESC`
	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err == nil {
			dates = append(dates, d)
		}
	}
	return dates, nil
}

func (database *DB) updateSituationReportResult(id string, content string, summary string, status string, iterations int, searchResults string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	_, err := database.Exec(`
		UPDATE situation_reports SET content = ?, summary = ?, status = ?, iterations = ?, search_results = ?, updated_at = ?
		WHERE id = ?
	`, content, summary, status, iterations, searchResults, time.Now().Unix(), id)
	return err
}

func (database *DB) saveSituationNews(taskID string, userID string, date string, results []SearchResult) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range results {
		if strings.TrimSpace(r.Title) == "" && strings.TrimSpace(r.Snippet) == "" {
			continue
		}
		key := sha256Hex(taskID + "|" + date + "|" + r.Title + "|" + r.URL + "|" + r.Snippet)
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO situation_news (id, key, task_id, user_id, title, link, summary, text, published_at, date, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?, ?)
		`, generateID(), key, taskID, userID, r.Title, r.URL, r.Snippet, date, time.Now().Unix())
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func buildSituationSystemPrompt(task *SituationTask) string {
	return "You are an intelligence analyst producing a daily situation report on a single subject. Today is " + time.Now().UTC().Format("2006-01-02") + ". You have a web_search tool. The sub-topics are areas to investigate, not mandatory sections. Include a section only when you have concrete, dated findings. Use specific facts: dates, locations, numbers, names. Never invent events and never write generic filler."
}

func buildSituationGatherPrompt(task *SituationTask) string {
	var sb strings.Builder
	sb.WriteString("SUBJECT: " + task.Subject + "\n")
	if len(task.SubTopics) > 0 {
		sb.WriteString("SUB-TOPICS (areas to investigate, not required sections):\n")
		for _, st := range task.SubTopics {
			sb.WriteString("- " + st + "\n")
		}
	}
	sb.WriteString("\nRun web searches for the subject and for each sub-topic. Look for specific, dated events: dates, locations, numbers, names, weapons deliveries, policy changes, casualties. Search for current dates, not past years. Use the web_search tool. Gather concrete facts; note when you find nothing for a sub-topic.")
	return sb.String()
}

func buildSituationComposePrompt(task *SituationTask, results []SearchResult) string {
	var sb strings.Builder
	sb.WriteString("SUBJECT: " + task.Subject + "\n")
	if len(task.SubTopics) > 0 {
		sb.WriteString("SUB-TOPICS (areas to investigate, not required sections):\n")
		for _, st := range task.SubTopics {
			sb.WriteString("- " + st + "\n")
		}
	}
	sb.WriteString("\nWeb search results and article content gathered today:\n")
	sb.WriteString(formatSearchResultsDetailed(results, 12000))
	sb.WriteString("\n\nWrite the situation report. Rules: include a section (## header) ONLY for sub-topics with specific, dated findings in the results above; skip sub-topics with no concrete information and do not invent or generalize. Use concrete facts with dates, locations and numbers; prefer details from the article content. If NO web results were gathered, write a short report stating that no specific developments were found. Respond with a single JSON object: {\"summary\": \"...\", \"report\": \"...\"} where report is markdown. Do not include any text outside the JSON.")
	return sb.String()
}

func parseSituationReportJSON(text string) (string, string) {
	start := strings.Index(text, "{")
	if start >= 0 {
		end := strings.LastIndex(text, "}")
		if end > start {
			var obj struct {
				Summary string `json:"summary"`
				Report  string `json:"report"`
			}
			if err := json.Unmarshal([]byte(text[start:end+1]), &obj); err == nil {
				return strings.TrimSpace(obj.Report), strings.TrimSpace(obj.Summary)
			}
		}
	}
	return strings.TrimSpace(stripWebSearchDSML(text)), ""
}

func generateSituationReport(task *SituationTask) {
	situationRunMu.Lock()
	if situationRun[task.Id] {
		situationRunMu.Unlock()
		return
	}
	situationRun[task.Id] = true
	situationRunMu.Unlock()
	defer func() {
		situationRunMu.Lock()
		delete(situationRun, task.Id)
		situationRunMu.Unlock()
	}()

	userID := task.UserID
	today := time.Now().UTC().Format("2006-01-02")

	existing, _ := db.getSituationReport(task.Id, today)
	if existing != nil && existing.Status == "running" && time.Since(time.Unix(existing.UpdatedAt, 0)) < 15*time.Minute {
		return
	}

	report := &SituationReport{
		Id:         generateID(),
		TaskID:     task.Id,
		UserID:     userID,
		Subject:    task.Subject,
		Date:       today,
		Status:     "running",
		Iterations: 0,
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
	}
	if err := db.upsertSituationReport(report); err != nil {
		log.Printf("situation: failed to create running report: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), situationRunTimeout)
	defer cancel()

	messages := []LLMMessage{
		{Role: "system", Content: buildSituationSystemPrompt(task)},
		{Role: "user", Content: buildSituationGatherPrompt(task)},
	}
	tools := []LLMTool{webSearchTool()}
	var gathered []SearchResult
	var finalAnswer string
	failed := false
	searches := 0
	maxRounds := 6

	for round := 0; round < maxRounds; round++ {
		msg, err := llmChat(ctx, messages, tools, "auto")
		if err != nil {
			log.Printf("situation: llm error: %v", err)
			failed = true
			break
		}
		if len(msg.ToolCalls) == 0 {
			dsmlQueries := extractWebSearchQueries(msg.Content)
			if len(dsmlQueries) > 0 && searches < situationMaxSearches {
				for _, q := range dsmlQueries {
					if searches >= situationMaxSearches {
						break
					}
					searches++
					_, results := executeWebSearchWithCache(q, userID)
					gathered = append(gathered, results...)
					messages = append(messages, LLMMessage{Role: "user", Content: "Web search results for \"" + q + "\":\n" + formatSearchResultsDetailed(results, 6000)})
				}
				continue
			}
			finalAnswer = strings.TrimSpace(stripWebSearchDSML(msg.Content))
			break
		}
		messages = append(messages, *msg)
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name != "web_search" {
				continue
			}
			if searches >= situationMaxSearches {
				break
			}
			searches++
			var args struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			_, results := executeWebSearchWithCache(args.Query, userID)
			gathered = append(gathered, results...)
			messages = append(messages, LLMMessage{
				Role:       "tool",
				ToolCallID: tc.Id,
				Name:       tc.Function.Name,
				Content:    formatSearchResultsDetailed(results, 6000),
			})
		}
	}

	content := finalAnswer
	summary := ""
	composeMessages := []LLMMessage{
		{Role: "system", Content: "You are an analyst producing a situation report for " + time.Now().UTC().Format("2006-01-02") + ". Output ONLY a JSON object with keys \"summary\" and \"report\". No extra text."},
		{Role: "user", Content: buildSituationComposePrompt(task, gathered)},
	}
	compMsg, err := llmChat(ctx, composeMessages, nil, "")
	if err != nil {
		log.Printf("situation: compose error: %v", err)
		failed = true
	} else if compMsg != nil {
		rep, sum := parseSituationReportJSON(compMsg.Content)
		if strings.TrimSpace(rep) != "" {
			content = rep
		}
		if strings.TrimSpace(sum) != "" {
			summary = sum
		}
	}
	if strings.TrimSpace(content) == "" {
		content = "I could not generate this situation report today. Please retry."
	}
	if strings.TrimSpace(summary) == "" {
		summary = truncateStr(content, 200)
	}

	searchJSON, _ := json.Marshal(gathered)
	status := "completed"
	if failed {
		status = "failed"
	}
	if err := db.updateSituationReportResult(report.Id, content, summary, status, len(gathered), string(searchJSON)); err != nil {
		log.Printf("situation: failed to save report: %v", err)
	}
	_ = db.saveSituationNews(task.Id, userID, today, gathered)
	_ = db.updateSituationTaskLastRun(task.Id, today)
	if !failed {
		triggerRagReindex()
	}
}

func situationSchedulerPeriodic(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		situationSchedulerTick()
	}
}

func situationSchedulerTick() {
	today := time.Now().UTC().Format("2006-01-02")
	hour := time.Now().Hour()
	tasks, err := db.listEnabledSituationTasks()
	if err != nil {
		log.Printf("situation: list tasks error: %v", err)
		return
	}
	for _, t := range tasks {
		if !situationTaskDue(&t, today, hour) {
			continue
		}
		go generateSituationReport(&t)
	}
}

func situationTaskDue(t *SituationTask, today string, hour int) bool {
	if t.LastRunDate != today {
		return hour >= t.DailyHour
	}
	report, err := db.getSituationReport(t.Id, today)
	if err != nil || report == nil {
		return hour >= t.DailyHour
	}
	if report.Status == "completed" {
		return false
	}
	age := time.Since(time.Unix(report.UpdatedAt, 0))
	if report.Status == "running" && age < 15*time.Minute {
		return false
	}
	if report.Status == "failed" && age < 30*time.Minute {
		return false
	}
	return true
}

func recoverStaleSituationReports() {
	if _, err := db.Exec(`UPDATE situation_reports SET status = 'failed' WHERE status = 'running'`); err != nil {
		log.Printf("situation: failed to mark stale reports: %v", err)
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func situationUserID(c echo.Context) string {
	user := c.Get("user").(*jwt.Token)
	claims := user.Claims.(*JWTClaims)
	return claims.UserID
}

func listSituationTasksHandler(c echo.Context) error {
	userID := situationUserID(c)
	tasks, err := db.listSituationTasks(userID, false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to list tasks"})
	}
	for i := range tasks {
		if latest, err := db.getLatestSituationReport(tasks[i].Id); err == nil && latest != nil {
			tasks[i].LastReportDate = latest.Date
			tasks[i].LastReportStatus = latest.Status
			tasks[i].LastReportSummary = latest.Summary
		}
	}
	if tasks == nil {
		tasks = []SituationTask{}
	}
	return c.JSON(http.StatusOK, tasks)
}

func createSituationTaskHandler(c echo.Context) error {
	userID := situationUserID(c)
	var req struct {
		Subject   string   `json:"subject"`
		SubTopics []string `json:"sub_topics"`
		DailyHour int      `json:"daily_hour"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Subject is required"})
	}
	if req.DailyHour < 0 || req.DailyHour > 23 {
		req.DailyHour = 9
	}
	task := &SituationTask{
		Id:        generateID(),
		UserID:    userID,
		Subject:   req.Subject,
		SubTopics: req.SubTopics,
		Enabled:   true,
		Deleted:   false,
		DailyHour: req.DailyHour,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	if task.SubTopics == nil {
		task.SubTopics = []string{}
	}
	if err := db.createSituationTask(task); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create task"})
	}
	return c.JSON(http.StatusOK, task)
}

func updateSituationTaskHandler(c echo.Context) error {
	userID := situationUserID(c)
	id := c.Param("id")
	task, err := db.getSituationTask(id, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}
	var req struct {
		Subject   string   `json:"subject"`
		SubTopics []string `json:"sub_topics"`
		DailyHour int      `json:"daily_hour"`
		Enabled   *bool    `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Subject != "" {
		task.Subject = strings.TrimSpace(req.Subject)
	}
	if req.SubTopics != nil {
		task.SubTopics = req.SubTopics
	}
	if req.DailyHour >= 0 && req.DailyHour <= 23 {
		task.DailyHour = req.DailyHour
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if err := db.updateSituationTask(task); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update task"})
	}
	return c.JSON(http.StatusOK, task)
}

func deleteSituationTaskHandler(c echo.Context) error {
	userID := situationUserID(c)
	id := c.Param("id")
	if err := db.softDeleteSituationTask(id, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete task"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Task deleted"})
}

func pauseSituationTaskHandler(c echo.Context) error {
	userID := situationUserID(c)
	if err := db.setSituationTaskEnabled(c.Param("id"), userID, false); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to pause task"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Task paused"})
}

func resumeSituationTaskHandler(c echo.Context) error {
	userID := situationUserID(c)
	if err := db.setSituationTaskEnabled(c.Param("id"), userID, true); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to resume task"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Task resumed"})
}

func generateSituationTaskHandler(c echo.Context) error {
	userID := situationUserID(c)
	id := c.Param("id")
	task, err := db.getSituationTask(id, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}
	if task.Deleted {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}
	go generateSituationReport(task)
	return c.JSON(http.StatusAccepted, map[string]string{"message": "Generation started"})
}

func getSituationReportsHandler(c echo.Context) error {
	userID := situationUserID(c)
	date := c.QueryParam("date")
	taskID := c.QueryParam("task_id")
	var reports []SituationReport
	var err error
	if date != "" {
		reports, err = db.getSituationReportsForDate(userID, date, taskID)
	} else if taskID != "" {
		reports, err = db.getSituationReportsForTask(userID, taskID)
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "date or task_id query parameter is required"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load reports"})
	}
	if reports == nil {
		reports = []SituationReport{}
	}
	return c.JSON(http.StatusOK, reports)
}

func getSituationReportHistoryHandler(c echo.Context) error {
	userID := situationUserID(c)
	taskID := c.QueryParam("task_id")
	dates, err := db.getSituationReportDates(userID, taskID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load history"})
	}
	if dates == nil {
		dates = []string{}
	}
	return c.JSON(http.StatusOK, dates)
}
