package sync

import (
	"testing"

	"github.com/conallob/jira-beads-sync/internal/converter"
	"github.com/conallob/jira-beads-sync/internal/jira"
)

func mkTransition(id, toName, catKey string) jira.Transition {
	var tr jira.Transition
	tr.ID = id
	tr.To.Name = toName
	tr.To.StatusCategory.Key = catKey
	return tr
}

func TestPickTransition(t *testing.T) {
	pc := converter.NewProtoConverter()
	transitions := []jira.Transition{
		mkTransition("10", "To Do", "new"),
		mkTransition("21", "In Progress", "indeterminate"),
	}

	id, err := pickTransition(pc, transitions, "in_progress")
	if err != nil {
		t.Fatal(err)
	}
	if id != "21" {
		t.Fatalf("got transition id %q, want 21", id)
	}

	_, err = pickTransition(pc, transitions, "closed")
	if err == nil {
		t.Fatal("expected error for impossible status")
	}
}

func TestMatchPriorityOption(t *testing.T) {
	opts := []jira.PriorityOption{
		{ID: "1", Name: "Highest"},
		{ID: "2", Name: "High"},
		{ID: "3", Name: "Medium"},
		{ID: "4", Name: "Low"},
		{ID: "5", Name: "Lowest"},
	}
	id, ok := matchPriorityOption(opts, 4)
	if !ok || id != "5" {
		t.Fatalf("got %q ok=%v", id, ok)
	}
}
