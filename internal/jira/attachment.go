package jira

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Metadata keys stored on beads issues (metadata map in issues.jsonl).
const (
	MetadataKeyAttachmentsDir = "jiraAttachmentsDir"
	MetadataKeyAttachments    = "jiraAttachments"
	AttachmentsSubdir           = "jira-attachments"
)

// Attachment is a Jira issue attachment returned by the REST API.
type Attachment struct {
	ID       string
	Filename string
	Content  string // download URL
	Size     int64
}

// IssueAttachmentManifest records where quickstart stored files for an issue.
type IssueAttachmentManifest struct {
	RelativeDir string   // e.g. .beads/jira-attachments/PROJ-123
	Filenames   []string // basenames written on disk
}

type jsonAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Content  string `json:"content"`
	Size     int64  `json:"size"`
}

func attachmentsFromJSON(raw []jsonAttachment) []Attachment {
	if len(raw) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(raw))
	for _, a := range raw {
		if a.Filename == "" || a.Content == "" {
			continue
		}
		out = append(out, Attachment{
			ID:       a.ID,
			Filename: a.Filename,
			Content:  a.Content,
			Size:     a.Size,
		})
	}
	return out
}

// safeAttachmentFilename returns a basename safe for local storage.
func safeAttachmentFilename(filename string, id string) string {
	base := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if base == "" || base == "." || base == ".." {
		base = "attachment"
	}
	if id != "" {
		// Avoid collisions when multiple files share a name.
		return id + "_" + base
	}
	return base
}

// DownloadAttachments writes Jira attachments under outputDir/.beads/jira-attachments/<issueKey>/.
func (c *Client) DownloadAttachments(
	outputDir string,
	attachmentsByKey map[string][]Attachment,
) (map[string]IssueAttachmentManifest, error) {
	manifests := make(map[string]IssueAttachmentManifest)
	if len(attachmentsByKey) == 0 {
		return manifests, nil
	}

	totalFiles := 0
	for _, list := range attachmentsByKey {
		totalFiles += len(list)
	}
	if totalFiles == 0 {
		return manifests, nil
	}

	fmt.Printf("Downloading attachments for %d issue(s)...\n", len(attachmentsByKey))

	for issueKey, attachments := range attachmentsByKey {
		if len(attachments) == 0 {
			continue
		}
		relDir := filepath.ToSlash(filepath.Join(".beads", AttachmentsSubdir, issueKey))
		absDir := filepath.Join(outputDir, filepath.FromSlash(relDir))
		if err := os.MkdirAll(absDir, 0o755); err != nil {
			return nil, fmt.Errorf("create attachment dir for %s: %w", issueKey, err)
		}

		manifest := IssueAttachmentManifest{RelativeDir: relDir}
		for _, att := range attachments {
			name := safeAttachmentFilename(att.Filename, att.ID)
			dest := filepath.Join(absDir, name)
			if err := c.downloadAttachment(att.Content, dest); err != nil {
				fmt.Printf("  WARNING: %s: failed to download %q: %v\n", issueKey, att.Filename, err)
				continue
			}
			fmt.Printf("  %s: %s\n", issueKey, name)
			manifest.Filenames = append(manifest.Filenames, name)
		}
		if len(manifest.Filenames) > 0 {
			manifests[issueKey] = manifest
		}
	}

	return manifests, nil
}

func (c *Client) downloadAttachment(contentURL, dest string) error {
	req, err := http.NewRequest(http.MethodGet, contentURL, nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}
