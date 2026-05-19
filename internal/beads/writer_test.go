package beads

import (
	"path/filepath"
	"testing"
)

func TestSaveIssuesJSONL(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "issues.jsonl")
	issues := []BeadsIssue{
		{ID: "a", Title: "t", Status: "open", Metadata: map[string]string{"k": "v"}},
	}
	if err := SaveIssuesJSONL(path, issues); err != nil {
		t.Fatal(err)
	}
	back, err := LoadIssuesJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0].ID != "a" || back[0].Metadata["k"] != "v" {
		t.Fatalf("%+v", back)
	}
}
