package jira

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachmentsFromJSON(t *testing.T) {
	raw := []jsonAttachment{
		{ID: "1", Filename: "dragen.log", Content: "http://example.com/a", Size: 10},
		{Filename: "skip-no-url"},
		{ID: "2", Filename: "../evil.txt", Content: "http://example.com/b"},
	}
	got := attachmentsFromJSON(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(got))
	}
	if got[0].Filename != "dragen.log" {
		t.Errorf("unexpected first filename: %q", got[0].Filename)
	}
}

func TestSafeAttachmentFilename(t *testing.T) {
	if got := safeAttachmentFilename("logs/run.log", "99"); got != "99_run.log" {
		t.Errorf("got %q", got)
	}
	if got := safeAttachmentFilename("../x", ""); got != "x" {
		t.Errorf("got %q", got)
	}
}

func TestDownloadAttachments(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if strings.HasPrefix(r.URL.Path, "/content/") {
			_, _ = w.Write([]byte("log line\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "token", "basic")
	byKey := map[string][]Attachment{
		"PROJ-1": {
			{ID: "100", Filename: "out.log", Content: server.URL + "/content/100"},
		},
	}

	dir := t.TempDir()
	manifests, err := client.DownloadAttachments(dir, byKey)
	if err != nil {
		t.Fatalf("DownloadAttachments: %v", err)
	}
	if auth == "" {
		t.Fatal("expected Authorization header on download")
	}
	m, ok := manifests["PROJ-1"]
	if !ok || len(m.Filenames) != 1 {
		t.Fatalf("manifest: %+v", manifests)
	}
	path := filepath.Join(dir, ".beads", AttachmentsSubdir, "PROJ-1", m.Filenames[0])
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "log line\n" {
		t.Errorf("content = %q", data)
	}
	if m.RelativeDir != ".beads/jira-attachments/PROJ-1" {
		t.Errorf("relative dir = %q", m.RelativeDir)
	}
}
