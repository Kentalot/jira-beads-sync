package sync

import (
	"testing"

	"github.com/Kentalot/jira-beads-sync/internal/jira"
)

func TestRunWithLinesRequiresIssueKeys(t *testing.T) {
	client := jira.NewClient("https://jira.example.com", "u", "t", "basic")
	err := RunWithLines(client, ".beads/issues.jsonl", nil, nil, RunOptions{})
	if err == nil {
		t.Fatal("expected error when no issue keys provided")
	}
}

func TestRunWithLinesFiltersToRequestedKeys(t *testing.T) {
	filter := normalizeKeySet([]string{"PROJ-2"})
	if len(filter) != 1 {
		t.Fatalf("filter: %#v", filter)
	}
	if _, ok := filter["PROJ-2"]; !ok {
		t.Fatal("expected PROJ-2 in filter")
	}
}
