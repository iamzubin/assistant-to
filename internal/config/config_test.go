package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetHomeDir(t *testing.T) {
	home := GetHomeDir()
	if home == "" {
		t.Error("Expected non-empty home directory")
	}
}

func TestOpenCodeAgentParsing(t *testing.T) {
	tmpDir := t.TempDir()

	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	os.MkdirAll(agentsDir, 0755)

	agentContent := `description: A test agent
mode: subagent
model: gemini-2.5-flash
temperature: 0.7
tools: mail,log,buffer

# Agent instructions
You are a test agent that does things.
`

	agentPath := filepath.Join(agentsDir, "test-agent.md")
	os.WriteFile(agentPath, []byte(agentContent), 0644)

	agent, err := parseAgentFile(agentPath)
	if err != nil {
		t.Fatalf("Failed to parse agent file: %v", err)
	}

	if agent.Name != "test-agent" {
		t.Errorf("Expected name 'test-agent', got '%s'", agent.Name)
	}
	if agent.Description != "A test agent" {
		t.Errorf("Expected description 'A test agent', got '%s'", agent.Description)
	}
	if agent.Mode != "subagent" {
		t.Errorf("Expected mode 'subagent', got '%s'", agent.Mode)
	}
	if agent.Model != "gemini-2.5-flash" {
		t.Errorf("Expected model 'gemini-2.5-flash', got '%s'", agent.Model)
	}
	if agent.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7, got %f", agent.Temperature)
	}
	if len(agent.AllowedTools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(agent.AllowedTools))
	}
}

func TestGeminiSkillParsing(t *testing.T) {
	tmpDir := t.TempDir()

	skillsDir := filepath.Join(tmpDir, ".gemini", "skills")
	os.MkdirAll(skillsDir, 0755)

	skillDir := filepath.Join(skillsDir, "test-skill")
	os.MkdirAll(skillDir, 0755)

	descriptionContent := `This is a test skill
for doing things.
`

	os.WriteFile(filepath.Join(skillDir, "description.md"), []byte(descriptionContent), 0644)

	skill, err := parseSkillDir(skillDir)
	if err != nil {
		t.Fatalf("Failed to parse skill dir: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("Expected name 'test-skill', got '%s'", skill.Name)
	}
	if skill.Description != "This is a test skill" {
		t.Errorf("Expected description 'This is a test skill', got '%s'", skill.Description)
	}
}

func TestDiscoverOpenCodeAgents(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		CustomAgents: CustomAgentsConfig{
			Enabled:    true,
			PerProject: true,
			Global:     false,
		},
	}

	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	os.MkdirAll(agentsDir, 0755)

	os.WriteFile(filepath.Join(agentsDir, "agent1.md"), []byte("description: Agent 1\nmode: primary\n"), 0644)
	os.WriteFile(filepath.Join(agentsDir, "agent2.md"), []byte("description: Agent 2\nmode: subagent\n"), 0644)

	agents := cfg.DiscoverOpenCodeAgents(tmpDir)
	if len(agents) != 2 {
		t.Errorf("Expected 2 agents, got %d", len(agents))
	}
}

func TestDiscoverGeminiSkills(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		GeminiSkills: GeminiSkillsConfig{
			Enabled:    true,
			PerProject: true,
			Global:     false,
		},
	}

	skillsDir := filepath.Join(tmpDir, ".gemini", "skills")
	os.MkdirAll(skillsDir, 0755)

	skill1Dir := filepath.Join(skillsDir, "skill1")
	os.MkdirAll(skill1Dir, 0755)
	os.WriteFile(filepath.Join(skill1Dir, "description.md"), []byte("Skill 1 description\n"), 0644)

	skill2Dir := filepath.Join(skillsDir, "skill2")
	os.MkdirAll(skill2Dir, 0755)
	os.WriteFile(filepath.Join(skill2Dir, "description.md"), []byte("Skill 2 description\n"), 0644)

	skills := cfg.DiscoverGeminiSkills(tmpDir)
	if len(skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(skills))
	}
}

func TestCustomAgentsConfigDefaults(t *testing.T) {
	cfg := Default()

	if !cfg.CustomAgents.Enabled {
		t.Error("Expected custom agents to be enabled by default")
	}
	if !cfg.CustomAgents.PerProject {
		t.Error("Expected per-project agents to be enabled by default")
	}
	if !cfg.CustomAgents.Global {
		t.Error("Expected global agents to be enabled by default")
	}
}

func TestGeminiSkillsConfigDefaults(t *testing.T) {
	cfg := Default()

	if !cfg.GeminiSkills.Enabled {
		t.Error("Expected Gemini skills to be enabled by default")
	}
	if !cfg.GeminiSkills.PerProject {
		t.Error("Expected per-project skills to be enabled by default")
	}
	if !cfg.GeminiSkills.Global {
		t.Error("Expected global skills to be enabled by default")
	}
}

func TestIsCustomAgentsEnabled(t *testing.T) {
	cfg := &Config{}
	if cfg.IsCustomAgentsEnabled() {
		t.Error("Expected disabled when not set")
	}

	cfg.CustomAgents.Enabled = true
	if !cfg.IsCustomAgentsEnabled() {
		t.Error("Expected enabled")
	}
}

func TestIsGeminiSkillsEnabled(t *testing.T) {
	cfg := &Config{}
	if cfg.IsGeminiSkillsEnabled() {
		t.Error("Expected disabled when not set")
	}

	cfg.GeminiSkills.Enabled = true
	if !cfg.IsGeminiSkillsEnabled() {
		t.Error("Expected enabled")
	}
}
