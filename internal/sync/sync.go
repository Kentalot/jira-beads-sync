package sync

import (
	"fmt"
	"os"
	"strings"

	jirapb "github.com/conallob/jira-beads-sync/gen/jira"
	"github.com/conallob/jira-beads-sync/internal/beads"
	"github.com/conallob/jira-beads-sync/internal/converter"
	"github.com/conallob/jira-beads-sync/internal/jira"
)

// Run pushes beads issue changes to Jira for issues listed in .beads/issues.jsonl that carry metadata.jiraKey.
// filterKeys, if non-empty, limits sync to those Jira keys (case-insensitive, e.g. PROJ-123).
func Run(client *jira.Client, issues []beads.BeadsIssue, filterKeys []string) error {
	filter := normalizeKeySet(filterKeys)
	if len(filter) > 0 {
		inFile := jiraKeysInIssues(issues)
		for k := range filter {
			if !inFile[k] {
				fmt.Fprintf(os.Stderr, "warning: %s not found in .beads/issues.jsonl (missing metadata.jiraKey on any issue)\n", k)
			}
		}
	}
	pc := converter.NewProtoConverter()

	var synced, skipped, failed int
	for i := range issues {
		issue := &issues[i]
		jkey := jiraKeyFromIssue(issue)
		if jkey == "" {
			fmt.Printf("skip %s: no metadata.jiraKey\n", issue.ID)
			skipped++
			continue
		}
		jkeyUpper := strings.ToUpper(jkey)
		if len(filter) > 0 {
			if _, ok := filter[jkeyUpper]; !ok {
				continue
			}
		}

		if err := validateKey(jkeyUpper); err != nil {
			fmt.Printf("skip %s (%s): %v\n", issue.ID, jkeyUpper, err)
			skipped++
			continue
		}

		fmt.Printf("sync %s -> %s ...\n", issue.ID, jkeyUpper)
		changed, err := syncOne(client, pc, issue, jkeyUpper)
		if err != nil {
			fmt.Printf("  error: %v\n", err)
			failed++
			continue
		}
		if !changed {
			fmt.Printf("  already up to date with Jira\n")
			skipped++
			continue
		}
		fmt.Printf("  ok\n")
		synced++
	}

	fmt.Printf("\nDone. Pushed updates for %d issue(s); %d unchanged or skipped (no key); %d failed.\n", synced, skipped, failed)
	if failed > 0 {
		return fmt.Errorf("sync completed with %d error(s)", failed)
	}
	return nil
}

func validateKey(key string) error {
	return jira.ValidateIssueKey(key)
}

func jiraKeyFromIssue(issue *beads.BeadsIssue) string {
	if issue.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(issue.Metadata["jiraKey"])
}

func normalizeKeySet(keys []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[strings.ToUpper(k)] = struct{}{}
	}
	return out
}

func jiraKeysInIssues(issues []beads.BeadsIssue) map[string]bool {
	m := make(map[string]bool)
	for i := range issues {
		k := jiraKeyFromIssue(&issues[i])
		if k == "" {
			continue
		}
		m[strings.ToUpper(k)] = true
	}
	return m
}

func remoteAssigneeString(remote *jirapb.Issue) string {
	if remote.Fields == nil || remote.Fields.Assignee == nil {
		return ""
	}
	u := remote.Fields.Assignee
	if strings.TrimSpace(u.EmailAddress) != "" {
		return strings.TrimSpace(u.EmailAddress)
	}
	return strings.TrimSpace(u.DisplayName)
}

func syncOne(client *jira.Client, pc *converter.ProtoConverter, local *beads.BeadsIssue, jiraKey string) (bool, error) {
	remote, err := client.FetchIssue(jiraKey)
	if err != nil {
		return false, err
	}

	wantStatus := strings.TrimSpace(local.Status)
	if wantStatus == "" {
		wantStatus = "open"
	}

	remoteStatus := pc.BeadsStatusStringFromJira(remote.Fields.Status)
	remoteRank := pc.BeadsPriorityRankFromJira(remote.Fields.Priority)
	remoteAssignee := remoteAssigneeString(remote)
	localAssignee := strings.TrimSpace(local.Assignee)

	titleChanged := strings.TrimSpace(local.Title) != strings.TrimSpace(remote.Fields.Summary)
	descChanged := strings.TrimSpace(local.Description) != strings.TrimSpace(remote.Fields.Description)
	statusChanged := wantStatus != remoteStatus
	prioChanged := local.Priority != remoteRank
	assigneeChanged := !assigneesEqual(localAssignee, remoteAssignee)

	if !titleChanged && !descChanged && !statusChanged && !prioChanged && !assigneeChanged {
		return false, nil
	}

	fields := map[string]any{}
	if titleChanged {
		fields["summary"] = local.Title
	}
	if descChanged {
		fields["description"] = local.Description
	}
	if prioChanged {
		id, err := resolvePriorityID(client, jiraKey, local.Priority)
		if err != nil {
			return fmt.Errorf("priority: %w", err)
		}
		if id != "" {
			fields["priority"] = map[string]any{"id": id}
		}
	}
	if assigneeChanged {
		if localAssignee == "" {
			fields["assignee"] = nil
		} else {
			accountID, err := client.SearchUserAccountID(localAssignee)
			if err != nil {
				return fmt.Errorf("assignee: %w", err)
			}
			fields["assignee"] = map[string]any{"accountId": accountID}
		}
	}

	if len(fields) > 0 {
		if err := client.UpdateIssueFields(jiraKey, fields); err != nil {
			return false, err
		}
	}

	if statusChanged {
		transitions, err := client.ListTransitions(jiraKey)
		if err != nil {
			return false, fmt.Errorf("status: %w", err)
		}
		tid, err := pickTransition(pc, transitions, wantStatus)
		if err != nil {
			return false, fmt.Errorf("status: %w", err)
		}
		if err := client.DoTransition(jiraKey, tid); err != nil {
			return false, err
		}
	}

	return true, nil
}

func assigneesEqual(local, remote string) bool {
	a, b := strings.TrimSpace(local), strings.TrimSpace(remote)
	if a == "" && b == "" {
		return true
	}
	if strings.Contains(strings.ToLower(a), "@") && strings.Contains(strings.ToLower(b), "@") {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(a, b)
}

func resolvePriorityID(client *jira.Client, jiraKey string, beadsRank int) (string, error) {
	opts, err := client.ListEditablePriorities(jiraKey)
	if err != nil {
		return "", err
	}
	if len(opts) == 0 {
		return "", fmt.Errorf("no editable priorities for %s", jiraKey)
	}
	if id, ok := matchPriorityOption(opts, beadsRank); ok {
		return id, nil
	}
	return "", fmt.Errorf("could not map beads priority rank %d to a Jira priority for %s", beadsRank, jiraKey)
}

func matchPriorityOption(options []jira.PriorityOption, beadsRank int) (string, bool) {
	if beadsRank < 0 || beadsRank > 4 {
		return "", false
	}
	patterns := [][]string{
		{"blocker", "critical", "highest"},
		{"highest", "major"},
		{"medium", "normal"},
		{"low", "minor"},
		{"lowest", "trivial"},
	}
	want := patterns[beadsRank]
	for _, opt := range options {
		ln := strings.ToLower(opt.Name)
		for _, p := range want {
			if strings.Contains(ln, p) {
				return opt.ID, true
			}
		}
	}
	return "", false
}

func pickTransition(pc *converter.ProtoConverter, transitions []jira.Transition, wantStatus string) (string, error) {
	var example string
	for _, tr := range transitions {
		st := &jirapb.Status{
			Name: tr.To.Name,
			StatusCategory: &jirapb.StatusCategory{
				Key:  tr.To.StatusCategory.Key,
				Name: tr.To.StatusCategory.Name,
			},
		}
		got := pc.BeadsStatusStringFromJira(st)
		if got == wantStatus {
			return tr.ID, nil
		}
		if example == "" {
			example = tr.Name
		}
	}
	if len(transitions) == 0 {
		return "", fmt.Errorf("no workflow transitions available")
	}
	return "", fmt.Errorf("no transition maps to beads status %q (try adjusting status in beads). Example transition: %q", wantStatus, example)
}
