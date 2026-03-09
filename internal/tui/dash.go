package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"assistant-to/internal/db"
	"assistant-to/internal/sandbox"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------
// Data Structures
// ---------------------------------------------------------

type AgentStatus struct {
	SessionName   string
	LastHeartbeat time.Time
	Status        string
}

// ---------------------------------------------------------
// List Items implementations
// ---------------------------------------------------------

type taskItem struct{ db.Task }

func (t taskItem) Title() string       { return fmt.Sprintf("[%d] %s", t.ID, t.Task.Title) }
func (t taskItem) Description() string { return fmt.Sprintf("Status: %s", t.Status) }
func (t taskItem) FilterValue() string { return t.Task.Title }

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
	return fmt.Sprintf("HB: %s | %s", timeStr, status)
}
func (a agentItem) FilterValue() string { return a.SessionName }

type feedItem struct {
	AgentID   string
	EventType string
	Details   string
	Timestamp time.Time
}

func (f feedItem) Title() string {
	typeStr := f.EventType
	if typeStr == "question" {
		typeStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF")).Bold(true).Render(typeStr)
	}
	return fmt.Sprintf("[%s] %s | %s", f.Timestamp.Format("15:04:05"), f.AgentID, typeStr)
}
func (f feedItem) Description() string { return f.Details }
func (f feedItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s", f.AgentID, f.EventType, f.Details)
}

// ---------------------------------------------------------
// Main Model
// ---------------------------------------------------------

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
	taskForm  *huh.Form

	// Task Form bindings
	newTaskTitle string
	newTaskDesc  string
	newTaskDiff  string

	// State
	activePane          int // 0: Tasks, 1: Agents, 2: Feed, 3: Coordinator
	showTasksPane       bool
	showAgentsPane      bool
	showCoordinatorPane bool // Default: true
	feedSortDesc        bool
	coordinatorBuffer   string

	// Styles
	inactivePaneStyle lipgloss.Style
	activePaneStyle   lipgloss.Style
	headerStyle       lipgloss.Style
	footerStyle       lipgloss.Style
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
		db:                  database,
		projectRoot:         projectRoot,
		taskList:            tList,
		agentList:           aList,
		feedList:            fList,
		activePane:          1, // Default to agents pane (agent mode)
		showTasksPane:       true,
		showAgentsPane:      true,
		showCoordinatorPane: true, // Show coordinator by default
		feedSortDesc:        true,

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

func (m dashModel) initTaskForm() *huh.Form {
	m.newTaskTitle = ""
	m.newTaskDesc = ""
	m.newTaskDiff = "Small Feature"

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Task Title").
				Description("A concise name for this work item.").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("title cannot be empty")
					}
					return nil
				}).
				Value(&m.newTaskTitle),
			huh.NewText().
				Title("Description").
				Value(&m.newTaskDesc),
			huh.NewSelect[string]().
				Title("Difficulty").
				Options(
					huh.NewOption("Small Fix", "small_fix"),
					huh.NewOption("Small Feature", "small_feature"),
					huh.NewOption("Complex Refactor", "complex_refactor"),
					huh.NewOption("Full Module", "full_module"),
				).
				Value(&m.newTaskDiff),
		),
	).WithTheme(huh.ThemeCharm())
}

func (m dashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// If form is active, intercept EVERYTHING except window resizes
	if m.taskForm != nil {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.resizePanes()
		}

		form, cmd := m.taskForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.taskForm = f

			if m.taskForm.State == huh.StateCompleted {
				// Save to DB
				taskID, err := m.db.AddTask(m.newTaskTitle, m.newTaskDesc, "")
				if err == nil {
					spec := fmt.Sprintf("# Task %d: %s\n\n**Status:** pending\n**Difficulty:** %s\n\n## Description\n\n%s\n",
						taskID, m.newTaskTitle, m.newTaskDiff, m.newTaskDesc)
					specsDir := filepath.Join(".assistant-to", "specs")
					os.MkdirAll(specsDir, 0755)
					os.WriteFile(filepath.Join(specsDir, fmt.Sprintf("%d.md", taskID)), []byte(spec), 0644)
				}
				m.taskForm = nil
				m.refreshData()
			} else if m.taskForm.State == huh.StateAborted {
				m.taskForm = nil
			}
		}
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If filtering in a list, do not intercept keys
		if (m.activePane == 0 && m.taskList.FilterState() == list.Filtering) ||
			(m.activePane == 1 && m.agentList.FilterState() == list.Filtering) ||
			(m.activePane == 2 && m.feedList.FilterState() == list.Filtering) {
			break
		}

		switch msg.String() {
		case "q", "ctrl+c", "esc":
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
			if os.Getenv("TMUX") != "" {
				// Use tmux popup for task addition
				cmd := exec.Command("tmux", "display-popup", "-E", "-w", "80%", "-h", "80%", "dwight task add")
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return tickMsg(time.Now())
				})
			} else {
				// Fallback to internal form if not in tmux
				m.taskForm = m.initTaskForm()
				return m, m.taskForm.Init()
			}
		case "s":
			m.feedSortDesc = !m.feedSortDesc
			m.refreshData()
		case "tab", "right":
			m.activePane = (m.activePane + 1) % 3
			if m.activePane == 0 && !m.showTasksPane {
				m.activePane = 1
			}
			if m.activePane == 1 && !m.showAgentsPane {
				m.activePane = 2
			}
		case "shift+tab", "left":
			m.activePane--
			if m.activePane < 0 {
				m.activePane = 2
			}
			if m.activePane == 1 && !m.showAgentsPane {
				m.activePane = 0
			}
			if m.activePane == 0 && !m.showTasksPane {
				m.activePane = 2
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
		case "c":
			m.showCoordinatorPane = !m.showCoordinatorPane
			m.resizePanes()
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

	// Decide if we should show the coordinator pane
	// We show it if width > 160 or height > 50
	m.showCoordinatorPane = m.width > 160 || m.height > 50

	leftWidth := 0
	if m.showTasksPane {
		leftWidth = (m.width * 30) / 100
	}

	rightWidth := m.width - leftWidth
	if m.showCoordinatorPane {
		rightWidth = (rightWidth * 60) / 100
	}

	// rightWidth is now calculated based on whether the coordinator pane is shown
	topRightHeight := 0
	if m.showAgentsPane {
		topRightHeight = (m.height * 40) / 100
	}

	// Account for the global footer height (approx 2 lines)
	feedHeight := m.height - topRightHeight - 2

	if m.showTasksPane {
		m.taskList.SetSize(leftWidth-hBord, m.height-vBord-3)
	}

	if m.showAgentsPane {
		m.agentList.SetSize(rightWidth-hBord, topRightHeight-vBord)
	}

	m.feedList.SetSize(rightWidth-hBord, feedHeight-vBord)
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
			agentItems = append(agentItems, agentItem{AgentStatus{"mock-builder", time.Now(), "mock (healthy)"}})
			agentItems = append(agentItems, agentItem{AgentStatus{"mock-coordinator", time.Now().Add(-10 * time.Minute), "mock (stuck)"}})
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

				agentItems = append(agentItems, agentItem{AgentStatus{
					SessionName:   displayName,
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

	// Coordinator Live View
	if m.showCoordinatorPane {
		coordSession := prefix + "Coordinator"
		// Capture last 50 lines
		captureCmd := exec.Command("tmux", "capture-pane", "-p", "-t", coordSession, "-S", "-50")
		if out, err := captureCmd.Output(); err == nil {
			m.coordinatorBuffer = string(out)
		} else {
			m.coordinatorBuffer = "Coordinator session not found or inactive."
		}
	}
}

func boolStatus(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func (m dashModel) View() string {
	if !m.ready {
		return "\n  Initializing Dashboard..."
	}

	// If the Add Task form is active, overlay it (only when not in tmux)
	if m.taskForm != nil && os.Getenv("TMUX") == "" {
		formView := m.taskForm.View()
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 3).
			Render(formView)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}

	getStyle := func(paneIdx int) lipgloss.Style {
		if m.activePane == paneIdx {
			return m.activePaneStyle
		}
		return m.inactivePaneStyle
	}

	hBord := 4
	vBord := 2

	// 4-panel grid: [Tasks 25%] [Agents 25%] [Feed 25%] [Coord 25%]
	taskWidth := (m.width * 25) / 100
	agentWidth := (m.width * 25) / 100
	feedWidth := (m.width * 25) / 100
	coordWidth := m.width - taskWidth - agentWidth - feedWidth - hBord*4
	topHeight := (m.height * 55) / 100

	// Build each pane
	var taskPane, agentPane, feedPane, coordPane string

	if m.showTasksPane {
		taskPane = getStyle(0).Width(taskWidth - hBord).Height(m.height - vBord - 2).Render(m.taskList.View())
	}

	if m.showAgentsPane {
		agentPane = getStyle(1).Width(agentWidth - hBord).Height(m.height - vBord - 2).Render(m.agentList.View())
	}

	feedPane = getStyle(2).Width(feedWidth - hBord).Height(topHeight - vBord).Render(m.feedList.View())

	if m.showCoordinatorPane {
		coordPane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00ADD8")).
			Width(coordWidth).
			Height(topHeight-vBord).
			Padding(0, 1).
			Render(lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00ADD8")).Render("Coordinator"),
				m.coordinatorBuffer,
			))
	}

	// Top row: Tasks | Agents | Feed | Coord
	topRow := ""
	if m.showTasksPane && m.showAgentsPane && m.showCoordinatorPane {
		topRow = lipgloss.JoinHorizontal(lipgloss.Top, taskPane, agentPane, feedPane, coordPane)
	} else if m.showTasksPane && m.showAgentsPane {
		topRow = lipgloss.JoinHorizontal(lipgloss.Top, taskPane, agentPane, feedPane)
	} else if m.showTasksPane {
		topRow = lipgloss.JoinHorizontal(lipgloss.Top, taskPane, feedPane)
	} else if m.showAgentsPane {
		topRow = lipgloss.JoinHorizontal(lipgloss.Top, agentPane, feedPane)
	} else {
		topRow = feedPane
	}

	// Status indicators
	coordStatus := boolStatus(m.showCoordinatorPane)
	taskStatus := boolStatus(m.showTasksPane)
	agentStatus := boolStatus(m.showAgentsPane)

	sortStr := "DESC"
	if !m.feedSortDesc {
		sortStr = "ASC"
	}

	footerText := fmt.Sprintf(" [n] new • [enter] attach • [t] %s tasks • [a] %s agents • [c] %s coord • [s] %s • [q] quit ",
		taskStatus, agentStatus, coordStatus, sortStr)
	footer := m.footerStyle.Width(m.width).Align(lipgloss.Right).Render(footerText)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, footer)
}
