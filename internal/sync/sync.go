package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	jirapb "github.com/Kentalot/jira-beads-sync/gen/jira"
	"github.com/Kentalot/jira-beads-sync/internal/beads"
	"github.com/Kentalot/jira-beads-sync/internal/converter"
	"github.com/Kentalot/jira-beads-sync/internal/jira"
)

// Metadata keys for queued Jira comments (see docs/CLI_GUIDE.md).
const (
	metaJiraPendingComment      = "jiraPendingComment"
	metaGitCommit               = "gitCommit"
	metaGitCommitURL            = "gitCommitUrl"
	metaRepositories            = "repositories"
	metaJiraLastPostedCommentFP = "jiraLastPostedCommentFingerprint"
)

// RunOptions configures sync (description policy, optional git SHA for commit URLs).
type RunOptions struct {
	DescPolicy   string // "replace" or "skip"
	GitCommitSHA string // optional; also read from issue metadata when empty
}

// Run loads issuesPath and delegates to RunWithLines.
func Run(client *jira.Client, issuesPath string, filterKeys []string, opts RunOptions) error {
	lines, err := beads.LoadIssuesJSONLinesPreserve(issuesPath)
	if err != nil {
		return err
	}
	return RunWithLines(client, issuesPath, lines, filterKeys, opts)
}

// RunWithLines pushes beads issue changes to Jira for issues in lines (from .beads/issues.jsonl)
// that carry metadata.jiraKey or external_ref in the form "jira-KEY".
// filterKeys, if non-empty, limits sync to those Jira keys (case-insensitive, e.g. PROJ-123).
// After a Jira comment is posted (or deduped), issues.jsonl is saved immediately so a disk
// failure cannot leave a queued comment that would duplicate on retry.
func RunWithLines(client *jira.Client, issuesPath string, lines []beads.IssueJSONLLine, filterKeys []string, opts RunOptions) error {
	filter := normalizeKeySet(filterKeys)

	if len(lines) == 0 {
		return fmt.Errorf("no issues found in %s", issuesPath)
	}

	if len(filter) > 0 {
		inFile := jiraKeysInLines(lines)
		for k := range filter {
			if !inFile[k] {
				fmt.Fprintf(os.Stderr, "warning: %s not found in %s (no metadata.jiraKey or jira-* external_ref on any issue)\n", k, issuesPath)
			}
		}
	}
	pc := converter.NewProtoConverter()

	var synced, skipped, failed int

	for i := range lines {
		issue := &lines[i].Issue
		jkey := jiraKeyFromIssue(issue)
		if jkey == "" {
			fmt.Printf("skip %s: no Jira key (set metadata.jiraKey or external_ref like \"jira-PROJ-123\")\n", issue.ID)
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
		changed, rowDirty, err := syncOne(client, pc, issue, jkeyUpper, opts)
		if err != nil {
			fmt.Printf("  error: %v\n", err)
			failed++
			continue
		}
		if rowDirty {
			if saveErr := beads.SaveIssuesJSONLinesPreserve(issuesPath, lines); saveErr != nil {
				return fmt.Errorf("issue %s: Jira comment was processed but saving %s failed (retry may duplicate a Jira comment; remove metadata.jiraPendingComment manually if needed): %w", issue.ID, issuesPath, saveErr)
			}
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
	if issue.Metadata != nil {
		if k := strings.TrimSpace(issue.Metadata["jiraKey"]); k != "" {
			return k
		}
	}
	return jiraKeyFromExternalRef(issue.ExternalRef)
}

// JiraKeyForIssue returns the Jira issue key for a beads issue from metadata.jiraKey or
// external_ref "jira-KEY", or "" if none.
func JiraKeyForIssue(issue *beads.BeadsIssue) string {
	return jiraKeyFromIssue(issue)
}

func jiraKeyFromExternalRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	const prefix = "jira-"
	if len(ref) > len(prefix) && strings.EqualFold(ref[:len(prefix)], prefix) {
		return ref[len(prefix):]
	}
	return ""
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

func jiraKeysInLines(lines []beads.IssueJSONLLine) map[string]bool {
	m := make(map[string]bool)
	for i := range lines {
		k := jiraKeyFromIssue(&lines[i].Issue)
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

// localAssigneeForJira prefers an email (owner or assignee) for Jira user search.
func localAssigneeForJira(issue *beads.BeadsIssue) string {
	a := strings.TrimSpace(issue.Assignee)
	o := strings.TrimSpace(issue.Owner)
	if strings.Contains(strings.ToLower(a), "@") {
		return a
	}
	if o != "" {
		return o
	}
	return a
}

func buildPendingCommentBody(meta map[string]string, envSHA string) string {
	if meta == nil {
		return ""
	}
	msg := strings.TrimSpace(meta[metaJiraPendingComment])
	if msg == "" {
		return ""
	}
	sha := strings.TrimSpace(envSHA)
	if sha == "" {
		sha = strings.TrimSpace(meta[metaGitCommit])
	}
	link := strings.TrimSpace(meta[metaGitCommitURL])
	if link == "" && sha != "" {
		link = commitURLFromRepos(meta[metaRepositories], sha)
	}
	if link != "" {
		msg = msg + "\n\n" + link
	}
	return msg
}

func commitURLFromRepos(reposCSV, sha string) string {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return ""
	}
	for _, raw := range strings.Split(reposCSV, ",") {
		base := normalizeRepoBase(strings.TrimSpace(raw))
		if base != "" {
			return strings.TrimSuffix(base, "/") + "/commit/" + sha
		}
	}
	return ""
}

func normalizeRepoBase(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	low := strings.ToLower(repo)

	const gh = "git@github.com:"
	if len(repo) >= len(gh) && low[:len(gh)] == gh {
		return "https://github.com/" + trimGitSuffix(repo[len(gh):])
	}
	if strings.Contains(low, "github.com/") {
		return trimGitSuffix(repo)
	}
	return ""
}

func trimGitSuffix(s string) string {
	s = strings.TrimSuffix(s, "/")
	return strings.TrimSuffix(s, ".git")
}

func commentBodyFingerprint(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

func clearPendingJiraQueueMetadata(issue *beads.BeadsIssue) {
	if issue.Metadata == nil {
		return
	}
	delete(issue.Metadata, metaJiraPendingComment)
	delete(issue.Metadata, metaGitCommit)
	delete(issue.Metadata, metaGitCommitURL)
}

func applyAfterSuccessfulJiraComment(issue *beads.BeadsIssue, postedBody string) {
	clearPendingJiraQueueMetadata(issue)
	if issue.Metadata == nil {
		issue.Metadata = make(map[string]string)
	}
	issue.Metadata[metaJiraLastPostedCommentFP] = commentBodyFingerprint(postedBody)
}

func syncOne(client *jira.Client, pc *converter.ProtoConverter, local *beads.BeadsIssue, jiraKey string, opts RunOptions) (changed bool, jsonlDirty bool, err error) {
	fetched, err := client.FetchIssueWithHints(jiraKey)
	if err != nil {
		return false, false, err
	}
	remote := fetched.Issue

	pendingBody := buildPendingCommentBody(local.Metadata, opts.GitCommitSHA)

	wantStatus := strings.TrimSpace(local.Status)
	if wantStatus == "" {
		wantStatus = "open"
	}

	remoteStatus := pc.BeadsStatusStringFromJira(remote.Fields.Status)
	remoteRank := pc.BeadsPriorityRankFromJira(remote.Fields.Priority)
	remoteAssignee := remoteAssigneeString(remote)
	localAssignee := localAssigneeForJira(local)

	titleChanged := strings.TrimSpace(local.Title) != strings.TrimSpace(remote.Fields.Summary)
	descChanged := strings.TrimSpace(local.Description) != strings.TrimSpace(remote.Fields.Description)
	statusChanged := wantStatus != remoteStatus
	prioChanged := local.Priority != remoteRank
	assigneeChanged := !assigneesEqual(localAssignee, remoteAssignee)

	mayPushDescription := false
	switch strings.ToLower(strings.TrimSpace(opts.DescPolicy)) {
	case "skip":
		mayPushDescription = false
	default: // replace
		mayPushDescription = descChanged && !fetched.DescriptionPresentButUnparsed
		if fetched.DescriptionPresentButUnparsed && descChanged {
			fmt.Printf("  warning: skipping Jira description update (remote description is rich text / ADF we cannot round-trip)\n")
		}
	}

	if !titleChanged && !mayPushDescription && !statusChanged && !prioChanged && !assigneeChanged && pendingBody == "" {
		return false, false, nil
	}

	// Post queued Jira comment first (even when the issue is already closed in Jira) so a
	// follow-up note is not skipped if later steps error.
	if pendingBody != "" {
		fp := commentBodyFingerprint(pendingBody)
		if local.Metadata != nil && local.Metadata[metaJiraLastPostedCommentFP] == fp {
			fmt.Printf("  skipping Jira comment (same content as last posted fingerprint); clearing queued metadata\n")
			clearPendingJiraQueueMetadata(local)
			changed = true
			jsonlDirty = true
		} else {
			if err := client.AddIssueComment(jiraKey, pendingBody); err != nil {
				return false, false, err
			}
			applyAfterSuccessfulJiraComment(local, pendingBody)
			changed = true
			jsonlDirty = true
		}
	}

	fields := map[string]any{}
	if titleChanged {
		fields["summary"] = local.Title
	}
	if mayPushDescription {
		fields["description"] = local.Description
	}
	if prioChanged {
		id, err := resolvePriorityID(client, jiraKey, local.Priority)
		if err != nil {
			return false, false, fmt.Errorf("priority: %w", err)
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
				return false, false, fmt.Errorf("assignee: %w", err)
			}
			fields["assignee"] = map[string]any{"accountId": accountID}
		}
	}

	if len(fields) > 0 {
		if err := client.UpdateIssueFields(jiraKey, fields); err != nil {
			return false, false, err
		}
		changed = true
	}

	if statusChanged {
		transitions, err := client.ListTransitions(jiraKey)
		if err != nil {
			return false, false, fmt.Errorf("status: %w", err)
		}
		tid, err := pickTransition(pc, transitions, wantStatus)
		if err != nil {
			return false, false, fmt.Errorf("status: %w", err)
		}
		if err := client.DoTransition(jiraKey, tid); err != nil {
			return false, false, fmt.Errorf("status: %w", err)
		}
		changed = true
	}

	return changed, jsonlDirty, nil
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
