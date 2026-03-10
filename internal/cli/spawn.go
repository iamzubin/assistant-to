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
	spawnAgent  string
	spawnSkill  string
)

var spawnCmd = &cobra.Command{
	Use:     "spawn <target>",
	Aliases: []string{"run"},
	Short:   "Run an agent for a specific task",
	Long:    `Creates an isolated worktree and spawns a new tmux session for an agent targeting the specified task.`,
	Args:    cobra.ExactArgs(1),
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

		// Discover custom OpenCode agents if enabled
		var customAgents []config.OpenCodeAgent
		if conf.IsCustomAgentsEnabled() {
			customAgents = conf.DiscoverOpenCodeAgents(pwd)
			if len(customAgents) > 0 {
				fmt.Printf("Discovered %d custom OpenCode agent(s)\n", len(customAgents))
				for _, agent := range customAgents {
					fmt.Printf("  - %s (%s, %s)\n", agent.Name, agent.Mode, agent.Scope)
				}
			}
		}

		// Discover Gemini skills if enabled
		var customSkills []config.GeminiSkill
		if conf.IsGeminiSkillsEnabled() {
			customSkills = conf.DiscoverGeminiSkills(pwd)
			if len(customSkills) > 0 {
				fmt.Printf("Discovered %d Gemini skill(s)\n", len(customSkills))
				for _, skill := range customSkills {
					fmt.Printf("  - %s (%s)\n", skill.Name, skill.Scope)
				}
			}
		}

		// Load prompt from agents.md if not provided
		finalPrompt := spawnPrompt
		if finalPrompt == "" {
			// Check if a custom agent was specified
			if spawnAgent != "" {
				for _, agent := range customAgents {
					if agent.Name == spawnAgent {
						finalPrompt = agent.Instructions
						fmt.Printf("Using custom agent: %s\n", agent.Name)
						// Override model if specified in agent
						if agent.Model != "" && (spawnModel == "" || spawnModel == "auto") {
							spawnModel = agent.Model
							fmt.Printf("  Using agent model: %s\n", agent.Model)
						}
						break
					}
				}
			}

			// If no custom agent or still no prompt, fall back to standard prompts
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
		}

		// Check if a custom skill was specified
		if spawnSkill != "" {
			for _, skill := range customSkills {
				if skill.Name == spawnSkill {
					fmt.Printf("Using Gemini skill: %s\n", skill.Name)
					// Skills are linked via gemini CLI, so we need to ensure they're linked
					// For now, we just note which skill is being used
					break
				}
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

var listAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List available custom OpenCode agents",
	Long:  `Discovers and lists custom OpenCode agents from per-project and global locations.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		configPath := filepath.Join(pwd, ".dwight", "config.yaml")
		conf, err := config.Load(configPath)
		if err != nil {
			fmt.Printf("Warning: failed to load workspace config, using defaults.\n")
			conf = config.Default()
		}

		if !conf.IsCustomAgentsEnabled() {
			fmt.Println("Custom OpenCode agents are disabled in config.")
			return
		}

		agents := conf.DiscoverOpenCodeAgents(pwd)
		if len(agents) == 0 {
			fmt.Println("No custom OpenCode agents found.")
			fmt.Println("Place .md files in:")
			fmt.Printf("  - Per-project: %s\n", filepath.Join(pwd, ".opencode", "agents"))
			fmt.Printf("  - Global: %s\n", filepath.Join(config.GetHomeDir(), ".config", "opencode", "agents"))
			return
		}

		fmt.Println("Available OpenCode Agents:")
		for _, agent := range agents {
			fmt.Printf("  %s (%s)\n", agent.Name, agent.Scope)
			if agent.Description != "" {
				fmt.Printf("    Description: %s\n", agent.Description)
			}
			if agent.Mode != "" {
				fmt.Printf("    Mode: %s\n", agent.Mode)
			}
			if agent.Model != "" {
				fmt.Printf("    Model: %s\n", agent.Model)
			}
		}
	},
}

var listSkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "List available Gemini skills",
	Long:  `Discovers and lists Gemini skills from per-project and global locations.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		configPath := filepath.Join(pwd, ".dwight", "config.yaml")
		conf, err := config.Load(configPath)
		if err != nil {
			fmt.Printf("Warning: failed to load workspace config, using defaults.\n")
			conf = config.Default()
		}

		if !conf.IsGeminiSkillsEnabled() {
			fmt.Println("Gemini skills are disabled in config.")
			return
		}

		skills := conf.DiscoverGeminiSkills(pwd)
		if len(skills) == 0 {
			fmt.Println("No Gemini skills found.")
			fmt.Println("Place skill directories in:")
			fmt.Printf("  - Per-project: %s\n", filepath.Join(pwd, ".gemini", "skills"))
			fmt.Printf("  - Global: %s\n", filepath.Join(config.GetHomeDir(), ".gemini", "skills"))
			return
		}

		fmt.Println("Available Gemini Skills:")
		for _, skill := range skills {
			fmt.Printf("  %s (%s)\n", skill.Name, skill.Scope)
			if skill.Description != "" {
				fmt.Printf("    Description: %s\n", skill.Description)
			}
		}
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agents and skills",
	Long:  `Discovers and lists custom OpenCode agents and Gemini skills.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pwd, err := findProjectRoot()
		if err != nil {
			fmt.Printf("Failed to find project root: %v\n", err)
			os.Exit(1)
		}

		configPath := filepath.Join(pwd, ".dwight", "config.yaml")
		conf, err := config.Load(configPath)
		if err != nil {
			fmt.Printf("Warning: failed to load workspace config, using defaults.\n")
			conf = config.Default()
		}

		fmt.Println("=== OpenCode Agents ===")
		if conf.IsCustomAgentsEnabled() {
			agents := conf.DiscoverOpenCodeAgents(pwd)
			if len(agents) == 0 {
				fmt.Println("  No custom agents found.")
			}
			for _, agent := range agents {
				fmt.Printf("  %s (%s)\n", agent.Name, agent.Scope)
			}
		} else {
			fmt.Println("  Disabled")
		}

		fmt.Println("\n=== Gemini Skills ===")
		if conf.IsGeminiSkillsEnabled() {
			skills := conf.DiscoverGeminiSkills(pwd)
			if len(skills) == 0 {
				fmt.Println("  No skills found.")
			}
			for _, skill := range skills {
				fmt.Printf("  %s (%s)\n", skill.Name, skill.Scope)
			}
		} else {
			fmt.Println("  Disabled")
		}
	},
}

func init() {
	spawnCmd.Flags().StringVarP(&spawnModel, "model", "m", "", "Model for the agent to use (set to 'auto' to use last used model)")
	spawnCmd.Flags().StringVarP(&spawnRole, "role", "r", "Builder", "Role of the agent (e.g., Builder, Reviewer)")
	spawnCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "Initial prompt or context for the agent")
	spawnCmd.Flags().StringVarP(&spawnTool, "tool", "t", "", "Runtime tool to use (gemini, opencode) - defaults to config or role setting")
	spawnCmd.Flags().StringVarP(&spawnAgent, "agent", "a", "", "Custom OpenCode agent to use (discovered from .opencode/agents or ~/.config/opencode/agents)")
	spawnCmd.Flags().StringVarP(&spawnSkill, "skill", "s", "", "Gemini skill to use (discovered from .gemini/skills or ~/.gemini/skills)")

	RootCmd.AddCommand(spawnCmd)
	RootCmd.AddCommand(connectCmd)
	RootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listAgentsCmd)
	listCmd.AddCommand(listSkillsCmd)
}
