package beads

import (
	"path/filepath"
)

// IssuesJSONLPath returns the canonical path to issues.jsonl under outputDir.
func IssuesJSONLPath(outputDir string) string {
	return filepath.Join(outputDir, ".beads", "issues.jsonl")
}

// LoadIssuesJSONL reads all issues from a beads issues.jsonl file.
func LoadIssuesJSONL(path string) ([]BeadsIssue, error) {
	lines, err := LoadIssuesJSONLinesPreserve(path)
	if err != nil {
		return nil, err
	}
	out := make([]BeadsIssue, len(lines))
	for i := range lines {
		out[i] = lines[i].Issue
	}
	return out, nil
}
