package beads

import (
	"encoding/json"
	"testing"
)

func TestParseCommentsJSON(t *testing.T) {
	data := `[{"id":"c1","issue_id":"bd-1","author":"A","text":"hi #jira","created_at":"2026-01-01T00:00:00Z"}]`
	got, err := parseCommentsJSON([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "c1" || got[0].Text != "hi #jira" {
		t.Fatalf("got %+v", got)
	}
}

func TestListCommentsFromRaw(t *testing.T) {
	raw := json.RawMessage(`[{"id":"x","text":"y"}]`)
	got, err := ListComments("", "bd-1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "x" {
		t.Fatalf("got %+v", got)
	}
}
