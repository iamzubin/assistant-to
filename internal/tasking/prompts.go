package tasking

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PromptBook holds the parsed system prompts from agents.md.
type PromptBook struct {
	prompts map[string]string
}

// LoadPrompts parses a Markdown file or a directory of Markdown files.
// If it's a file, it extracts sections by "## RoleName" headings.
// If it's a directory, it loads each .md file as a role prompt (using the filename).
func LoadPrompts(path string) (*PromptBook, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat prompts path: %w", err)
	}

	prompts := make(map[string]string)

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read prompts directory: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				role := strings.TrimSuffix(entry.Name(), ".md")
				// Capitalize the first letter to match current role naming convention
				if len(role) > 0 {
					role = strings.ToUpper(role[:1]) + role[1:]
				}

				data, err := os.ReadFile(filepath.Join(path, entry.Name()))
				if err != nil {
					return nil, fmt.Errorf("failed to read prompt file %s: %w", entry.Name(), err)
				}
				prompts[role] = strings.TrimSpace(string(data))
			}
		}

		// Load MCP files from mcp subdirectory
		mcpPath := filepath.Join(path, "mcp")
		if _, err := os.Stat(mcpPath); err == nil {
			mcpEntries, err := os.ReadDir(mcpPath)
			if err == nil {
				for _, entry := range mcpEntries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
						role := strings.TrimSuffix(entry.Name(), ".md")
						data, err := os.ReadFile(filepath.Join(mcpPath, entry.Name()))
						if err == nil {
							prompts["mcp/"+role] = strings.TrimSpace(string(data))
						}
					}
				}
			}
		}

		return &PromptBook{prompts: prompts}, nil
	}

	// Legacy single-file parsing
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var currentRole string
	var buf strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if currentRole != "" {
				prompts[currentRole] = strings.TrimSpace(buf.String())
			}
			currentRole = strings.TrimPrefix(line, "## ")
			buf.Reset()
			continue
		}
		if currentRole != "" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if currentRole != "" {
		prompts[currentRole] = strings.TrimSpace(buf.String())
	}

	return &PromptBook{prompts: prompts}, nil
}

// Get retrieves the system prompt for a given role.
// Returns an empty string if the role is not found.
func (pb *PromptBook) Get(role string) string {
	return pb.prompts[role]
}

// Roles returns all role names found in the prompts file.
func (pb *PromptBook) Roles() []string {
	roles := make([]string, 0, len(pb.prompts))
	for r := range pb.prompts {
		roles = append(roles, r)
	}
	return roles
}

// GetMCP retrieves the MCP configuration content for a given role.
// It looks in the "mcp" subdirectory of the prompts path.
func (pb *PromptBook) GetMCP(role string) string {
	return pb.prompts["mcp/"+strings.ToLower(role)]
}

// LoadMCPs loads MCP configuration files from the mcp subdirectory.
func (pb *PromptBook) LoadMCPs(mcpPath string) error {
	info, err := os.Stat(mcpPath)
	if err != nil {
		// MCP directory might not exist, that's ok
		return nil
	}

	if !info.IsDir() {
		return fmt.Errorf("mcp path is not a directory: %s", mcpPath)
	}

	entries, err := os.ReadDir(mcpPath)
	if err != nil {
		return fmt.Errorf("failed to read mcp directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			role := strings.TrimSuffix(entry.Name(), ".md")
			data, err := os.ReadFile(filepath.Join(mcpPath, entry.Name()))
			if err != nil {
				return fmt.Errorf("failed to read mcp file %s: %w", entry.Name(), err)
			}
			pb.prompts["mcp/"+role] = strings.TrimSpace(string(data))
		}
	}

	return nil
}
