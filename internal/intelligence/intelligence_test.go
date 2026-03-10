package intelligence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodeIndex(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "test-index.db")

	index, err := NewCodeIndex(indexPath)
	if err != nil {
		t.Fatalf("Failed to create code index: %v", err)
	}
	defer index.Close()

	if err := index.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
}

func TestIndexProject(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "test-index.db")

	index, err := NewCodeIndex(indexPath)
	if err != nil {
		t.Fatalf("Failed to create code index: %v", err)
	}
	defer index.Close()

	projectPath := filepath.Join(tmpDir, "testproject")
	os.MkdirAll(filepath.Join(projectPath, "pkg1"), 0755)
	os.WriteFile(filepath.Join(projectPath, "pkg1", "file.go"), []byte(`package pkg1
func Hello() string { return "hello" }
type MyStruct struct {}
`), 0644)

	if err := index.IndexProject(projectPath); err != nil {
		t.Fatalf("Failed to index project: %v", err)
	}

	packages, err := index.GetAllPackages()
	if err != nil {
		t.Fatalf("Failed to get packages: %v", err)
	}

	if len(packages) == 0 {
		t.Error("Expected packages to be indexed")
	}
}

func TestGetFileInfo(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "test-index.db")

	projectPath := filepath.Join(tmpDir, "testproject")
	os.MkdirAll(filepath.Join(projectPath, "pkg1"), 0755)
	os.WriteFile(filepath.Join(projectPath, "pkg1", "file.go"), []byte(`package pkg1
import "fmt"
func Hello() string { return "hello" }
type MyStruct struct {}
`), 0644)

	index, err := NewCodeIndex(indexPath)
	if err != nil {
		t.Fatalf("Failed to create code index: %v", err)
	}
	defer index.Close()

	if err := index.IndexProject(projectPath); err != nil {
		t.Fatalf("Failed to index project: %v", err)
	}

	files, err := index.GetPackageFiles("pkg1")
	if err != nil {
		t.Fatalf("Failed to get package files: %v", err)
	}

	if len(files) == 0 {
		t.Error("Expected files in intelligence package")
	}

	fileInfo, err := index.GetFileInfo(files[0])
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}

	if fileInfo.Package == "" {
		t.Error("Expected package name in file info")
	}
}

func TestSearchDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "test-index.db")

	projectPath := filepath.Join(tmpDir, "testproject")
	os.MkdirAll(filepath.Join(projectPath, "pkg1"), 0755)
	os.WriteFile(filepath.Join(projectPath, "pkg1", "file.go"), []byte(`package pkg1
func Hello() string { return "hello" }
`), 0644)

	index, err := NewCodeIndex(indexPath)
	if err != nil {
		t.Fatalf("Failed to create code index: %v", err)
	}
	defer index.Close()

	if err := index.IndexProject(projectPath); err != nil {
		t.Fatalf("Failed to index project: %v", err)
	}

	deps, err := index.SearchDependencies("pkg1/file.go")
	if err != nil {
		t.Fatalf("Failed to search dependencies: %v", err)
	}

	_ = deps
}

func TestPackageStructures(t *testing.T) {
	pkg := Package{
		Path:     "test/path",
		Name:     "test",
		Files:    []string{"file1.go", "file2.go"},
		Imports:  []string{"fmt", "os"},
		Exported: []string{"Func1", "Func2"},
	}

	if pkg.Path != "test/path" {
		t.Errorf("Expected path 'test/path', got '%s'", pkg.Path)
	}

	file := File{
		Path:      "test/path/file.go",
		Package:   "test",
		Imports:   []string{"fmt"},
		Types:     []string{"MyType"},
		Functions: []string{"MyFunc"},
		Methods:   []Method{{Name: "Method1", Receiver: "MyType"}},
	}

	if len(file.Types) != 1 {
		t.Errorf("Expected 1 type, got %d", len(file.Types))
	}
}

func TestImpactAnalyzer(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "test-index.db")

	projectPath := filepath.Join(tmpDir, "testproject")
	os.MkdirAll(filepath.Join(projectPath, "pkg1"), 0755)
	os.WriteFile(filepath.Join(projectPath, "pkg1", "file.go"), []byte(`package pkg1
func Hello() string { return "hello" }
`), 0644)

	index, err := NewCodeIndex(indexPath)
	if err != nil {
		t.Fatalf("Failed to create code index: %v", err)
	}
	defer index.Close()

	if err := index.IndexProject(projectPath); err != nil {
		t.Fatalf("Failed to index project: %v", err)
	}

	analyzer := NewImpactAnalyzer(index)
	report, err := analyzer.AnalyzeChangeImpact("pkg1/file.go")
	if err != nil {
		t.Fatalf("Failed to analyze impact: %v", err)
	}

	if report == nil {
		t.Error("Expected impact report")
	}

	if report.RiskLevel < RiskLow || report.RiskLevel > RiskCritical {
		t.Errorf("Invalid risk level: %v", report.RiskLevel)
	}
}

func TestFormatReport(t *testing.T) {
	report := &ImpactReport{
		TargetFile:       "test.go",
		DirectDependents: []string{"a.go", "b.go"},
		TransitiveDeps:   []string{"c.go"},
		AffectedPackages: []string{"pkg1", "pkg2"},
		RiskLevel:        RiskMedium,
		Recommendations:  []string{"Test recommendation"},
	}

	formatted := FormatReport(report)
	if formatted == "" {
		t.Error("Expected formatted report")
	}
}

func TestRiskLevelString(t *testing.T) {
	tests := []struct {
		level    RiskLevel
		expected string
	}{
		{RiskLow, "low"},
		{RiskMedium, "medium"},
		{RiskHigh, "high"},
		{RiskCritical, "critical"},
		{RiskLevel(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.level.String() != tt.expected {
			t.Errorf("Expected '%s', got '%s'", tt.expected, tt.level.String())
		}
	}
}

func TestContains(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !contains(slice, "a") {
		t.Error("Expected 'a' to be in slice")
	}

	if contains(slice, "d") {
		t.Error("Expected 'd' to not be in slice")
	}
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "test-index.db")

	index, err := NewCodeIndex(indexPath)
	if err != nil {
		t.Fatalf("Failed to create code index: %v", err)
	}

	if err := index.Close(); err != nil {
		t.Fatalf("Failed to close index: %v", err)
	}

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("Expected index file to exist")
	}
}
