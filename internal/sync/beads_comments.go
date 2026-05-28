package sync

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Kentalot/jira-beads-sync/internal/beads"
	"github.com/Kentalot/jira-beads-sync/internal/jira"
)

const (
	metaJiraPostedCommentIDs = "jiraPostedCommentIds"

	// BeadsCommentsTagged: only comments containing #jira are pushed (default).
	BeadsCommentsTagged = "tagged"
	// BeadsCommentsAll: push every beads comment not yet recorded in jiraPostedCommentIds.
	BeadsCommentsAll = "all"
	// BeadsCommentsOff: do not read or push native beads comments.
	BeadsCommentsOff = "off"
)

var jiraTagPattern = regexp.MustCompile(`(?i)#jira\b`)

// NormalizeBeadsCommentsPolicy returns tagged, all, or off.
func NormalizeBeadsCommentsPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case BeadsCommentsAll:
		return BeadsCommentsAll
	case BeadsCommentsOff:
		return BeadsCommentsOff
	default:
		return BeadsCommentsTagged
	}
}

func commentEligibleForJira(text, policy string) bool {
	switch NormalizeBeadsCommentsPolicy(policy) {
	case BeadsCommentsAll:
		return strings.TrimSpace(text) != ""
	case BeadsCommentsTagged:
		return jiraTagPattern.MatchString(text)
	default:
		return false
	}
}

func stripJiraTag(text string) string {
	out := jiraTagPattern.ReplaceAllString(text, "")
	return strings.TrimSpace(out)
}

func jiraBodyFromBeadsComment(c beads.Comment) string {
	body := stripJiraTag(c.Text)
	if body == "" {
		return ""
	}
	author := strings.TrimSpace(c.Author)
	if author == "" {
		return body
	}
	return fmt.Sprintf("[%s] %s", author, body)
}

func parsePostedCommentIDSet(meta map[string]string) map[string]struct{} {
	out := make(map[string]struct{})
	if meta == nil {
		return out
	}
	raw := strings.TrimSpace(meta[metaJiraPostedCommentIDs])
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func formatPostedCommentIDs(ids map[string]struct{}) string {
	if len(ids) == 0 {
		return ""
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Strings(list)
	return strings.Join(list, ",")
}

func recordPostedCommentID(meta map[string]string, commentID string) {
	commentID = strings.TrimSpace(commentID)
	if commentID == "" {
		return
	}
	set := parsePostedCommentIDSet(meta)
	set[commentID] = struct{}{}
	if meta == nil {
		return
	}
	meta[metaJiraPostedCommentIDs] = formatPostedCommentIDs(set)
}

type beadsCommentToPost struct {
	ID   string
	Body string
}

func selectBeadsCommentsForJira(comments []beads.Comment, posted map[string]struct{}, policy string) []beadsCommentToPost {
	if NormalizeBeadsCommentsPolicy(policy) == BeadsCommentsOff {
		return nil
	}
	var out []beadsCommentToPost
	for _, c := range comments {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			continue
		}
		if _, done := posted[id]; done {
			continue
		}
		if !commentEligibleForJira(c.Text, policy) {
			continue
		}
		body := jiraBodyFromBeadsComment(c)
		if body == "" {
			continue
		}
		out = append(out, beadsCommentToPost{ID: id, Body: body})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func listBeadsCommentsForIssue(opts RunOptions, issueID string, raw map[string]json.RawMessage) ([]beads.Comment, error) {
	if opts.ListBeadsComments != nil {
		return opts.ListBeadsComments(issueID, raw)
	}
	var rawComments []byte
	if raw != nil {
		if b, ok := raw["comments"]; ok {
			rawComments = b
		}
	}
	return beads.ListComments(opts.WorkDir, issueID, rawComments)
}

// postBeadsCommentsToJira posts pre-selected beads comments and records their IDs in metadata.
func postBeadsCommentsToJira(client *jira.Client, local *beads.BeadsIssue, jiraKey string, toPost []beadsCommentToPost) (changed bool, err error) {
	if len(toPost) == 0 {
		return false, nil
	}
	if local.Metadata == nil {
		local.Metadata = make(map[string]string)
	}

	for _, item := range toPost {
		fmt.Printf("  posting beads comment %s to Jira ...\n", item.ID)
		if err := client.AddIssueComment(jiraKey, item.Body); err != nil {
			return changed, fmt.Errorf("beads comment %s: %w", item.ID, err)
		}
		recordPostedCommentID(local.Metadata, item.ID)
		changed = true
	}
	return changed, nil
}
