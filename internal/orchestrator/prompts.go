package orchestrator

import (
	"fmt"
	"os"
	"strings"
)

// PromptBook holds the parsed system prompts from agents.md.
type PromptBook struct {
	prompts map[string]string
}

// LoadPrompts parses a Markdown file and extracts sections by "## RoleName" headings.
// Each section becomes a named prompt in the PromptBook.
func LoadPrompts(path string) (*PromptBook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts file: %w", err)
	}

	prompts := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	var currentRole string
	var buf strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			// Save the previous section
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
	// Flush the last section
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
