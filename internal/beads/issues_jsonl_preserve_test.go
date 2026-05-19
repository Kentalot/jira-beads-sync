package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIssueJSONLLineSkipsNonStringMetadata(t *testing.T) {
	const line = `{"id":"x","title":"t","status":"open","metadata":{"jiraPendingComment":"hi","count":42}}`
	l, err := parseIssueJSONLLine([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if l.Issue.Metadata["jiraPendingComment"] != "hi" {
		t.Fatalf("%v", l.Issue.Metadata)
	}
	if _, ok := l.Issue.Metadata["count"]; ok {
		t.Fatal("non-string metadata should not appear on BeadsIssue.Metadata")
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(l.Raw["metadata"], &meta); err != nil {
		t.Fatal(err)
	}
	if string(meta["count"]) != "42" {
		t.Fatalf("raw metadata should preserve count: %s", meta["count"])
	}
}

func TestSaveIssuesJSONLinesPreserveKeepsNativeTopLevelKeys(t *testing.T) {
	const original = `{"_type":"issue","id":"a","title":"t","status":"open","close_reason":"done","metadata":{"jiraKey":"K-1","jiraPendingComment":"n"}}` + "\n"
	tmp := t.TempDir()
	path := filepath.Join(tmp, "issues.jsonl")
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	lines, err := LoadIssuesJSONLinesPreserve(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(lines[0].Issue.Metadata, "jiraPendingComment")
	delete(lines[0].Issue.Metadata, "gitCommit")
	delete(lines[0].Issue.Metadata, "gitCommitUrl")
	if err := SaveIssuesJSONLinesPreserve(path, lines); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"_type"`) || !strings.Contains(s, `"close_reason"`) {
		t.Fatalf("lost native keys: %s", s)
	}
	if strings.Contains(s, `"jiraPendingComment"`) {
		t.Fatalf("pending should be stripped: %s", s)
	}
}
