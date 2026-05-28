package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Comment is one beads issue comment (from export JSONL or `bd comments --json`).
type Comment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// ListComments returns comments for a beads issue. If rawCommentsJSON is non-empty
// (e.g. from issues.jsonl export), it is parsed first; otherwise `bd comments <id> --json`
// is run in workDir when bd is available.
func ListComments(workDir, issueID string, rawCommentsJSON []byte) ([]Comment, error) {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return nil, fmt.Errorf("issue id is required")
	}
	if len(bytesTrimSpace(rawCommentsJSON)) > 0 {
		return parseCommentsJSON(rawCommentsJSON)
	}
	return listCommentsViaBD(workDir, issueID)
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func parseCommentsJSON(data []byte) ([]Comment, error) {
	data = bytesTrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var comments []Comment
	if err := json.Unmarshal(data, &comments); err != nil {
		return nil, fmt.Errorf("parse comments json: %w", err)
	}
	return comments, nil
}

func listCommentsViaBD(workDir, issueID string) ([]Comment, error) {
	if _, err := exec.LookPath("bd"); err != nil {
		return nil, nil
	}
	args := []string{"comments", issueID, "--json"}
	var cmd *exec.Cmd
	if workDir != "" {
		cmd = exec.Command("bd", append([]string{"-C", workDir}, args...)...)
	} else {
		cmd = exec.Command("bd", args...)
	}
	out, err := cmd.Output()
	if err != nil {
		if isBDNoDatabase(err) {
			return nil, nil
		}
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("bd comments %s: %w (%s)", issueID, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("bd comments %s: %w", issueID, err)
	}
	return parseCommentsJSON(out)
}

func isBDNoDatabase(err error) bool {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	msg := strings.ToLower(string(ee.Stderr))
	return strings.Contains(msg, "no beads database")
}
