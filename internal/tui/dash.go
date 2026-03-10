package tui

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const tokenPaneWidth = 25

// ---------------------------------------------------------
// Data Structures
// ---------------------------------------------------------

type AgentStatus struct {
	SessionName   string
	TaskID        string
	TaskTitle     string
	LastHeartbeat time.Time
	Status        string
}

// ---------------------------------------------------------
// List Items implementations
// ---------------------------------------------------------

type taskItem struct{ db.Task }

func priorityIndicator(p int) string {
	switch p {
	case 1:
		return "🔥" // critical
	case 2:
		return "⚡" // high
	case 3:
		return "●" // normal
	case 4:
		return "○" // low
	case 5:
		return "◌" // trivial
	default:
		return "●"
	}
}

func statusColor(s string) lipgloss.Color {
	switch s {
	case "complete":
		return "#04B575" // green
	case "failed":
		return "#FF0000" // red
	case "building", "merging":
		return "#00BFFF" // blue
	case "review":
		return "#FFD700" // gold
	case "scouted":
		return "#9370DB" // purple
	case "started":
		return "#FFA500" // orange
	default:
		return "#A8A8A8" // gray
	}
}

func (t taskItem) Title() string {
	priority := priorityIndicator(t.Priority)
	parentInfo := ""
	if t.ParentID > 0 {
		parentInfo = fmt.Sprintf(" ↳(P:%d)", t.ParentID)
	}
	return fmt.Sprintf("[%d] %s %s%s", t.ID, priority, t.Task.Title, parentInfo)
}
func (t taskItem) Description() string {
	statusColored := lipgloss.NewStyle().Foreground(statusColor(t.Status)).Bold(true).Render(t.Status)

	var descParts []string
	descParts = append(descParts, fmt.Sprintf("Status: %s", statusColored))

	if t.Task.Description != "" {
		desc := t.Task.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		descParts = append(descParts, fmt.Sprintf("Desc: %s", desc))
	}

	if t.Task.TargetFiles != "" {
		files := t.Task.TargetFiles
		if len(files) > 50 {
			files = files[:47] + "..."
		}
		descParts = append(descParts, fmt.Sprintf("Files: %s", files))
	}

	age := time.Since(t.Task.CreatedAt)
	ageStr := formatDuration(age)
	descParts = append(descParts, fmt.Sprintf("Created: %s ago", ageStr))

	return strings.Join(descParts, " • ")
}
func (t taskItem) FilterValue() string { return t.Task.Title }

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

type agentItem struct{ AgentStatus }

func (a agentItem) Title() string { return a.SessionName }
func (a agentItem) Description() string {
	timeStr := "never seen"
	if !a.LastHeartbeat.IsZero() {
		timeStr = fmt.Sprintf("%v ago", time.Since(a.LastHeartbeat).Round(time.Second))
	}
	status := a.Status
	if status == "WAITING FOR INPUT" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true).Render(status)
	} else if strings.HasPrefix(status, "stuck") {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).Render(status)
	}

	taskInfo := ""
	if a.TaskTitle != "" {
		taskInfo = fmt.Sprintf("Task: %s | ", lipgloss.NewStyle().Foreground(lipgloss.Color("#00BFFF")).Render(a.TaskTitle))
	}

	return fmt.Sprintf("%sHB: %s | %s", taskInfo, timeStr, status)
}
func (a agentItem) FilterValue() string { return a.SessionName }

type feedItem struct {
	AgentID   string
	EventType string
	Details   string
	Timestamp time.Time
}

const maxFeedDescriptionLength = 120

func (f feedItem) Title() string {
	typeStr := f.EventType
	if typeStr == "question" {
		typeStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF")).Bold(true).Render(typeStr)
	}
	return fmt.Sprintf("[%s] %s | %s", f.Timestamp.Format("15:04:05"), f.AgentID, typeStr)
}
func (f feedItem) Description() string {
	details := f.Details
	if len(details) > maxFeedDescriptionLength {
		details = details[:maxFeedDescriptionLength-3] + "..."
	}
	return details
}
func (f feedItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s", f.AgentID, f.EventType, f.Details)
}

// ---------------------------------------------------------
// Main Model
// ---------------------------------------------------------

type TokenSummary struct {
	TotalTokens           int64
	TotalCostUSD          float64
	TotalPromptTokens     int64
	TotalCompletionTokens int64
	AgentCount            int
	TopConsumers          []db.TokenMetrics
}

type dashModel struct {
	db          *db.DB
	projectRoot string
	width       int
	height      int
	ready       bool

	// Components
	taskList  list.Model
	agentList list.Model
	feedList  list.Model

	// State
	activePane      int // 0: Tasks, 1: Agents, 2: Feed
	showTasksPane   bool
	showAgentsPane  bool
	showTokensPane  bool
	feedSortDesc    bool
	showQuitConfirm bool
	quitConfirmed   bool

	// Token data
	tokenSummary TokenSummary

	// Coordinator info
	coordinatorRunning bool
	apiPort            int
	mcpPort            int
	serverSessions     []string

	// Styles
	inactivePaneStyle lipgloss.Style
	activePaneStyle   lipgloss.Style
	headerStyle       lipgloss.Style
	footerStyle       lipgloss.Style
	confirmStyle      lipgloss.Style
}

type tickMsg time.Time

func NewDashModel(database *db.DB, projectRoot string) tea.Model {

	tList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	tList.Title = "Task Board"
	tList.SetShowStatusBar(false)

	aList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	aList.Title = "Agent Status"
	aList.SetShowStatusBar(false)

	fList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	fList.Title = "Events Feed"
	fList.SetShowStatusBar(true) // Helpful for filtering counts

	m := dashModel{
		db:             database,
		projectRoot:    projectRoot,
		taskList:       tList,
		agentList:      aList,
		feedList:       fList,
		activePane:     1, // Default to agents pane
		showTasksPane:  true,
		showAgentsPane: true,
		showTokensPane: true,
		feedSortDesc:   true,

		inactivePaneStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#565656")).
			Padding(0, 1),
		activePaneStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1),
		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1),
		footerStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8A8A8")).
			MarginTop(1),
		confirmStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#E8183C")).
			Background(lipgloss.Color("#1a1a1a")).
			Foreground(lipgloss.Color("#FAFAFA")).
			Padding(1, 2).
			Bold(true),
	}
	return m
}

func (m dashModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.tick(),
	)
}

func (m dashModel) tick() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m dashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle quit confirmation dialog first
		if m.showQuitConfirm {
			switch msg.String() {
			case "y", "Y", "q", "ctrl+c":
				// Kill all project sessions before quitting
				m.cleanupProjectSessions()
				return m, tea.Quit
			case "n", "N", "esc":
				m.showQuitConfirm = false
				return m, nil
			}
			return m, nil
		}

		// If filtering in a list, do not intercept keys
		if (m.activePane == 0 && m.taskList.FilterState() == list.Filtering) ||
			(m.activePane == 1 && m.agentList.FilterState() == list.Filtering) ||
			(m.activePane == 2 && m.feedList.FilterState() == list.Filtering) {
			break
		}

		switch msg.String() {
		case "q", "ctrl+c":
			if !m.showQuitConfirm {
				m.showQuitConfirm = true
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			if m.activePane == 1 {
				if i, ok := m.agentList.SelectedItem().(agentItem); ok {
					pwd := m.projectRoot
					fullSession := sandbox.ProjectPrefix(pwd) + i.SessionName

					tmuxCmd := "attach-session"
					if os.Getenv("TMUX") != "" {
						tmuxCmd = "switch-client"
					}
					c := exec.Command("tmux", tmuxCmd, "-t", fullSession)

					return m, tea.ExecProcess(c, func(err error) tea.Msg {
						return tickMsg(time.Now())
					})
				}
			}
		case "n":
			exePath, err := os.Executable()
			if err != nil {
				exePath = "dwight"
			}
			c := exec.Command(exePath, "task", "add")
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return tickMsg(time.Now())
			})
		case "s":
			m.feedSortDesc = !m.feedSortDesc
			m.refreshData()
		case "tab", "right":
			m.activePane = (m.activePane + 1) % 4
			if m.activePane == 0 && !m.showTasksPane {
				m.activePane = 1
			}
			if m.activePane == 1 && !m.showAgentsPane {
				m.activePane = 2
			}
			if m.activePane == 2 && !m.showTokensPane {
				m.activePane = 3
			}
		case "shift+tab", "left":
			m.activePane--
			if m.activePane < 0 {
				m.activePane = 3
			}
			if m.activePane == 3 && !m.showTokensPane {
				m.activePane = 2
			}
			if m.activePane == 2 && !m.showTokensPane {
				m.activePane = 1
			}
			if m.activePane == 1 && !m.showAgentsPane {
				m.activePane = 0
			}
			if m.activePane == 0 && !m.showTasksPane {
				m.activePane = 1
			}
		case "t":
			m.showTasksPane = !m.showTasksPane
			if !m.showTasksPane && m.activePane == 0 {
				m.activePane = 2
			}
			m.resizePanes()
		case "a":
			m.showAgentsPane = !m.showAgentsPane
			if !m.showAgentsPane && m.activePane == 1 {
				m.activePane = 2
			}
			m.resizePanes()
		case "k":
			m.showTokensPane = !m.showTokensPane
			m.resizePanes()
		case "c":
			// Open Coordinator in tmux popup for interaction
			if os.Getenv("TMUX") != "" {
				prefix := sandbox.ProjectPrefix(m.projectRoot)
				coordSession := prefix + "coordinator"
				cmd := exec.Command("tmux", "display-popup", "-E", "-w", "80%", "-h", "80%", "-t", coordSession)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return tickMsg(time.Now())
				})
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			m.ready = true
			m.refreshData()
		}
		m.resizePanes()

	case tickMsg:
		m.refreshData()
		return m, m.tick()
	}

	// Route updates
	if m.ready {
		switch m.activePane {
		case 0:
			if m.showTasksPane {
				m.taskList, cmd = m.taskList.Update(msg)
				cmds = append(cmds, cmd)
			}
		case 1:
			if m.showAgentsPane {
				m.agentList, cmd = m.agentList.Update(msg)
				cmds = append(cmds, cmd)
			}
		case 2:
			m.feedList, cmd = m.feedList.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *dashModel) resizePanes() {
	if !m.ready {
		return
	}

	hBord := 4
	vBord := 2

	// Layout: Tasks (left), Agents/Feed (center), Tokens (right)
	tokensWidth := 0
	if m.showTokensPane {
		tokensWidth = tokenPaneWidth
	}

	leftWidth := 0
	if m.showTasksPane {
		leftWidth = (m.width * 30) / 100
	}

	centerWidth := m.width - leftWidth - tokensWidth

	// Split center into Agents (top 45%) and Feed (bottom 55%)
	agentsHeight := 0
	if m.showAgentsPane {
		agentsHeight = (m.height * 45) / 100
	}

	// Feed takes remaining space (minus footer)
	feedHeight := m.height - agentsHeight - 3

	if m.showTasksPane {
		m.taskList.SetSize(leftWidth-hBord, m.height-vBord-3)
	}

	if m.showAgentsPane {
		m.agentList.SetSize(centerWidth-hBord, agentsHeight-vBord)
	}

	m.feedList.SetSize(centerWidth-hBord, feedHeight-vBord)
}

func (m *dashModel) refreshData() {
	// Tasks
	tasks, err := m.db.ListTasksByStatus("")
	if err == nil {
		sort.Slice(tasks, func(i, j int) bool {
			order := map[string]int{"started": 0, "scouted": 1, "building": 2, "review": 3, "pending": 4, "complete": 5}
			return order[tasks[i].Status] < order[tasks[j].Status]
		})

		var items []list.Item
		for _, t := range tasks {
			items = append(items, taskItem{t})
		}
		m.taskList.SetItems(items)
	}

	// Agents
	var agentItems []list.Item
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()

	pwd := m.projectRoot
	prefix := sandbox.ProjectPrefix(pwd)

	// Add mock agents directly from DB if any exist (used for testing without tmux)
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		mockLog, _ := m.db.GetAgentHistory("mock-builder")
		if len(mockLog) > 0 {
			agentItems = append(agentItems, agentItem{AgentStatus{"mock-builder", "1", "Mock Task 1", time.Now(), "mock (healthy)"}})
			agentItems = append(agentItems, agentItem{AgentStatus{"mock-coordinator", "", "", time.Now().Add(-10 * time.Minute), "mock (stuck)"}})
		}
	} else {
		sessions := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, s := range sessions {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, prefix) {
				fullAgentID := s
				displayName := strings.TrimPrefix(s, prefix)

				// Get last event and its type
				var lastSeen time.Time
				var lastType string
				query := "SELECT timestamp, event_type FROM events WHERE agent_id = ? ORDER BY timestamp DESC LIMIT 1"
				err := m.db.QueryRow(query, fullAgentID).Scan(&lastSeen, &lastType)

				status := "healthy"
				if err == nil && !lastSeen.IsZero() {
					if lastType == "question" {
						status = "WAITING FOR INPUT"
					} else if time.Since(lastSeen) > 5*time.Minute {
						status = "stuck (>5m)"
					}
				} else {
					status = "unknown"
				}

				// Try to extract task ID from display name (e.g., "builder-1", "scout-2")
				taskID := ""
				taskTitle := ""
				parts := strings.Split(displayName, "-")
				if len(parts) >= 2 {
					potentialID := parts[len(parts)-1]
					if id, err := strconv.Atoi(potentialID); err == nil {
						taskID = potentialID
						if task, err := m.db.GetTaskByID(id); err == nil {
							taskTitle = task.Title
						}
					}
				}

				agentItems = append(agentItems, agentItem{AgentStatus{
					SessionName:   displayName,
					TaskID:        taskID,
					TaskTitle:     taskTitle,
					LastHeartbeat: lastSeen,
					Status:        status,
				}})
			}
		}
	}
	m.agentList.SetItems(agentItems)

	// Feed
	query := "SELECT agent_id, event_type, details, timestamp FROM events"
	if m.feedSortDesc {
		query += " ORDER BY timestamp DESC LIMIT 100"
	} else {
		query += " ORDER BY timestamp ASC LIMIT 100"
	}

	rows, err := m.db.Query(query)
	var feedItems []list.Item
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var f feedItem
			if err := rows.Scan(&f.AgentID, &f.EventType, &f.Details, &f.Timestamp); err == nil {
				feedItems = append(feedItems, f)
			}
		}
	}
	m.feedList.SetItems(feedItems)

	// Token Metrics
	if m.showTokensPane {
		summary, err := m.db.GetTokenMetricsSummary()
		if err == nil {
			m.tokenSummary.TotalTokens = summary["total_tokens"].(int64)
			m.tokenSummary.TotalCostUSD = summary["total_cost_usd"].(float64)
			m.tokenSummary.TotalPromptTokens = summary["total_prompt_tokens"].(int64)
			m.tokenSummary.TotalCompletionTokens = summary["total_completion_tokens"].(int64)
			m.tokenSummary.AgentCount = summary["agent_count"].(int)
		}
		topConsumers, err := m.db.GetTopTokenConsumers(5)
		if err == nil {
			m.tokenSummary.TopConsumers = topConsumers
		}
	}

	// Coordinator Status
	coordSession := prefix + "coordinator"
	coordCheck := exec.Command("tmux", "has-session", "-t", coordSession)
	m.coordinatorRunning = coordCheck.Run() == nil

	// Get project-specific ports
	apiPort, mcpPort := getProjectPorts(m.projectRoot)
	m.apiPort = apiPort
	m.mcpPort = mcpPort

	// Get server sessions
	var sessions []string
	serversSession := prefix + "servers"
	serversCheck := exec.Command("tmux", "has-session", "-t", serversSession)
	if serversCheck.Run() == nil {
		sessions = append(sessions, "servers")
	}
	if m.coordinatorRunning {
		sessions = append(sessions, "coordinator")
	}
	m.serverSessions = sessions
}

// getProjectPorts calculates project-specific ports
func getProjectPorts(projectPath string) (apiPort, mcpPort int) {
	basePort := 15000
	maxPort := 65000

	hash := sha256.Sum256([]byte(projectPath))
	offset := int(binary.BigEndian.Uint32(hash[:4]))

	portRange := maxPort - basePort
	apiPort = basePort + (offset % portRange)
	mcpPort = apiPort + 1
	if mcpPort > maxPort {
		mcpPort = basePort
	}
	return apiPort, mcpPort
}

func boolStatus(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

// cleanupProjectSessions kills all tmux sessions for this project
func (m *dashModel) cleanupProjectSessions() {
	prefix := sandbox.ProjectPrefix(m.projectRoot)

	// List all tmux sessions
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return // No tmux server or no sessions
	}

	sessions := strings.Split(strings.TrimSpace(string(out)), "\n")
	killedCount := 0

	for _, session := range sessions {
		session = strings.TrimSpace(session)
		// Kill sessions managed by this project's orchestrator (including task-add)
		if strings.HasPrefix(session, prefix) {
			killCmd := exec.Command("tmux", "kill-session", "-t", session)
			if err := killCmd.Run(); err == nil {
				killedCount++
			}
		}
	}

	// Log the cleanup
	if killedCount > 0 {
		m.db.RecordEvent("dashboard", "cleanup", fmt.Sprintf("Killed %d project sessions on exit", killedCount))
	}
}

func (m dashModel) View() string {
	if !m.ready {
		return "\n  Initializing Dashboard..."
	}

	// Show quit confirmation dialog
	if m.showQuitConfirm {
		confirmText := "⚠️  Quit and stop all agents?\n\nThis will kill:\n• Coordinator\n• All builder agents\n• API/MCP servers\n\n[y] Yes, stop everything  [n] No, keep running"
		confirmBox := m.confirmStyle.Width(50).Render(confirmText)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, confirmBox)
	}

	getStyle := func(paneIdx int) lipgloss.Style {
		if m.activePane == paneIdx {
			return m.activePaneStyle
		}
		return m.inactivePaneStyle
	}

	hBord := 4
	vBord := 2

	// Layout: [Tasks 30%] [Center: Agents/Feed] [Tokens 25]
	tokensWidth := 0
	if m.showTokensPane {
		tokensWidth = tokenPaneWidth
	}

	taskWidth := 0
	if m.showTasksPane {
		taskWidth = (m.width * 30) / 100
	}
	centerWidth := m.width - taskWidth - tokensWidth

	agentsHeight := 0
	if m.showAgentsPane {
		agentsHeight = (m.height * 45) / 100
	}
	feedHeight := m.height - agentsHeight - 3

	// Build each pane
	var taskPane, agentPane, feedPane, tokenPane string

	if m.showTasksPane {
		taskPane = getStyle(0).Width(taskWidth - hBord).Height(m.height - vBord - 3).Render(m.taskList.View())
	}

	if m.showAgentsPane {
		agentPane = getStyle(1).Width(centerWidth - hBord).Height(agentsHeight - vBord).Render(m.agentList.View())
	}

	feedPane = getStyle(2).Width(centerWidth - hBord).Height(feedHeight - vBord).Render(m.feedList.View())

	// Token pane
	if m.showTokensPane {
		tokenPane = m.renderTokenPane(tokensWidth - hBord)
	}

	// Center column: Agents on top, Feed on bottom
	centerColumn := lipgloss.JoinVertical(lipgloss.Left, agentPane, feedPane)

	// Main layout
	var mainRow string
	if m.showTasksPane && m.showTokensPane {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, taskPane, centerColumn, tokenPane)
	} else if m.showTasksPane {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, taskPane, centerColumn)
	} else if m.showTokensPane {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, centerColumn, tokenPane)
	} else {
		mainRow = centerColumn
	}

	// Status indicators
	taskStatus := boolStatus(m.showTasksPane)
	agentStatus := boolStatus(m.showAgentsPane)
	tokenStatus := boolStatus(m.showTokensPane)

	// Coordinator status for header
	coordIndicator := "✗"
	coordColor := "#E8183C"
	if m.coordinatorRunning {
		coordIndicator = "●"
		coordColor = "#04B575"
	}

	sortStr := "DESC"
	if !m.feedSortDesc {
		sortStr = "ASC"
	}

	// Header with coordinator status and token summary
	tokenStr := ""
	if m.tokenSummary.TotalTokens > 0 {
		tokenStr = fmt.Sprintf(" | Tokens: %s ($%.2f)", formatTokens(m.tokenSummary.TotalTokens), m.tokenSummary.TotalCostUSD)
	}
	headerText := fmt.Sprintf(" Coordinator: %s | API: %d | MCP: %d%s ",
		lipgloss.NewStyle().Foreground(lipgloss.Color(coordColor)).Render(coordIndicator),
		m.apiPort, m.mcpPort, tokenStr)
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(m.width).
		Render(headerText)

	// Footer with keybindings
	footerText := fmt.Sprintf(" [n] new task • [enter] attach agent • [c] coordinator • [t] %s tasks • [a] %s agents • [k] %s tokens • [s] %s • [q] quit ",
		taskStatus, agentStatus, tokenStatus, sortStr)
	footer := m.footerStyle.Width(m.width).Align(lipgloss.Right).Render(footerText)

	return lipgloss.JoinVertical(lipgloss.Left, header, mainRow, footer)
}

func (m dashModel) renderTokenPane(width int) string {
	style := m.inactivePaneStyle.Width(width).Height(m.height - 5)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD700"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A8A8A8"))

	var lines []string
	lines = append(lines, titleStyle.Render("TOKEN USAGE"))

	lines = append(lines, "")
	lines = append(lines, valueStyle.Render(fmt.Sprintf("Total: %s", formatTokens(m.tokenSummary.TotalTokens))))
	lines = append(lines, labelStyle.Render(fmt.Sprintf("Cost: $%.4f", m.tokenSummary.TotalCostUSD)))

	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Prompt:"))
	lines = append(lines, valueStyle.Render(formatTokens(m.tokenSummary.TotalPromptTokens)))

	lines = append(lines, "")
	lines = append(lines, labelStyle.Render("Completion:"))
	lines = append(lines, valueStyle.Render(formatTokens(m.tokenSummary.TotalCompletionTokens)))

	lines = append(lines, "")
	lines = append(lines, labelStyle.Render(fmt.Sprintf("Agents: %d", m.tokenSummary.AgentCount)))

	if len(m.tokenSummary.TopConsumers) > 0 {
		lines = append(lines, "")
		lines = append(lines, labelStyle.Render("Top Consumers:"))
		for _, tc := range m.tokenSummary.TopConsumers {
			agentName := tc.AgentID
			if idx := strings.LastIndex(agentName, "/"); idx != -1 {
				agentName = agentName[idx+1:]
			}
			lines = append(lines, valueStyle.Render(fmt.Sprintf("  %s: %s", agentName, formatTokens(int64(tc.TotalTokens)))))
		}
	}

	content := lipgloss.NewStyle().Width(width - 2).Render(strings.Join(lines, "\n"))
	return style.Render(content)
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
