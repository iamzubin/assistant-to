package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"assistant-to/internal/config"
	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"
)

type Server struct {
	httpServer *http.Server
	pwd        string
	config     *config.Config
	db         *db.DB
	prefix     string
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func NewServer(pwd string, cfg *config.Config, database *db.DB) *Server {
	return &Server{
		pwd:    pwd,
		config: cfg,
		db:     database,
		prefix: sandbox.ProjectPrefix(pwd),
	}
}

func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.API.Host, s.config.API.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/mail/list", s.handleMailList)
	mux.HandleFunc("/api/mail/send", s.handleMailSend)
	mux.HandleFunc("/api/mail/check", s.handleMailCheck)
	mux.HandleFunc("/api/log", s.handleLog)
	mux.HandleFunc("/api/task/list", s.handleTaskList)
	mux.HandleFunc("/api/task/update", s.handleTaskUpdate)
	mux.HandleFunc("/api/buffer", s.handleBuffer)
	mux.HandleFunc("/api/session/list", s.handleSessionList)
	mux.HandleFunc("/api/session/kill", s.handleSessionKill)
	mux.HandleFunc("/api/session/send", s.handleSessionSend)
	mux.HandleFunc("/api/session/clear", s.handleSessionClear)
	mux.HandleFunc("/api/cleanup", s.handleCleanup)
	mux.HandleFunc("/api/worktree/merge", s.handleWorktreeMerge)
	mux.HandleFunc("/api/worktree/teardown", s.handleWorktreeTeardown)
	mux.HandleFunc("/api/config", s.handleConfig)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("API Server starting on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("API Server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		log.Println("API Server shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}()

	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, APIResponse{Success: true, Data: map[string]string{"status": "ok"}})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	allowedTools := s.config.AgentsRT

	// Get role from query param or header
	role := r.URL.Query().Get("role")
	if role == "" {
		role = r.Header.Get("X-Agent-Role")
	}

	if role != "" {
		if rt, ok := allowedTools[role]; ok {
			s.jsonResponse(w, APIResponse{Success: true, Data: rt})
			return
		}
	}

	s.jsonResponse(w, APIResponse{Success: true, Data: allowedTools})
}

func (s *Server) handleMailList(w http.ResponseWriter, r *http.Request) {
	recipient := r.URL.Query().Get("recipient")

	var rows *sql.Rows
	var err error
	if recipient != "" {
		rows, err = s.db.Query(`SELECT sender, subject, body, type, priority, timestamp FROM mail WHERE recipient = ? ORDER BY timestamp DESC LIMIT 50`, recipient)
	} else {
		rows, err = s.db.Query(`SELECT sender, subject, body, type, priority, timestamp FROM mail ORDER BY timestamp DESC LIMIT 50`)
	}
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var sender, subject, body, msgType string
		var priority int
		var timestamp time.Time
		rows.Scan(&sender, &subject, &body, &msgType, &priority, &timestamp)
		messages = append(messages, map[string]interface{}{
			"sender":   sender,
			"subject":  subject,
			"body":     body,
			"type":     msgType,
			"priority": priority,
			"time":     timestamp,
		})
	}
	s.jsonResponse(w, APIResponse{Success: true, Data: messages})
}

func (s *Server) handleMailSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		To      string `json:"to"`
		From    string `json:"from"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		Type    string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.From == "" {
		req.From = "api"
	}
	if req.Type == "" {
		req.Type = "status"
	}

	err := s.db.SendMail(req.From, req.To, req.Subject, req.Body, req.Type, 3)
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleMailCheck(w http.ResponseWriter, r *http.Request) {
	recipient := r.URL.Query().Get("recipient")
	if recipient == "" {
		recipient = r.Header.Get("X-Agent-Role")
	}
	if recipient == "" {
		recipient = "User"
	}

	mail, err := s.db.GetUnreadMail(recipient)
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	// Mark as read
	for _, m := range mail {
		s.db.MarkMailRead(m.ID)
	}

	s.jsonResponse(w, APIResponse{Success: true, Data: mail})
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
		Type    string `json:"type"`
		Details string `json:"details"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.AgentID == "" {
		req.AgentID = r.Header.Get("X-Agent-Role")
	}
	if req.Type == "" {
		req.Type = "info"
	}

	s.db.RecordEvent(req.AgentID, req.Type, req.Details)
	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	var tasks []db.Task
	var err error
	if status != "" {
		tasks, err = s.db.ListTasksByStatus(status)
	} else {
		tasks, err = s.db.ListTasksByStatus("")
	}

	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true, Data: tasks})
}

func (s *Server) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		TaskID  int    `json:"task_id"`
		Status  string `json:"status"`
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	err := s.db.UpdateTaskStatus(req.TaskID, req.Status)
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.db.RecordEvent(req.AgentID, "task_status", fmt.Sprintf("Task %d -> %s", req.TaskID, req.Status))
	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleBuffer(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	lines := r.URL.Query().Get("lines")

	if agentID == "" {
		s.jsonResponse(w, APIResponse{Success: false, Error: "agent_id required"})
		return
	}

	lineCount := 20
	fmt.Sscanf(lines, "%d", &lineCount)

	sessionName := s.prefix + agentID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	if !session.HasSession() {
		s.jsonResponse(w, APIResponse{Success: false, Error: "session not found"})
		return
	}

	output, err := session.CaptureBuffer(lineCount)
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true, Data: map[string]string{"buffer": output}})
}

func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	sessions, err := sandbox.ListSessions(s.prefix)
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}
	s.jsonResponse(w, APIResponse{Success: true, Data: sessions})
}

func (s *Server) handleSessionKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	sessionName := s.prefix + req.AgentID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	if !session.HasSession() {
		s.jsonResponse(w, APIResponse{Success: false, Error: "session not found"})
		return
	}

	err := session.Kill()
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleSessionSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
		Input   string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	sessionName := s.prefix + req.AgentID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	if !session.HasSession() {
		s.jsonResponse(w, APIResponse{Success: false, Error: "session not found"})
		return
	}

	err := session.SendInput(req.Input)
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleSessionClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	sessionName := s.prefix + req.AgentID
	session := &sandbox.TmuxSession{SessionName: sessionName}

	if !session.HasSession() {
		s.jsonResponse(w, APIResponse{Success: false, Error: "session not found"})
		return
	}

	err := session.ClearBuffer()
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		TaskID string `json:"task_id"`
		All    bool   `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.All {
		tasks, err := s.db.ListTasksByStatus("")
		if err != nil {
			s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
			return
		}

		cleaned := 0
		for _, task := range tasks {
			if task.Status == "complete" || task.Status == "review" {
				taskID := fmt.Sprintf("%d", task.ID)
				sessionName := s.prefix + taskID
				session := &sandbox.TmuxSession{SessionName: sessionName}
				if session.HasSession() {
					session.Kill()
				}
				sandbox.TeardownWorktree(taskID, s.pwd, "")
				cleaned++
			}
		}
		s.jsonResponse(w, APIResponse{Success: true, Data: map[string]int{"cleaned": cleaned}})
		return
	}

	taskID := req.TaskID
	sessionName := s.prefix + taskID
	session := &sandbox.TmuxSession{SessionName: sessionName}
	if session.HasSession() {
		session.Kill()
	}

	sandbox.TeardownWorktree(taskID, s.pwd, "")
	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleWorktreeMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		TaskID     string `json:"task_id"`
		BaseBranch string `json:"base_branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.BaseBranch == "" {
		req.BaseBranch = "main"
	}

	err := sandbox.MergeWorktree(req.TaskID, req.BaseBranch, s.pwd)
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) handleWorktreeTeardown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonResponse(w, APIResponse{Success: false, Error: "POST only"})
		return
	}

	var req struct {
		TaskID string `json:"task_id"`
		All    bool   `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	if req.All {
		err := sandbox.TeardownAllWorktrees(s.pwd, "")
		if err != nil {
			s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
			return
		}
		s.jsonResponse(w, APIResponse{Success: true})
		return
	}

	err := sandbox.TeardownWorktree(req.TaskID, s.pwd, "")
	if err != nil {
		s.jsonResponse(w, APIResponse{Success: false, Error: err.Error()})
		return
	}

	s.jsonResponse(w, APIResponse{Success: true})
}

func (s *Server) jsonResponse(w http.ResponseWriter, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
