package beads

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// IssuesJSONLPath returns the canonical path to issues.jsonl under outputDir.
func IssuesJSONLPath(outputDir string) string {
	return filepath.Join(outputDir, ".beads", "issues.jsonl")
}

// LoadIssuesJSONL reads all issues from a beads issues.jsonl file.
func LoadIssuesJSONL(path string) ([]BeadsIssue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open issues jsonl: %w", err)
	}
	defer f.Close()

	var issues []BeadsIssue
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		var issue BeadsIssue
		if err := json.Unmarshal(scanner.Bytes(), &issue); err != nil {
			return nil, fmt.Errorf("parse issues.jsonl line %d: %w", line, err)
		}
		issues = append(issues, issue)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read issues jsonl: %w", err)
	}
	return issues, nil
}
