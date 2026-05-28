package jira

import pb "github.com/Kentalot/jira-beads-sync/gen/jira"

// DependencyFetch is the result of fetching one or more Jira issues and their dependency tree.
type DependencyFetch struct {
	Export           *pb.Export
	AttachmentsByKey map[string][]Attachment
}
