package api

import (
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"portfolio/internal/store"
)

func (s *Server) listSituationTasks(c echo.Context) error {
	tasks, err := s.DB.ListSituationTasks(s.userID(c), false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to list tasks"})
	}
	for i := range tasks {
		if latest, err := s.DB.LatestSituationReport(tasks[i].ID); err == nil && latest != nil {
			tasks[i].LastReportDate = latest.Date
			tasks[i].LastReportStatus = latest.Status
			tasks[i].LastReportSummary = latest.Summary
		}
	}
	if tasks == nil {
		tasks = []store.SituationTask{}
	}
	return c.JSON(http.StatusOK, tasks)
}

func (s *Server) createSituationTask(c echo.Context) error {
	var request struct {
		Subject   string   `json:"subject"`
		SubTopics []string `json:"sub_topics"`
		DailyHour int      `json:"daily_hour"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	request.Subject = trimSpace(request.Subject)
	if request.Subject == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Subject is required"})
	}
	if request.DailyHour < 0 || request.DailyHour > 23 {
		request.DailyHour = 9
	}
	if request.SubTopics == nil {
		request.SubTopics = []string{}
	}
	task := &store.SituationTask{
		ID: store.NewID(), UserID: s.userID(c), Subject: request.Subject, SubTopics: request.SubTopics,
		Enabled: true, DailyHour: request.DailyHour, CreatedAt: store.Now(), UpdatedAt: store.Now(),
	}
	if err := s.DB.CreateSituationTask(task); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create task"})
	}
	return c.JSON(http.StatusOK, task)
}

func (s *Server) updateSituationTask(c echo.Context) error {
	task, err := s.DB.GetSituationTask(c.Param("id"), s.userID(c))
	if err != nil || task == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}
	var request struct {
		Subject   string   `json:"subject"`
		SubTopics []string `json:"sub_topics"`
		DailyHour int      `json:"daily_hour"`
		Enabled   *bool    `json:"enabled"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if request.Subject != "" {
		task.Subject = trimSpace(request.Subject)
	}
	if request.SubTopics != nil {
		task.SubTopics = request.SubTopics
	}
	if request.DailyHour >= 0 && request.DailyHour <= 23 {
		task.DailyHour = request.DailyHour
	}
	if request.Enabled != nil {
		task.Enabled = *request.Enabled
	}
	if err := s.DB.UpdateSituationTask(task); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update task"})
	}
	return c.JSON(http.StatusOK, task)
}

func (s *Server) deleteSituationTask(c echo.Context) error {
	if err := s.DB.SoftDeleteSituationTask(c.Param("id"), s.userID(c)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete task"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Task deleted"})
}

func (s *Server) pauseSituationTask(c echo.Context) error {
	if err := s.DB.SetSituationTaskEnabled(c.Param("id"), s.userID(c), false); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to pause task"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Task paused"})
}

func (s *Server) resumeSituationTask(c echo.Context) error {
	if err := s.DB.SetSituationTaskEnabled(c.Param("id"), s.userID(c), true); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to resume task"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Task resumed"})
}

func (s *Server) generateSituationTask(c echo.Context) error {
	task, err := s.DB.GetSituationTask(c.Param("id"), s.userID(c))
	if err != nil || task == nil || task.Deleted {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}
	if ok, retry := s.allow("situation:"+task.ID, 2*time.Minute); !ok {
		return c.JSON(http.StatusTooManyRequests, map[string]any{"error": "Generation already running", "retry_after_sec": retry})
	}
	copy := *task
	go s.Jobs.GenerateSituation(copy)
	return c.JSON(http.StatusAccepted, map[string]string{"message": "Generation started"})
}

func (s *Server) getSituationReports(c echo.Context) error {
	date := c.QueryParam("date")
	taskID := c.QueryParam("task_id")
	if date == "" && taskID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "date or task_id query parameter is required"})
	}
	reports, err := s.DB.SituationReports(s.userID(c), date, taskID)
	if err != nil {
		log.Printf("api: situation reports: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load reports"})
	}
	if reports == nil {
		reports = []store.SituationReport{}
	}
	return c.JSON(http.StatusOK, reports)
}

func (s *Server) getSituationReportHistory(c echo.Context) error {
	dates, err := s.DB.SituationReportDates(s.userID(c), c.QueryParam("task_id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load history"})
	}
	if dates == nil {
		dates = []string{}
	}
	return c.JSON(http.StatusOK, dates)
}

func trimSpace(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}
