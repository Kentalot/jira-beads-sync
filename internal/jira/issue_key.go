package jira

import (
	"fmt"
	"regexp"
)

var issueKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-[1-9][0-9]*$`)

// ValidateIssueKey ensures issueKey is safe to embed in REST paths (no slashes, traversal, etc.).
func ValidateIssueKey(issueKey string) error {
	if issueKey == "" {
		return fmt.Errorf("issue key is empty")
	}
	if !issueKeyPattern.MatchString(issueKey) {
		return fmt.Errorf("invalid Jira issue key: %q", issueKey)
	}
	return nil
}
