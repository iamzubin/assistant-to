package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"assistant-to/internal/config"
	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"
)

type MCPServer struct {
	port       int
	pwd        string
	config     *config.Config
	db         *db.DB
	prefix     string
	tools      []MCPTool
	httpServer *http.Server
	mu         sync.RWMutex
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Handler     func(params json.RawMessage) (interface{}, error)
}

type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewMCPServer(port int, pwd string, cfg *config.Config, database *db.DB) *MCPServer {
	s := &MCPServer{
		port:   port,
		pwd:    pwd,
		config: cfg,
		db:     database,
		prefix: sandbox.ProjectPrefix(pwd),
	}
	s.initTools()
	return s
}

func (s *MCPServer) initTools() {
	s.tools = []MCPTool{
		{
			Name:        "mail_list",
			Description: "List mail messages, optionally filtered by recipient",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"recipient":{"type":"string"}}}`),
			Handler:     s.handleMailList,
		},
		{
			Name:        "mail_send",
			Description: "Send a mail message to another agent",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"to":{"type":"string"},"subject":{"type":"string"},"body":{"type":"string"},"type":{"type":"string"}},"required":["to","subject"]}`),
			Handler:     s.handleMailSend,
		},
		{
			Name:        "mail_check",
			Description: "Check and retrieve unread mail messages, marks them as read",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"recipient":{"type":"string"}}}`),
			Handler:     s.handleMailCheck,
		},
		{
			Name:        "log_event",
			Description: "Log an event to the coordinator's event log",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"type":{"type":"string"},"details":{"type":"string"}},"required":["details"]}`),
			Handler:     s.handleLog,
		},
		{
			Name:        "task_list",
			Description: "List tasks, optionally filtered by status",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string"}}}`),
			Handler:     s.handleTaskList,
		},
		{
			Name:        "task_update",
			Description: "Update a task's status",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"integer"},"status":{"type":"string"}},"required":["task_id","status"]}`),
			Handler:     s.handleTaskUpdate,
		},
		{
			Name:        "buffer_capture",
			Description: "Capture the tmux buffer for an agent session",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string"},"lines":{"type":"integer"}},"required":["agent_id"]}`),
			Handler:     s.handleBuffer,
		},
		{
			Name:        "session_send",
			Description: "Send input to an agent's tmux session",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string"},"input":{"type":"string"}},"required":["agent_id","input"]}`),
			Handler:     s.handleSessionSend,
		},
		{
			Name:        "session_kill",
			Description: "Kill an agent's tmux session",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string"}},"required":["agent_id"]}`),
			Handler:     s.handleSessionKill,
		},
		{
			Name:        "session_clear",
			Description: "Clear an agent's tmux buffer",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string"}},"required":["agent_id"]}`),
			Handler:     s.handleSessionClear,
		},
		{
			Name:        "session_list",
			Description: "List all active agent sessions",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			Handler:     s.handleSessionList,
		},
		{
			Name:        "cleanup",
			Description: "Clean up a completed task (kill session + remove worktree)",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string"}},"required":["task_id"]}`),
			Handler:     s.handleCleanup,
		},
		{
			Name:        "worktree_merge",
			Description: "Merge a task's worktree branch into base branch",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string"},"base_branch":{"type":"string"}},"required":["task_id"]}`),
			Handler:     s.handleWorktreeMerge,
		},
		{
			Name:        "worktree_teardown",
			Description: "Remove a task's worktree",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string"}},"required":["task_id"]}`),
			Handler:     s.handleWorktreeTeardown,
		},
	}
}

func (s *MCPServer) Start(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("MCP Server starting on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("MCP Server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		log.Println("MCP Server shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}()

	return nil
}

func (s *MCPServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(MCPResponse{
			JSONRPC: "2.0",
			Error:   &MCPError{Code: -32700, Message: "Parse error"},
		})
		return
	}

	resp := s.ProcessRequest(req)
	json.NewEncoder(w).Encode(resp)
}

// ProcessRequest handles an MCP request and returns a response
// This can be used for both HTTP and stdio communication
func (s *MCPServer) ProcessRequest(req MCPRequest) MCPResponse {
	var resp MCPResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "tools/list":
		s.mu.RLock()
		tools := s.tools
		s.mu.RUnlock()
		resp = MCPResponse{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"tools": tools,
			},
			ID: req.ID,
		}
	case "tools/call":
		var params struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp = MCPResponse{
				JSONRPC: "2.0",
				Error:   &MCPError{Code: -32602, Message: "Invalid params"},
				ID:      req.ID,
			}
		} else {
			s.mu.RLock()
			var result interface{}
			var toolErr error
			for _, t := range s.tools {
				if t.Name == params.Name {
					result, toolErr = t.Handler(params.Args)
					break
				}
			}
			s.mu.RUnlock()

			if toolErr != nil {
				resp = MCPResponse{
					JSONRPC: "2.0",
					Error:   &MCPError{Code: -32000, Message: toolErr.Error()},
					ID:      req.ID,
				}
			} else {
				resp = MCPResponse{
					JSONRPC: "2.0",
					Result:  map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": fmt.Sprintf("%v", result)}}},
					ID:      req.ID,
				}
			}
		}
	case "initialize":
		// MCP initialize method
		resp = MCPResponse{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]string{
					"name":    "assistant-to",
					"version": "1.0.0",
				},
			},
			ID: req.ID,
		}
	default:
		resp = MCPResponse{
			JSONRPC: "2.0",
			Error:   &MCPError{Code: -32601, Message: "Method not found"},
			ID:      req.ID,
		}
	}

	return resp
}

// Tool handlers
func (s *MCPServer) handleMailList(params json.RawMessage) (interface{}, error) {
	var p struct {
		Recipient string `json:"recipient"`
	}
	json.Unmarshal(params, &p)

	var rows *sql.Rows
	var err error
	if p.Recipient != "" {
		rows, err = s.db.Query(`SELECT sender, subject, body, type, priority, timestamp FROM mail WHERE recipient = ? ORDER BY timestamp DESC LIMIT 50`, p.Recipient)
	} else {
		rows, err = s.db.Query(`SELECT sender, subject, body, type, priority, timestamp FROM mail ORDER BY timestamp DESC LIMIT 50`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var sender, subject, body, msgType string
		var priority int
		var timestamp interface{}
		rows.Scan(&sender, &subject, &body, &msgType, &priority, &timestamp)
		messages = append(messages, map[string]interface{}{
			"sender": sender, "subject": subject, "body": body, "type": msgType, "priority": priority,
		})
	}
	return messages, nil
}

func (s *MCPServer) handleMailSend(params json.RawMessage) (interface{}, error) {
	var p struct {
		To      string `json:"to"`
		From    string `json:"from"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		Type    string `json:"type"`
	}
	json.Unmarshal(params, &p)
	if p.From == "" {
		p.From = "mcp"
	}
	if p.Type == "" {
		p.Type = "status"
	}
	return nil, s.db.SendMail(p.From, p.To, p.Subject, p.Body, p.Type, 3)
}

func (s *MCPServer) handleMailCheck(params json.RawMessage) (interface{}, error) {
	var p struct {
		Recipient string `json:"recipient"`
	}
	json.Unmarshal(params, &p)
	if p.Recipient == "" {
		p.Recipient = "User"
	}

	mail, err := s.db.GetUnreadMail(p.Recipient)
	if err != nil {
		return nil, err
	}
	for _, m := range mail {
		s.db.MarkMailRead(m.ID)
	}
	return mail, nil
}

func (s *MCPServer) handleLog(params json.RawMessage) (interface{}, error) {
	var p struct {
		Type    string `json:"type"`
		Details string `json:"details"`
	}
	json.Unmarshal(params, &p)
	if p.Type == "" {
		p.Type = "info"
	}
	s.db.RecordEvent("agent", p.Type, p.Details)
	return "logged", nil
}

func (s *MCPServer) handleTaskList(params json.RawMessage) (interface{}, error) {
	var p struct {
		Status string `json:"status"`
	}
	json.Unmarshal(params, &p)
	if p.Status != "" {
		return s.db.ListTasksByStatus(p.Status)
	}
	return s.db.ListTasksByStatus("")
}

func (s *MCPServer) handleTaskUpdate(params json.RawMessage) (interface{}, error) {
	var p struct {
		TaskID int    `json:"task_id"`
		Status string `json:"status"`
	}
	json.Unmarshal(params, &p)
	return nil, s.db.UpdateTaskStatus(p.TaskID, p.Status)
}

func (s *MCPServer) handleBuffer(params json.RawMessage) (interface{}, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Lines   int    `json:"lines"`
	}
	json.Unmarshal(params, &p)
	if p.Lines == 0 {
		p.Lines = 20
	}

	sessionName := s.prefix + p.AgentID
	session := &sandbox.TmuxSession{SessionName: sessionName}
	if !session.HasSession() {
		return nil, fmt.Errorf("session not found")
	}
	return session.CaptureBuffer(p.Lines)
}

func (s *MCPServer) handleSessionSend(params json.RawMessage) (interface{}, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Input   string `json:"input"`
	}
	json.Unmarshal(params, &p)

	sessionName := s.prefix + p.AgentID
	session := &sandbox.TmuxSession{SessionName: sessionName}
	if !session.HasSession() {
		return nil, fmt.Errorf("session not found")
	}
	return nil, session.SendInput(p.Input)
}

func (s *MCPServer) handleSessionKill(params json.RawMessage) (interface{}, error) {
	var p struct {
		AgentID string `json:"agent_id"`
	}
	json.Unmarshal(params, &p)

	sessionName := s.prefix + p.AgentID
	session := &sandbox.TmuxSession{SessionName: sessionName}
	if !session.HasSession() {
		return nil, fmt.Errorf("session not found")
	}
	return nil, session.Kill()
}

func (s *MCPServer) handleSessionClear(params json.RawMessage) (interface{}, error) {
	var p struct {
		AgentID string `json:"agent_id"`
	}
	json.Unmarshal(params, &p)

	sessionName := s.prefix + p.AgentID
	session := &sandbox.TmuxSession{SessionName: sessionName}
	if !session.HasSession() {
		return nil, fmt.Errorf("session not found")
	}
	return nil, session.ClearBuffer()
}

func (s *MCPServer) handleSessionList(params json.RawMessage) (interface{}, error) {
	return sandbox.ListSessions(s.prefix)
}

func (s *MCPServer) handleCleanup(params json.RawMessage) (interface{}, error) {
	var p struct {
		TaskID string `json:"task_id"`
	}
	json.Unmarshal(params, &p)

	sessionName := s.prefix + p.TaskID
	session := &sandbox.TmuxSession{SessionName: sessionName}
	if session.HasSession() {
		session.Kill()
	}
	return nil, sandbox.TeardownWorktree(p.TaskID, s.pwd, "")
}

func (s *MCPServer) handleWorktreeMerge(params json.RawMessage) (interface{}, error) {
	var p struct {
		TaskID     string `json:"task_id"`
		BaseBranch string `json:"base_branch"`
	}
	json.Unmarshal(params, &p)
	if p.BaseBranch == "" {
		p.BaseBranch = "main"
	}
	return nil, sandbox.MergeWorktree(p.TaskID, p.BaseBranch, s.pwd)
}

func (s *MCPServer) handleWorktreeTeardown(params json.RawMessage) (interface{}, error) {
	var p struct {
		TaskID string `json:"task_id"`
	}
	json.Unmarshal(params, &p)
	return nil, sandbox.TeardownWorktree(p.TaskID, s.pwd, "")
}
