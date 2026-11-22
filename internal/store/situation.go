package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
)

type SituationTask struct {
	ID                string   `json:"id"`
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
	ID            string `json:"id"`
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

func (d *DB) CreateSituationTables() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS situation_tasks (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, subject TEXT NOT NULL, sub_topics TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1, deleted INTEGER NOT NULL DEFAULT 0,
			daily_hour INTEGER NOT NULL DEFAULT 9, last_run_date TEXT, created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_situation_tasks_user ON situation_tasks(user_id)`,
		`CREATE TABLE IF NOT EXISTS situation_reports (
			id TEXT PRIMARY KEY, task_id TEXT NOT NULL, user_id TEXT NOT NULL, subject TEXT, date TEXT NOT NULL,
			summary TEXT, content TEXT, status TEXT NOT NULL DEFAULT 'pending', iterations INTEGER NOT NULL DEFAULT 0,
			attempts INTEGER NOT NULL DEFAULT 0, search_results TEXT, created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL, UNIQUE(task_id, date))`,
		`CREATE INDEX IF NOT EXISTS idx_situation_reports_user_date ON situation_reports(user_id, date)`,
		`CREATE TABLE IF NOT EXISTS situation_news (
			id TEXT PRIMARY KEY, key TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL, user_id TEXT NOT NULL,
			title TEXT, link TEXT, summary TEXT, text TEXT, published_at TEXT, date TEXT NOT NULL,
			created_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_situation_news_user_date ON situation_news(user_id, date)`,
	}
	for _, statement := range statements {
		if _, err := d.Exec(statement); err != nil {
			return err
		}
	}
	d.Exec(`ALTER TABLE situation_reports ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`)
	return nil
}

func (d *DB) CreateSituationTask(task *SituationTask) error {
	subTopics, _ := json.Marshal(task.SubTopics)
	_, err := d.Exec(`INSERT INTO situation_tasks (id, user_id, subject, sub_topics, enabled, deleted,
		daily_hour, last_run_date, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.UserID, task.Subject, string(subTopics), boolToInt(task.Enabled), boolToInt(task.Deleted),
		task.DailyHour, task.LastRunDate, task.CreatedAt, task.UpdatedAt)
	return err
}

const taskColumns = `id, user_id, subject, sub_topics, enabled, deleted, daily_hour,
	COALESCE(last_run_date,''), created_at, updated_at`

func scanTask(scanner interface{ Scan(...any) error }) (*SituationTask, error) {
	var task SituationTask
	var subTopics string
	var enabled, deleted int
	err := scanner.Scan(&task.ID, &task.UserID, &task.Subject, &subTopics, &enabled, &deleted,
		&task.DailyHour, &task.LastRunDate, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	task.Enabled = enabled == 1
	task.Deleted = deleted == 1
	_ = json.Unmarshal([]byte(subTopics), &task.SubTopics)
	if task.SubTopics == nil {
		task.SubTopics = []string{}
	}
	return &task, nil
}

func (d *DB) GetSituationTask(id string, userID string) (*SituationTask, error) {
	task, err := scanTask(d.QueryRow(`SELECT `+taskColumns+` FROM situation_tasks WHERE id = ? AND user_id = ?`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return task, err
}

func (d *DB) ListSituationTasks(userID string, includeDeleted bool) ([]SituationTask, error) {
	query := `SELECT ` + taskColumns + ` FROM situation_tasks WHERE user_id = ?`
	if !includeDeleted {
		query += ` AND deleted = 0`
	}
	rows, err := d.Query(query+` ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []SituationTask
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, *task)
	}
	return tasks, rows.Err()
}

func (d *DB) ListEnabledSituationTasks() ([]SituationTask, error) {
	rows, err := d.Query(`SELECT ` + taskColumns + ` FROM situation_tasks WHERE enabled = 1 AND deleted = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []SituationTask
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, *task)
	}
	return tasks, rows.Err()
}

func (d *DB) UpdateSituationTask(task *SituationTask) error {
	subTopics, _ := json.Marshal(task.SubTopics)
	_, err := d.Exec(`UPDATE situation_tasks SET subject = ?, sub_topics = ?, enabled = ?, daily_hour = ?,
		updated_at = ? WHERE id = ? AND user_id = ?`,
		task.Subject, string(subTopics), boolToInt(task.Enabled), task.DailyHour, Now(), task.ID, task.UserID)
	return err
}

func (d *DB) SoftDeleteSituationTask(id string, userID string) error {
	_, err := d.Exec(`UPDATE situation_tasks SET deleted = 1, updated_at = ? WHERE id = ? AND user_id = ?`, Now(), id, userID)
	return err
}

func (d *DB) SetSituationTaskEnabled(id string, userID string, enabled bool) error {
	_, err := d.Exec(`UPDATE situation_tasks SET enabled = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		boolToInt(enabled), Now(), id, userID)
	return err
}

func (d *DB) UpdateSituationTaskLastRun(id string, date string) error {
	_, err := d.Exec(`UPDATE situation_tasks SET last_run_date = ?, updated_at = ? WHERE id = ?`, date, Now(), id)
	return err
}

func (d *DB) UpsertSituationReport(report *SituationReport) error {
	_, err := d.Exec(`INSERT INTO situation_reports (id, task_id, user_id, subject, date, summary, content,
		status, iterations, search_results, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, date) DO UPDATE SET subject = excluded.subject, summary = excluded.summary,
		content = excluded.content, status = excluded.status, iterations = excluded.iterations,
		search_results = excluded.search_results, updated_at = excluded.updated_at`,
		report.ID, report.TaskID, report.UserID, report.Subject, report.Date, report.Summary, report.Content,
		report.Status, report.Iterations, report.SearchResults, report.CreatedAt, report.UpdatedAt)
	return err
}

const reportColumns = `id, task_id, user_id, COALESCE(subject,''), date, COALESCE(summary,''),
	COALESCE(content,''), status, COALESCE(iterations,0), COALESCE(search_results,''), created_at, updated_at`

func scanReport(scanner interface{ Scan(...any) error }) (SituationReport, error) {
	var report SituationReport
	err := scanner.Scan(&report.ID, &report.TaskID, &report.UserID, &report.Subject, &report.Date,
		&report.Summary, &report.Content, &report.Status, &report.Iterations, &report.SearchResults,
		&report.CreatedAt, &report.UpdatedAt)
	return report, err
}

func (d *DB) GetSituationReport(taskID string, date string) (*SituationReport, error) {
	report, err := scanReport(d.QueryRow(`SELECT `+reportColumns+` FROM situation_reports WHERE task_id = ? AND date = ?`, taskID, date))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (d *DB) LatestSituationReport(taskID string) (*SituationReport, error) {
	report, err := scanReport(d.QueryRow(`SELECT `+reportColumns+` FROM situation_reports
		WHERE task_id = ? ORDER BY date DESC LIMIT 1`, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (d *DB) SituationReports(userID string, date string, taskID string) ([]SituationReport, error) {
	query := `SELECT ` + reportColumns + ` FROM situation_reports WHERE user_id = ?`
	args := []any{userID}
	if date != "" {
		query += ` AND date = ?`
		args = append(args, date)
	}
	if taskID != "" {
		query += ` AND task_id = ?`
		args = append(args, taskID)
	}
	rows, err := d.Query(query+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []SituationReport
	for rows.Next() {
		report, err := scanReport(rows)
		if err == nil {
			reports = append(reports, report)
		}
	}
	return reports, rows.Err()
}

func (d *DB) SituationReportDates(userID string, taskID string) ([]string, error) {
	query := `SELECT DISTINCT date FROM situation_reports WHERE user_id = ?`
	args := []any{userID}
	if taskID != "" {
		query += ` AND task_id = ?`
		args = append(args, taskID)
	}
	rows, err := d.Query(query+` ORDER BY date DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err == nil {
			dates = append(dates, date)
		}
	}
	return dates, rows.Err()
}

func (d *DB) UpdateSituationReportResult(id string, content string, summary string, status string, iterations int, searchResults string) error {
	_, err := d.Exec(`UPDATE situation_reports SET content = ?, summary = ?, status = ?, iterations = ?,
		search_results = ?, updated_at = ? WHERE id = ?`, content, summary, status, iterations, searchResults, Now(), id)
	return err
}

func (d *DB) BumpSituationAttempts(taskID string, date string) int {
	if _, err := d.Exec(`UPDATE situation_reports SET attempts = attempts + 1 WHERE task_id = ? AND date = ?`, taskID, date); err != nil {
		return 0
	}
	var attempts int
	if err := d.QueryRow(`SELECT attempts FROM situation_reports WHERE task_id = ? AND date = ?`, taskID, date).Scan(&attempts); err != nil {
		return 0
	}
	return attempts
}

func (d *DB) SaveSituationNews(taskID string, userID string, date string, results []SearchResult) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, result := range results {
		if result.Title == "" && result.Snippet == "" {
			continue
		}
		key := HashString(taskID, date, result.Title, result.URL, result.Snippet)
		if _, err := tx.Exec(`INSERT OR IGNORE INTO situation_news (id, key, task_id, user_id, title, link,
			summary, text, published_at, date, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?, ?)`,
			NewID(), key, taskID, userID, result.Title, result.URL, result.Snippet, date, Now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) PreviousSituationSummary(taskID string, date string) string {
	var summary, reportDate string
	err := d.QueryRow(`SELECT date, COALESCE(summary,'') FROM situation_reports
		WHERE task_id = ? AND date < ? AND status = 'completed' ORDER BY date DESC LIMIT 1`, taskID, date).
		Scan(&reportDate, &summary)
	if err != nil || summary == "" {
		return ""
	}
	return "Previous report (" + reportDate + "): " + summary
}

func (d *DB) RecoverStaleSituationReports() {
	if _, err := d.Exec(`UPDATE situation_reports SET status = 'failed' WHERE status = 'running'`); err != nil {
		log.Printf("situation: could not mark stale reports: %v", err)
	}
}
