package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"assistant-to/internal/api"
	"assistant-to/internal/config"
	"assistant-to/internal/db"

	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server commands",
	Long:  `Commands for running the MCP (Model Context Protocol) server.`,
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run MCP server via stdio",
	Long: `Runs the MCP server in stdio mode for connecting to AI tools like opencode, Claude Desktop, etc.

This command starts an MCP server that communicates via stdin/stdout using JSON-RPC 2.0.
It allows AI tools to discover and call tools provided by the assistant-to coordinator.

Environment Variables:
  AT_MCP_PORT    MCP HTTP port (default: 8766)

Example usage with opencode:
  Add to ~/.config/opencode/mcp.json:
  {
    "mcpServers": {
      "assistant-to": {
        "command": "dwight",
        "args": ["mcp", "serve"]
      }
    }
  }`,
	Run: func(cmd *cobra.Command, args []string) {
		// First check for env vars - these take priority
		pwd := os.Getenv("AT_PROJECT_ROOT")
		if pwd == "" {
			// Fallback to auto-detection
			var err error
			pwd, err = findProjectRoot()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to find project root: %v\n", err)
				os.Exit(1)
			}
		}

		configPath := filepath.Join(pwd, ".assistant-to", "config.yaml")
		cfg, err := config.Load(configPath)
		if err != nil {
			cfg = config.Default()
		}

		dbPath := filepath.Join(pwd, ".assistant-to", "state.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		mcpPort := cfg.API.MCPPort
		if envPort := os.Getenv("AT_MCP_PORT"); envPort != "" {
			fmt.Sscanf(envPort, "%d", &mcpPort)
		}

		server := api.NewMCPServer(mcpPort, pwd, cfg, database)

		// Run stdio server
		fmt.Fprintf(os.Stderr, "MCP server starting on port %d...\n", mcpPort)
		runStdioServer(server)
	},
}

func runStdioServer(server *api.MCPServer) {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req api.MCPRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := api.MCPResponse{
				JSONRPC: "2.0",
				Error:   &api.MCPError{Code: -32700, Message: "Parse error"},
			}
			respJSON, _ := json.Marshal(resp)
			fmt.Println(string(respJSON))
			continue
		}

		resp := server.ProcessRequest(req)
		respJSON, _ := json.Marshal(resp)
		fmt.Println(string(respJSON))
	}
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	RootCmd.AddCommand(mcpCmd)
}
