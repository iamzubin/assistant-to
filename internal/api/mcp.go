package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"assistant-to/internal/config"
	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"
	"assistant-to/internal/tasking"
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

// MCPToolInfo is used for JSON serialization (excludes Handler function)
type MCPToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
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
			InputSchema: json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string"},"type":{"type":"string"},"details":{"type":"string"}},"required":["details"]}`),
			Handler:     s.handleLog,
		},
		{
			Name:        "event_list",
			Description: "List recent swarm events",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string"},"limit":{"type":"integer"}}}`),
			Handler:     s.handleEventList,
		},
		{
			Name:        "expertise_list",
			Description: "Search or list shared project knowledge/expertise",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"domain":{"type":"string"},"query":{"type":"string"}}}`),
			Handler:     s.handleExpertiseList,
		},
		{
			Name:        "expertise_record",
			Description: "Record a new piece of project knowledge/expertise",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"domain":{"type":"string"},"type":{"type":"string","enum":["convention","pattern","failure","decision"]},"description":{"type":"string"}},"required":["domain","type","description"]}`),
			Handler:     s.handleExpertiseRecord,
		},
		{
			Name:        "task_list",
			Description: "List tasks, optionally filtered by status. Omitting status returns ALL tasks across the entire lifecycle.",
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
			Name:        "task_add",
			Description: "Add a new task to the swarm",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"},"description":{"type":"string"},"target_files":{"type":"string"},"difficulty":{"type":"string","enum":["small_fix","small_feature","complex_refactor","full_module"]},"parent_id":{"type":"integer"}},"required":["title","description"]}`),
			Handler:     s.handleTaskAdd,
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
		{
			Name:        "agent_spawn",
			Description: "Spawn a new agent for a task (role: Builder, Scout, Reviewer, Merger)",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"string"},"role":{"type":"string"}},"required":["task_id","role"]}`),
			Handler:     s.handleAgentSpawn,
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
			ID:      nil,
		})
		return
	}

	resp := s.ProcessRequest(req)
	if resp != nil {
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("Error encoding MCP response: %v", err)
		}
	}
}

// ProcessRequest handles an MCP request and returns a response
// This can be used for both HTTP and stdio communication
func (s *MCPServer) ProcessRequest(req MCPRequest) *MCPResponse {
	// Log incoming request to stderr for debugging (won't interfere with stdio JSON-RPC)
	fmt.Fprintf(os.Stderr, "[MCP] Processing method: %s (ID: %v)\n", req.Method, req.ID)

	var resp MCPResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "tools/list":
		s.mu.RLock()
		tools := s.tools
		s.mu.RUnlock()

		// Convert MCPTool to MCPToolInfo for JSON serialization
		toolInfos := make([]MCPToolInfo, len(tools))
		for i, t := range tools {
			toolInfos[i] = MCPToolInfo{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			}
		}

		resp.Result = map[string]interface{}{
			"tools": toolInfos,
		}
	case "tools/call":
		var params struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &MCPError{Code: -32602, Message: "Invalid params"}
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
				resp.Error = &MCPError{Code: -32000, Message: toolErr.Error()}
			} else {
				resp.Result = map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": fmt.Sprintf("%v", result)}}}
			}
		}
	case "initialize":
		// MCP initialize method - return server capabilities
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "assistant-to",
				"version": "1.0.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"listChanged": true,
				},
			},
		}
	case "notifications/initialized", "initialized":
		// No-op for initialized notification
		fmt.Fprintf(os.Stderr, "[MCP] Received initialized notification\n")
		return nil
	default:
		// If it's a notification (ID is nil), don't return an error response
		if req.ID == nil {
			fmt.Fprintf(os.Stderr, "[MCP] Ignoring unknown notification: %s\n", req.Method)
			return nil
		}
		resp.Error = &MCPError{Code: -32601, Message: "Method not found"}
	}

	return &resp
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
		AgentID string `json:"agent_id"`
		Type    string `json:"type"`
		Details string `json:"details"`
	}
	json.Unmarshal(params, &p)
	if p.AgentID == "" {
		p.AgentID = "unknown"
	}
	if p.Type == "" {
		p.Type = "info"
	}
	s.db.RecordEvent(p.AgentID, p.Type, p.Details)
	return "logged", nil
}

func (s *MCPServer) handleEventList(params json.RawMessage) (interface{}, error) {
	var p struct {
		AgentID string `json:"agent_id"`
		Limit   int    `json:"limit"`
	}
	json.Unmarshal(params, &p)

	if p.AgentID != "" {
		return s.db.GetAgentHistory(p.AgentID)
	}
	return s.db.GetAllEvents(p.Limit)
}

func (s *MCPServer) handleExpertiseList(params json.RawMessage) (interface{}, error) {
	var p struct {
		Domain string `json:"domain"`
		Query  string `json:"query"`
	}
	json.Unmarshal(params, &p)

	if p.Query != "" {
		return s.db.SearchExpertise(p.Query)
	}
	if p.Domain != "" {
		return s.db.GetExpertiseByDomain(p.Domain)
	}
	return s.db.GetAllExpertise()
}

func (s *MCPServer) handleExpertiseRecord(params json.RawMessage) (interface{}, error) {
	var p struct {
		Domain      string `json:"domain"`
		Type        string `json:"type"`
		Description string `json:"description"`
	}
	json.Unmarshal(params, &p)

	if p.Domain == "" || p.Type == "" || p.Description == "" {
		return nil, fmt.Errorf("domain, type, and description are required")
	}

	id, err := s.db.RecordExpertise(p.Domain, p.Type, p.Description)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": id, "status": "recorded"}, nil
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

func (s *MCPServer) handleTaskAdd(params json.RawMessage) (interface{}, error) {
	var p struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		TargetFiles string `json:"target_files"`
		Difficulty  string `json:"difficulty"`
		ParentID    int    `json:"parent_id"`
	}
	json.Unmarshal(params, &p)

	if p.Difficulty == "" {
		p.Difficulty = "small_feature"
	}

	// Insert the task
	taskID, err := s.db.AddTask(p.Title, p.Description, p.TargetFiles, p.ParentID)
	if err != nil {
		return nil, fmt.Errorf("failed to add task to DB: %w", err)
	}

	// Ensure specs directory exists
	specsDir := filepath.Join(s.pwd, ".assistant-to", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		s.db.RemoveTask(int(taskID))
		return nil, fmt.Errorf("failed to create specs directory: %w", err)
	}

	// Generate AT_INSTRUCTIONS.md
	specPath := filepath.Join(specsDir, fmt.Sprintf("%d.md", taskID))
	if err := tasking.GenerateTaskInstructions(s.pwd, int(taskID), p.Title, p.Description, p.TargetFiles, p.Difficulty, specPath); err != nil {
		s.db.RemoveTask(int(taskID))
		return nil, fmt.Errorf("failed to generate AT_INSTRUCTIONS.md: %w", err)
	}

	return map[string]interface{}{
		"task_id":   taskID,
		"spec_path": specPath,
		"status":    "created",
	}, nil
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

func (s *MCPServer) handleAgentSpawn(params json.RawMessage) (interface{}, error) {
	var p struct {
		TaskID string `json:"task_id"`
		Role   string `json:"role"`
	}
	json.Unmarshal(params, &p)

	// Get absolute path to the dwight executable
	exePath, err := os.Executable()
	if err != nil {
		exePath = "dwight" // Fallback
	}
	if absPath, err := filepath.Abs(exePath); err == nil {
		exePath = absPath
	}

	// Use exec.Command to run 'dwight run' which handles all the worktree and session setup
	// We run it as a detached process or just wait for it to return after it spawns the tmux session
	cmd := exec.Command(exePath, "run", p.TaskID, "--role", p.Role)
	cmd.Dir = s.pwd // Run from project root

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to spawn agent: %v (output: %s)", err, output)
	}

	return string(output), nil
}
