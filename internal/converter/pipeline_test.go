package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kentalot/jira-beads-sync/internal/beads"
)

func TestNewPipeline(t *testing.T) {
	pipeline := NewPipeline("/tmp/test")
	if pipeline == nil {
		t.Fatal("NewPipeline returned nil")
	}
	if pipeline.jiraAdapter == nil {
		t.Error("jiraAdapter is nil")
	}
	if pipeline.converter == nil {
		t.Error("converter is nil")
	}
	if pipeline.jsonlRenderer == nil {
		t.Error("jsonlRenderer is nil")
	}
}

func TestPipelineConvertFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pipeline := NewPipeline(tmpDir)

	err = pipeline.ConvertFile("../../testdata/sample-jira-export.json")
	if err != nil {
		t.Fatalf("ConvertFile failed: %v", err)
	}

	// Verify output directory exists
	beadsDir := filepath.Join(tmpDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		t.Error("Beads directory was not created")
	}

	// Verify JSONL files exist
	issuesFile := filepath.Join(beadsDir, "issues.jsonl")
	if _, err := os.Stat(issuesFile); os.IsNotExist(err) {
		t.Error("issues.jsonl file was not created")
	}

	epicsFile := filepath.Join(beadsDir, "epics.jsonl")
	if _, err := os.Stat(epicsFile); os.IsNotExist(err) {
		t.Error("epics.jsonl file was not created")
	}

	// Read and verify issues.jsonl content
	loaded, err := beads.LoadIssuesJSONL(issuesFile)
	if err != nil {
		t.Fatalf("Failed to read issues.jsonl: %v", err)
	}
	var proj2 *beads.BeadsIssue
	for i := range loaded {
		if loaded[i].ID == "proj-2" {
			proj2 = &loaded[i]
			break
		}
	}
	if proj2 == nil {
		t.Fatal("expected issue proj-2 in issues.jsonl")
	}
	if proj2.Title != "Create login API endpoint" {
		t.Errorf("proj-2 title: got %q", proj2.Title)
	}
	if proj2.Status != "open" {
		t.Errorf("proj-2 status: got %q", proj2.Status)
	}
	if proj2.Priority != 1 {
		t.Errorf("proj-2 priority: got %d", proj2.Priority)
	}
	if proj2.Epic != "proj-1" {
		t.Errorf("proj-2 epic: got %q", proj2.Epic)
	}
	if len(proj2.DependsOn) != 1 || proj2.DependsOn[0] != "proj-4" {
		t.Errorf("proj-2 dependsOn: got %#v", proj2.DependsOn)
	}
	if proj2.Metadata == nil || proj2.Metadata["jiraKey"] != "PROJ-2" {
		t.Errorf("proj-2 jiraKey metadata: got %#v", proj2.Metadata)
	}
}

func TestPipelineConvertFileInvalid(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pipeline := NewPipeline(tmpDir)

	// Test with non-existent file
	err = pipeline.ConvertFile("nonexistent.json")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestPipelineEndToEnd(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pipeline-e2e-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pipeline := NewPipeline(tmpDir)

	// Convert the sample file
	err = pipeline.ConvertFile("../../testdata/sample-jira-export.json")
	if err != nil {
		t.Fatalf("ConvertFile failed: %v", err)
	}

	// Verify the conversion produced correct structure:
	// - 1 epic (PROJ-1)
	// - 3 issues (PROJ-2, PROJ-3, PROJ-4)
	// - PROJ-2 depends on PROJ-4
	// - PROJ-2 and PROJ-3 are linked to epic PROJ-1

	beadsDir := filepath.Join(tmpDir, ".beads")
	issuesFile := filepath.Join(beadsDir, "issues.jsonl")
	epicsFile := filepath.Join(beadsDir, "epics.jsonl")

	// Read and check epics JSONL
	epicsContent, err := os.ReadFile(epicsFile)
	if err != nil {
		t.Fatalf("Failed to read epics.jsonl: %v", err)
	}
	epicsStr := string(epicsContent)
	epicLines := 0
	for _, line := range splitLines(epicsStr) {
		if len(line) > 0 {
			epicLines++
		}
	}
	if epicLines != 1 {
		t.Errorf("Expected 1 epic, got %d", epicLines)
	}

	// Read and check issues JSONL
	issuesContent, err := os.ReadFile(issuesFile)
	if err != nil {
		t.Fatalf("Failed to read issues.jsonl: %v", err)
	}
	issuesStr := string(issuesContent)
	issueLines := 0
	for _, line := range splitLines(issuesStr) {
		if len(line) > 0 {
			issueLines++
		}
	}
	if issueLines != 3 {
		t.Errorf("Expected 3 issues, got %d", issueLines)
	}

	loaded, err := beads.LoadIssuesJSONL(issuesFile)
	if err != nil {
		t.Fatalf("load issues: %v", err)
	}
	var proj2 *beads.BeadsIssue
	for i := range loaded {
		if loaded[i].ID == "proj-2" {
			proj2 = &loaded[i]
			break
		}
	}
	if proj2 == nil {
		t.Fatal("PROJ-2 (proj-2) should exist in issues")
	}
	if proj2.Epic != "proj-1" {
		t.Error("PROJ-2 should be linked to epic proj-1")
	}
	if len(proj2.DependsOn) != 1 || proj2.DependsOn[0] != "proj-4" {
		t.Error("PROJ-2 should depend on proj-4")
	}
}

// Helper function to split string into lines
func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}
