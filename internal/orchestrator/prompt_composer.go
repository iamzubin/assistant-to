package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// PromptTemplate represents a composable prompt fragment
type PromptTemplate struct {
	Name      string
	Content   string
	Type      TemplateType
	Variables []string
	Inherits  []string
}

// TemplateType categorizes prompt fragments
type TemplateType int

const (
	TemplateBase TemplateType = iota
	TemplateOverlay
	TemplateSnippet
)

func (t TemplateType) String() string {
	switch t {
	case TemplateBase:
		return "base"
	case TemplateOverlay:
		return "overlay"
	case TemplateSnippet:
		return "snippet"
	default:
		return "unknown"
	}
}

// CompositionContext holds data for template rendering
type CompositionContext struct {
	Role        string
	TaskID      string
	TaskTitle   string
	TaskDesc    string
	TargetFiles []string
	Timestamp   time.Time
	Expertise   []string
	BasePath    string
}

// PromptComposer handles prompt composition with inheritance
type PromptComposer struct {
	templates map[string]*PromptTemplate
	basePath  string
}

// NewPromptComposer creates a new prompt composer
func NewPromptComposer(basePath string) (*PromptComposer, error) {
	composer := &PromptComposer{
		templates: make(map[string]*PromptTemplate),
		basePath:  basePath,
	}

	// Load all templates from the prompts directory
	if err := composer.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return composer, nil
}

// loadTemplates loads all .md files from the prompts directory
func (pc *PromptComposer) loadTemplates() error {
	entries, err := os.ReadDir(pc.basePath)
	if err != nil {
		return fmt.Errorf("failed to read prompts directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		content, err := os.ReadFile(filepath.Join(pc.basePath, entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", name, err)
		}

		// Determine template type based on naming convention
		templateType := TemplateBase
		if strings.HasPrefix(name, "_") {
			templateType = TemplateSnippet
		} else if strings.Contains(name, "-") {
			templateType = TemplateOverlay
		}

		// Parse variables from content {{.VariableName}}
		variables := extractVariables(string(content))

		// Parse inheritance directives
		inherits := extractInheritance(string(content))

		pc.templates[name] = &PromptTemplate{
			Name:      name,
			Content:   string(content),
			Type:      templateType,
			Variables: variables,
			Inherits:  inherits,
		}
	}

	return nil
}

// Compose creates a final prompt by composing templates
func (pc *PromptComposer) Compose(role string, ctx *CompositionContext) (string, error) {
	// Get base template for the role
	baseTemplate, ok := pc.templates[role]
	if !ok {
		return "", fmt.Errorf("no base template found for role: %s", role)
	}

	// Build inheritance chain
	chain := pc.buildInheritanceChain(baseTemplate)

	// Compose final content
	var parts []string
	for _, tmpl := range chain {
		// Render template with context
		rendered, err := pc.renderTemplate(tmpl, ctx)
		if err != nil {
			return "", fmt.Errorf("failed to render template %s: %w", tmpl.Name, err)
		}
		parts = append(parts, rendered)
	}

	// Join with separators
	final := strings.Join(parts, "\n\n---\n\n")

	return final, nil
}

// buildInheritanceChain creates the template inheritance chain
func (pc *PromptComposer) buildInheritanceChain(base *PromptTemplate) []*PromptTemplate {
	var chain []*PromptTemplate
	visited := make(map[string]bool)

	var buildChain func(t *PromptTemplate)
	buildChain = func(t *PromptTemplate) {
		if visited[t.Name] {
			return
		}
		visited[t.Name] = true

		// First, add inherited templates
		for _, inheritName := range t.Inherits {
			if parent, ok := pc.templates[inheritName]; ok {
				buildChain(parent)
			}
		}

		// Then add this template
		chain = append(chain, t)
	}

	buildChain(base)
	return chain
}

// renderTemplate renders a template with the given context
func (pc *PromptComposer) renderTemplate(tmpl *PromptTemplate, ctx *CompositionContext) (string, error) {
	// Create template with functions
	templateFuncs := template.FuncMap{
		"join": strings.Join,
		"now":  func() string { return time.Now().Format(time.RFC3339) },
	}

	t, err := template.New(tmpl.Name).Funcs(templateFuncs).Parse(tmpl.Content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// AddTemplate adds or updates a template dynamically
func (pc *PromptComposer) AddTemplate(name string, content string, templateType TemplateType) error {
	variables := extractVariables(content)
	inherits := extractInheritance(content)

	pc.templates[name] = &PromptTemplate{
		Name:      name,
		Content:   content,
		Type:      templateType,
		Variables: variables,
		Inherits:  inherits,
	}

	return nil
}

// GetTemplate retrieves a specific template
func (pc *PromptComposer) GetTemplate(name string) *PromptTemplate {
	return pc.templates[name]
}

// ListTemplates returns all available template names
func (pc *PromptComposer) ListTemplates() []string {
	names := make([]string, 0, len(pc.templates))
	for name := range pc.templates {
		names = append(names, name)
	}
	return names
}

// extractVariables finds {{.VariableName}} patterns in content
func extractVariables(content string) []string {
	var variables []string
	// Simple regex-like extraction
	for {
		start := strings.Index(content, "{{.")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], "}}")
		if end == -1 {
			break
		}
		variable := content[start+3 : start+end]
		variables = append(variables, variable)
		content = content[start+end+2:]
	}
	return variables
}

// extractInheritance finds @inherit directive in content
func extractInheritance(content string) []string {
	var inherits []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@inherit ") {
			inheritList := strings.TrimPrefix(line, "@inherit ")
			parts := strings.Split(inheritList, ",")
			for _, part := range parts {
				inherits = append(inherits, strings.TrimSpace(part))
			}
		}
	}
	return inherits
}

// RenderATInstructions creates the final AT_INSTRUCTIONS.md for injection into worktrees
func (pc *PromptComposer) RenderATInstructions(role string, ctx *CompositionContext, outputPath string) error {
	content, err := pc.Compose(role, ctx)
	if err != nil {
		return err
	}

	header := fmt.Sprintf(`# AT_INSTRUCTIONS.md
# Generated: %s
# Role: %s
# Task: %s

`, time.Now().Format(time.RFC3339), role, ctx.TaskID)

	fullContent := header + content

	if err := os.WriteFile(outputPath, []byte(fullContent), 0644); err != nil {
		return fmt.Errorf("failed to write AT_INSTRUCTIONS.md: %w", err)
	}

	return nil
}
