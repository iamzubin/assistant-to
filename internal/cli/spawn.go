package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"dwight/internal/config"
	"dwight/internal/db"
	"dwight/internal/sandbox"
	"dwight/internal/tasking"

	"github.com/spf13/cobra"
)

var (
	spawnModel  string
	spawnRole   string
	spawnPrompt string
	spawnTool   string
)

var runCmd = &cobra.Command{
	Use:   "run <task-id>",
	Short: "Run an agent for a specific task",
	Long:  `Creates an isolated worktree and spawns a new tmux session for an agent targeting the specified task.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		// Determine the working directory for the agent session
		// Most agents (Builder, Scout, Reviewer) work in the task's worktree
		// But Merger needs to run from the main repo to perform git merge
		worktreeDir := filepath.Join(pwd, ".dwight", "worktrees", taskID)
		if taskID == "Coordinator" {
			worktreeDir = pwd
		} else if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
			// Skip worktree creation for Merger - it runs from main repo
			if spawnRole != "Merger" {
				fmt.Printf("Worktree not found, attempting to create it on 'main'...\n")
				_, err = sandbox.CreateWorktree(pwd, taskID, "main", "")
				if err != nil {
					fmt.Printf("Failed to create worktree: %v\n", err)
					os.Exit(1)
				}
			}
		}

		// Determine session directory: Merger runs from main repo, others from worktree
		sessionDir := worktreeDir
		if spawnRole == "Merger" {
			sessionDir = pwd
		}

		// Load config to determine the tool
		configPath := filepath.Join(pwd, ".dwight", "config.yaml")
		conf, err := config.Load(configPath)
		if err != nil {
			fmt.Printf("Warning: failed to load workspace config, using defaults.\n")
			conf = config.Default()
		}

		tool := spawnTool
		if tool == "" {
			// Fall back to config tool, then role-specific runtime, then default
			if conf != nil && conf.Tool != "" {
				tool = conf.Tool
			} else if conf != nil {
				tool = conf.RuntimeForRole(spawnRole)
			}
			if tool == "" {
				tool = "gemini"
			}
		}

		model := spawnModel
		if model == "auto" || model == "" {
			if conf != nil && conf.LastModel != "" {
				model = conf.LastModel
			} else if conf != nil {
				model = conf.ModelForRole(spawnRole)
			}
		}

		// Update last used model if explicitly provided
		if spawnModel != "" && spawnModel != "auto" && conf != nil {
			conf.LastModel = spawnModel
			conf.Save(configPath)
		}

		// Calculate project-specific ports
		apiPort, mcpPort := conf.GetProjectPorts(pwd)

		// Ensure config reflects these project-specific ports even if load failed
		conf.API.Port = apiPort
		conf.API.MCPPort = mcpPort

		// Normalize role (capitalize first letter) to match LoadPrompts convention
		role := spawnRole
		if len(role) > 0 {
			role = strings.ToUpper(role[:1]) + strings.ToLower(role[1:])
		}
		mcpRole := strings.ToLower(role)

		suffix := taskID
		if role != "" && role != "Coordinator" {
			suffix = strings.ToLower(role) + "-" + taskID
		}
		sessionName := sandbox.ProjectPrefix(pwd) + suffix

		// Load prompt from agents.md if not provided
		finalPrompt := spawnPrompt
		if finalPrompt == "" {
			// Look for prompts directory
			promptsPath := filepath.Join(pwd, ".dwight", "prompts")
			if _, err := os.Stat(promptsPath); os.IsNotExist(err) {
				promptsPath = filepath.Join(pwd, "internal", "orchestrator", "prompts")
			}
			prompts, err := tasking.LoadPrompts(promptsPath)
			if err == nil {
				finalPrompt = prompts.Get(role)
				if finalPrompt == "" {
					fmt.Printf("Warning: no prompt found for role %q\n", role)
				}

				// Inject MCP documentation for ALL roles that have it
				mcpContent := prompts.GetMCP(mcpRole)
				if mcpContent != "" {
					mcpContent = strings.ReplaceAll(mcpContent, "{{.MCPPort}}", fmt.Sprintf("%d", mcpPort))
					mcpContent = strings.ReplaceAll(mcpContent, "{{.APIPort}}", fmt.Sprintf("%d", apiPort))
					mcpContent = strings.ReplaceAll(mcpContent, "{{.TaskID}}", taskID)
					finalPrompt = finalPrompt + "\n\n" + mcpContent
				}
			} else {
				fmt.Printf("Warning: failed to load prompts: %v\n", err)
			}
		}

		// If it's a numeric task ID, enrich the prompt with task details from DB
		if id, err := strconv.Atoi(taskID); err == nil {
			dbPath := filepath.Join(pwd, ".dwight", "state.db")
			database, err := db.Open(dbPath)
			if err != nil {
				fmt.Printf("Warning: failed to open state database for task enrichment: %v\n", err)
			} else {
				defer database.Close()
				task, err := database.GetTaskByID(id)
				if err != nil {
					fmt.Printf("Warning: failed to find task %d in database: %v\n", id, err)
				} else {
					// Enrich the prompt with task details (matching Coordinator logic)
					finalPrompt = fmt.Sprintf("%s\n\n---\n\n## Your Task (ID: %d)\n\n**Title:** %s\n\n**Description:**\n%s\n\n**Target Files:**\n%s",
						finalPrompt, task.ID, task.Title, task.Description, task.TargetFiles)
				}
			}
		}

		// Write prompt to a mission file in the worktree
		missionPath := filepath.Join(worktreeDir, ".mission.md")
		if err := os.WriteFile(missionPath, []byte(finalPrompt), 0644); err != nil {
			fmt.Printf("Warning: failed to write mission file: %v\n", err)
		}

		// Calculate absolute path to the dwight executable
		exePath, err := os.Executable()
		if err != nil {
			exePath = "dwight" // Fallback
		}
		if absPath, err := filepath.Abs(exePath); err == nil {
			exePath = absPath
		}

		// Generate MCP config files in the worktree (opencode.json, .gemini/settings.json, etc.)
		// This ensures AI tools (like gemini-cli or opencode) can connect to the local MCP server.
		mcpRole = strings.ToLower(role)
		mcpPortForRole := conf.MCPPortForRole(mcpRole)

		// Generate opencode.json
		opencodeConfig := map[string]interface{}{
			"$schema": "https://opencode.ai/config.json",
			"mcp": map[string]interface{}{
				"dwight": map[string]interface{}{
					"type":    "local",
					"command": []string{exePath, "mcp", "serve"},
					"enabled": true,
					"environment": map[string]string{
						"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPortForRole),
						"AT_AGENT_ROLE":   role,
						"AT_TASK_ID":      taskID,
						"AT_PROJECT_ROOT": pwd,
					},
				},
			},
		}
		opencodeCfgPath := filepath.Join(worktreeDir, "opencode.json")
		opencodeData, _ := json.MarshalIndent(opencodeConfig, "", "  ")
		os.WriteFile(opencodeCfgPath, opencodeData, 0644)

		// Generate Gemini settings.json
		geminiConfig := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"dwight": map[string]interface{}{
					"command": exePath,
					"args":    []string{"mcp", "serve"},
					"env": map[string]string{
						"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPortForRole),
						"AT_AGENT_ROLE":   role,
						"AT_TASK_ID":      taskID,
						"AT_PROJECT_ROOT": pwd,
					},
					"trust": true,
				},
			},
		}
		geminiSettingsPath := filepath.Join(worktreeDir, ".gemini", "settings.json")
		os.MkdirAll(filepath.Dir(geminiSettingsPath), 0755)
		geminiData, _ := json.MarshalIndent(geminiConfig, "", "  ")
		os.WriteFile(geminiSettingsPath, geminiData, 0644)

		// Generate generic mcp.json
		mcpConfig := map[string]interface{}{
			"name":        "dwight",
			"description": fmt.Sprintf("Assistant-to %s agent MCP server", role),
			"transport":   "stdio",
			"command":     exePath,
			"args":        []string{"mcp", "serve"},
			"env": map[string]string{
				"AT_MCP_PORT":     fmt.Sprintf("%d", mcpPortForRole),
				"AT_AGENT_ROLE":   role,
				"AT_TASK_ID":      taskID,
				"AT_PROJECT_ROOT": pwd,
			},
		}
		mcpCfgPath := filepath.Join(worktreeDir, "mcp.json")
		mcpData, _ := json.MarshalIndent(mcpConfig, "", "  ")
		os.WriteFile(mcpCfgPath, mcpData, 0644)

		geminiPath, err := exec.LookPath("gemini")
		if err != nil {
			geminiPath = "gemini" // Fallback
		}

		opencodePath, err := exec.LookPath("opencode")
		if err != nil {
			opencodePath = "opencode" // Fallback
		}

		var agentCmd string
		modelFlag := ""
		if model != "auto" && model != "" {
			modelFlag = fmt.Sprintf("--model %s ", model)
		}

		switch tool {
		case "gemini":
			// -i (prompt-interactive) takes the prompt as its argument.
			// --approval-mode=yolo allows tools to run without manual confirmation.
			// We use $(cat ...) inside the shell to avoid passing huge strings through tmux send-keys
			agentCmd = fmt.Sprintf("%s %s--approval-mode=yolo -i \"$(cat .mission.md)\"", geminiPath, modelFlag)
		case "opencode":
			// opencode [project] --prompt [prompt]
			// We run it in the current directory (.) and pass the mission as prompt
			agentCmd = fmt.Sprintf("%s . %s--prompt \"$(cat .mission.md)\"", opencodePath, modelFlag)
		default:
			// Generic fallback
			agentCmd = fmt.Sprintf("%s %s--prompt \"$(cat .mission.md)\"", tool, modelFlag)
		}

		// Update mission status if it's a numeric task ID
		if id, err := strconv.Atoi(taskID); err == nil {
			dbPath := filepath.Join(pwd, ".dwight", "state.db")
			database, err := db.Open(dbPath)
			if err == nil {
				database.UpdateTaskStatus(id, "started")
				database.Close()
			}
		}

		session := sandbox.TmuxSession{
			SessionName: sessionName,
			WorktreeDir: sessionDir,
			Command:     agentCmd,
			EnvVars: map[string]string{
				"AT_MCP_PORT":                     fmt.Sprintf("%d", mcpPort),
				"AT_PROJECT_ROOT":                 pwd,
				"AT_AGENT_ROLE":                   role,
				"AT_TASK_ID":                      taskID,
				"GEMINI_CLI_SYSTEM_SETTINGS_PATH": filepath.Join(worktreeDir, ".gemini", "settings.json"),
			},
		}

		fmt.Printf("Spawning tmux session: %s\n", sessionName)
		if err := session.Start(cmd.Context()); err != nil {
			fmt.Printf("Error spawning session: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Success! Run 'dwight connect %s' to attach.\n", taskID)
	},
}

var connectCmd = &cobra.Command{
	Use:   "connect <target>",
	Short: "Connect to an active agent's tmux session",
	Long: `Attaches your terminal to the tmux session of an actively running agent.
Target can be a task ID (e.g., 1) or a full agent ID (e.g., builder-1).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}
		prefix := sandbox.ProjectPrefix(pwd)

		// Try different naming patterns
		var sessionNames []string
		if strings.Contains(target, "-") {
			// Already looks like a full agent ID (role-id)
			sessionNames = append(sessionNames, prefix+target)
		} else {
			// Likely just a task ID, try common roles
			sessionNames = append(sessionNames, prefix+target) // Fallback for coordinator or legacy
			sessionNames = append(sessionNames, prefix+"builder-"+target)
			sessionNames = append(sessionNames, prefix+"scout-"+target)
			sessionNames = append(sessionNames, prefix+"reviewer-"+target)
		}

		var sessionName string
		for _, name := range sessionNames {
			checkCmd := exec.Command("tmux", "has-session", "-t", name)
			if checkCmd.Run() == nil {
				sessionName = name
				break
			}
		}

		if sessionName == "" {
			fmt.Printf("Error: could not find an active session for '%s'\n", target)
			fmt.Println("Available sessions:")
			sessions, _ := sandbox.ListSessions(prefix)
			for _, s := range sessions {
				fmt.Printf(" - %s\n", s)
			}
			os.Exit(1)
		}

		// If we are currently inside of a tmux session, we switch-client to avoid nesting
		tmuxCmd := "attach-session"
		if os.Getenv("TMUX") != "" {
			tmuxCmd = "switch-client"
		}

		c := exec.Command("tmux", tmuxCmd, "-t", sessionName)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Printf("Failed to connect to session '%s': %v\n", sessionName, err)
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().StringVarP(&spawnModel, "model", "m", "", "Model for the agent to use (set to 'auto' to use last used model)")
	runCmd.Flags().StringVarP(&spawnRole, "role", "r", "Builder", "Role of the agent (e.g., Builder, Reviewer)")
	runCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "Initial prompt or context for the agent")
	runCmd.Flags().StringVarP(&spawnTool, "tool", "t", "", "Runtime tool to use (gemini, opencode) - defaults to config or role setting")

	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(connectCmd)
}
