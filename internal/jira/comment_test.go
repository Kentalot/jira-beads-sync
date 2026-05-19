package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAddIssueCommentV2Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issue/PROJ-1/comment" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatal(err)
		}
		if body["body"] != "hello" {
			t.Fatalf("body: %v", body["body"])
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "t", "basic")
	if err := c.AddIssueComment("PROJ-1", "hello"); err != nil {
		t.Fatal(err)
	}
}

func TestAddIssueCommentV2FailsV3Works(t *testing.T) {
	var v3hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/2/issue/PROJ-1/comment":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errorMessages":["use ADF"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issue/PROJ-1/comment":
			v3hit = true
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"type":"doc"`) {
				t.Fatalf("expected ADF doc: %s", b)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "t", "basic")
	if err := c.AddIssueComment("PROJ-1", "hello"); err != nil {
		t.Fatal(err)
	}
	if !v3hit {
		t.Fatal("expected v3 comment fallback")
	}
}

func TestFetchIssueWithHintsADF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"key": "PROJ-1",
			"id":  "1",
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
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "t", "basic")
	f, err := c.FetchIssueWithHints("PROJ-1")
	if err != nil {
		t.Fatal(err)
	}
	if !f.DescriptionPresentButUnparsed {
		t.Fatal("expected ADF hint")
	}
	if f.Issue.Fields.Description != "" {
		t.Fatalf("parsed description should be empty, got %q", f.Issue.Fields.Description)
	}
}
