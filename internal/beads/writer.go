package beads

import (
	"encoding/json"
	"fmt"
)

// SaveIssuesJSONL writes issues to path (one JSON object per line).
// Lines are built from BeadsIssue only (no extra native keys); use SaveIssuesJSONLinesPreserve
// when round-tripping native beads JSONL.
func SaveIssuesJSONL(path string, issues []BeadsIssue) error {
	lines := make([]IssueJSONLLine, len(issues))
	for i := range issues {
		b, err := json.Marshal(issues[i])
		if err != nil {
			return fmt.Errorf("issue %s: %w", issues[i].ID, err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err != nil {
			return fmt.Errorf("issue %s: %w", issues[i].ID, err)
		}
		lines[i] = IssueJSONLLine{Raw: raw, Issue: issues[i]}
	}
	return SaveIssuesJSONLinesPreserve(path, lines)
}
