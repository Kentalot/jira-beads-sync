package sync

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kentalot/jira-beads-sync/internal/beads"
	"github.com/Kentalot/jira-beads-sync/internal/converter"
	"github.com/Kentalot/jira-beads-sync/internal/jira"
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

func TestJiraKeyFromExternalRef(t *testing.T) {
	if got := jiraKeyFromExternalRef("jira-WTF-26227"); got != "WTF-26227" {
		t.Fatalf("got %q", got)
	}
	if got := jiraKeyFromExternalRef("JIRA-abc-1"); got != "abc-1" {
		t.Fatalf("got %q", got)
	}
	if jiraKeyFromExternalRef("") != "" || jiraKeyFromExternalRef("other") != "" {
		t.Fatal("expected empty")
	}
}

func TestRunADFSkipsDescriptionOnPut(t *testing.T) {
	var lastPut []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/PROJ-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key": "PROJ-1",
				"id":  "99",
				"fields": map[string]any{
					"summary":     "S",
					"description": map[string]any{"type": "doc", "version": 1},
					"issuetype":   map[string]any{"name": "Story"},
					"status": map[string]any{
						"name":           "To Do",
						"statusCategory": map[string]any{"key": "new"},
					},
					"priority": map[string]any{"name": "Medium", "id": "3"},
					"created":  "2024-01-01T10:00:00.000+0000",
					"updated":  "2024-01-15T14:30:00.000+0000",
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/rest/api/2/issue/PROJ-1":
			lastPut, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	beadsDir := filepath.Join(tmp, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")

	row := map[string]any{
		"id":          "bd-1",
		"title":       "S2",
		"description": "beads-only",
		"status":      "open",
		"priority":    2,
		"metadata": map[string]string{
			"jiraKey": "PROJ-1",
		},
	}
	f, err := os.Create(issuesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(row); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	client := jira.NewClient(srv.URL, "u", "t", "basic")
	if err := Run(client, issuesPath, nil, RunOptions{DescPolicy: "replace"}); err != nil {
		t.Fatal(err)
	}
	if len(lastPut) == 0 {
		t.Fatal("expected PUT")
	}
	var payload struct {
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(lastPut, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.Fields["description"]; ok {
		t.Fatalf("PUT should not include description with ADF remote, got %s", string(lastPut))
	}
	if payload.Fields["summary"] != "S2" {
		t.Fatalf("summary: %v", payload.Fields["summary"])
	}
}

func TestRunPostsBeadsCommentWithJiraTag(t *testing.T) {
	var commentBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/PROJ-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key": "PROJ-1",
				"id":  "99",
				"fields": map[string]any{
					"summary":     "S",
					"description": "same",
					"issuetype":   map[string]any{"name": "Story"},
					"status": map[string]any{
						"name":           "To Do",
						"statusCategory": map[string]any{"key": "new"},
					},
					"priority": map[string]any{"name": "Medium", "id": "3"},
					"created":  "2024-01-01T10:00:00.000+0000",
					"updated":  "2024-01-15T14:30:00.000+0000",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/2/issue/PROJ-1/comment":
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			commentBodies = append(commentBodies, body.Body)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	beadsDir := filepath.Join(tmp, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")

	row := map[string]any{
		"id": "bd-1", "title": "S", "description": "same", "status": "open", "priority": 2,
		"metadata": map[string]string{"jiraKey": "PROJ-1"},
		"comments": []map[string]any{
			{"id": "cmt-1", "issue_id": "bd-1", "author": "Agent", "text": "Ready for review #jira"},
		},
	}
	f, err := os.Create(issuesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(row); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	client := jira.NewClient(srv.URL, "u", "t", "basic")
	if err := Run(client, issuesPath, []string{"PROJ-1"}, RunOptions{
		DescPolicy:          "replace",
		BeadsCommentsPolicy: BeadsCommentsTagged,
	}); err != nil {
		t.Fatal(err)
	}
	if len(commentBodies) != 1 {
		t.Fatalf("comments posted: %d", len(commentBodies))
	}
	if commentBodies[0] != "[Agent] Ready for review" {
		t.Fatalf("body %q", commentBodies[0])
	}

	lines, err := beads.LoadIssuesJSONLinesPreserve(issuesPath)
	if err != nil {
		t.Fatal(err)
	}
	if lines[0].Issue.Metadata[metaJiraPostedCommentIDs] != "cmt-1" {
		t.Fatalf("posted ids %q", lines[0].Issue.Metadata[metaJiraPostedCommentIDs])
	}

	if err := Run(client, issuesPath, []string{"PROJ-1"}, RunOptions{
		DescPolicy:          "replace",
		BeadsCommentsPolicy: BeadsCommentsTagged,
	}); err != nil {
		t.Fatal(err)
	}
	if len(commentBodies) != 1 {
		t.Fatalf("second sync should not repost, got %d bodies", len(commentBodies))
	}
}

func TestRunSkipsBeadsCommentWithoutJiraTag(t *testing.T) {
	var commentPosted int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/PROJ-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key": "PROJ-1",
				"id":  "99",
				"fields": map[string]any{
					"summary":     "S",
					"description": "same",
					"issuetype":   map[string]any{"name": "Story"},
					"status": map[string]any{
						"name":           "To Do",
						"statusCategory": map[string]any{"key": "new"},
					},
					"priority": map[string]any{"name": "Medium", "id": "3"},
					"created":  "2024-01-01T10:00:00.000+0000",
					"updated":  "2024-01-15T14:30:00.000+0000",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/2/issue/PROJ-1/comment":
			commentPosted++
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	beadsDir := filepath.Join(tmp, ".beads")
	_ = os.MkdirAll(beadsDir, 0755)
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	row := map[string]any{
		"id": "bd-1", "title": "S", "description": "same", "status": "open", "priority": 2,
		"metadata": map[string]string{"jiraKey": "PROJ-1"},
		"comments": []map[string]any{
			{"id": "cmt-1", "text": "local only"},
		},
	}
	f, _ := os.Create(issuesPath)
	_ = json.NewEncoder(f).Encode(row)
	_ = f.Close()

	client := jira.NewClient(srv.URL, "u", "t", "basic")
	if err := Run(client, issuesPath, []string{"PROJ-1"}, RunOptions{BeadsCommentsPolicy: BeadsCommentsTagged}); err != nil {
		t.Fatal(err)
	}
	if commentPosted != 0 {
		t.Fatalf("expected no comment, got %d", commentPosted)
	}
}
