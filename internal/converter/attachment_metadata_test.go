package converter

import (
	"strings"
	"testing"

	beadspb "github.com/Kentalot/jira-beads-sync/gen/beads"
	jirapb "github.com/Kentalot/jira-beads-sync/gen/jira"
	jirasync "github.com/Kentalot/jira-beads-sync/internal/jira"
)

func TestConvertWithAttachmentMetadata(t *testing.T) {
	jiraExport := &jirapb.Export{
		Issues: []*jirapb.Issue{
			{
				Id:  "1",
				Key: "PROJ-9",
				Fields: &jirapb.Fields{
					Summary: "Bug",
					IssueType: &jirapb.IssueType{Name: "Bug"},
					Status: &jirapb.Status{
						Name: "Open",
						StatusCategory: &jirapb.StatusCategory{Key: "new"},
					},
					Priority: &jirapb.Priority{Name: "High"},
				},
			},
		},
	}
	manifests := map[string]jirasync.IssueAttachmentManifest{
		"PROJ-9": {
			RelativeDir: ".beads/jira-attachments/PROJ-9",
			Filenames:   []string{"100_dragen.log"},
		},
	}

	conv := NewProtoConverter()
	beadsExport, err := conv.Convert(jiraExport, manifests)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(beadsExport.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(beadsExport.Issues))
	}
	meta := beadsExport.Issues[0].Metadata
	if meta == nil || meta.Custom == nil {
		t.Fatal("expected metadata.custom")
	}
	if meta.Custom[jirasync.MetadataKeyAttachmentsDir] != ".beads/jira-attachments/PROJ-9" {
		t.Errorf("dir: %v", meta.Custom)
	}
	if !strings.Contains(meta.Custom[jirasync.MetadataKeyAttachments], "dragen.log") {
		t.Errorf("files: %v", meta.Custom)
	}
}

func TestApplyAttachmentMetadataEmpty(t *testing.T) {
	issue := &beadspb.Issue{Metadata: &beadspb.Metadata{}}
	applyAttachmentMetadata(issue, jirasync.IssueAttachmentManifest{})
	if issue.Metadata.Custom != nil {
		t.Error("expected no custom metadata for empty manifest")
	}
}
