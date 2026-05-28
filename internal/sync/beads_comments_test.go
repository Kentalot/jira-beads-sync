package sync

import (
	"testing"

	"github.com/Kentalot/jira-beads-sync/internal/beads"
)

func TestCommentEligibleForJira(t *testing.T) {
	if !commentEligibleForJira("done #jira", BeadsCommentsTagged) {
		t.Fatal("expected tagged match")
	}
	if commentEligibleForJira("internal note", BeadsCommentsTagged) {
		t.Fatal("expected no match without tag")
	}
	if !commentEligibleForJira("any text", BeadsCommentsAll) {
		t.Fatal("expected all policy")
	}
	if commentEligibleForJira("x", BeadsCommentsOff) {
		t.Fatal("expected off")
	}
}

func TestStripJiraTag(t *testing.T) {
	got := stripJiraTag("Shipped fix.  #jira  ")
	if got != "Shipped fix." {
		t.Fatalf("got %q", got)
	}
}

func TestSelectBeadsCommentsForJira(t *testing.T) {
	comments := []beads.Comment{
		{ID: "c1", Text: "skip me"},
		{ID: "c2", Text: "push #jira"},
		{ID: "c3", Text: "already"},
	}
	posted := map[string]struct{}{"c3": {}}
	got := selectBeadsCommentsForJira(comments, posted, BeadsCommentsTagged)
	if len(got) != 1 || got[0].ID != "c2" || got[0].Body != "push" {
		t.Fatalf("got %+v", got)
	}
}

func TestPostedCommentIDRoundTrip(t *testing.T) {
	meta := map[string]string{}
	recordPostedCommentID(meta, "id-a")
	recordPostedCommentID(meta, "id-b")
	set := parsePostedCommentIDSet(meta)
	if len(set) != 2 {
		t.Fatalf("set %v", set)
	}
	if meta[metaJiraPostedCommentIDs] != "id-a,id-b" {
		t.Fatalf("stored %q", meta[metaJiraPostedCommentIDs])
	}
}

func TestJiraBodyFromBeadsComment(t *testing.T) {
	body := jiraBodyFromBeadsComment(beads.Comment{Author: "Bot", Text: "note #jira"})
	if body != "[Bot] note" {
		t.Fatalf("got %q", body)
	}
}
