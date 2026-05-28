package jira

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveAssigneeV2FallbackWhenV3Unavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/user/search":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>not available</html>"))
		case "/rest/api/2/user/search":
			switch r.URL.Query().Get("username") {
			case "kho":
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{
						"key":          "kho",
						"name":         "kho",
						"emailAddress": "kho@example.com",
						"displayName":  "Kent Ho",
					},
				})
			default:
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "u", "t", "basic")
	field, err := client.ResolveAssignee("", "kho@example.com")
	if err != nil {
		t.Fatalf("ResolveAssignee: %v", err)
	}
	if field["name"] != "kho" {
		t.Fatalf("assignee name: %#v", field)
	}
	if _, ok := field["accountId"]; ok {
		t.Fatalf("expected v2 name assignee, got accountId: %#v", field)
	}
}

func TestResolveAssigneeV3Cloud(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/user/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"accountId":    "acc-1",
				"emailAddress": "user@example.com",
				"displayName":  "User",
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "u", "t", "basic")
	field, err := client.ResolveAssignee("PROJ-1", "user@example.com")
	if err != nil {
		t.Fatalf("ResolveAssignee: %v", err)
	}
	if field["accountId"] != "acc-1" {
		t.Fatalf("assignee: %#v", field)
	}
}

func TestResolveAssigneeAssignableSearchByDisplayName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/user/search":
			w.WriteHeader(http.StatusNotFound)
		case "/rest/api/2/user/assignable/search":
			user := r.URL.Query().Get("username")
			if r.URL.Query().Get("issueKey") != "PROJ-2" || (user != "Kent Ho" && user != "Kent") {
				t.Fatalf("assignable search query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"key": "kho", "name": "kho", "displayName": "Kent Ho"},
			})
		case "/rest/api/2/user/search":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "u", "t", "basic")
	field, err := client.ResolveAssignee("PROJ-2", "Kent Ho")
	if err != nil {
		t.Fatalf("ResolveAssignee: %v", err)
	}
	if field["name"] != "kho" {
		t.Fatalf("assignee: %#v", field)
	}
}
