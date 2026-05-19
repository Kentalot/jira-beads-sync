package sync

import (
	"os/exec"
	"strings"

	"github.com/Kentalot/jira-beads-sync/internal/beads"
)

// BranchContainsJiraKey reports whether branchName contains jiraKey as a standalone token
// (so PROJ-123 does not match inside PROJ-1234).
func BranchContainsJiraKey(branchName, jiraKey string) bool {
	b := strings.ToUpper(strings.TrimSpace(branchName))
	k := strings.ToUpper(strings.TrimSpace(jiraKey))
	if b == "" || k == "" {
		return false
	}
	for {
		idx := strings.Index(b, k)
		if idx < 0 {
			return false
		}
		beforeOK := idx == 0 || !isJiraKeyRune(rune(b[idx-1]))
		after := idx + len(k)
		afterOK := after >= len(b) || !isJiraKeyRune(rune(b[after]))
		if beforeOK && afterOK {
			return true
		}
		b = b[idx+1:]
	}
}

func isJiraKeyRune(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ApplyBranchScopedGitPending, when explicitFilter is empty, inspects the current git branch
// in workDir and the Jira keys on each beads line. If some line's key appears in the branch
// name, it sets metadata.jiraPendingComment from the latest commit message, gitCommitUrl when
// derivable from origin, ensures opts.GitCommitSHA is set from HEAD when empty, and returns
// a single-element filter for that Jira key. Otherwise returns nil (caller syncs all issues).
func ApplyBranchScopedGitPending(workDir string, lines []beads.IssueJSONLLine, explicitFilter []string, opts *RunOptions) []string {
	if len(explicitFilter) > 0 {
		return explicitFilter
	}
	branch := gitRevParseAbbrevRef(workDir)
	if branch == "" || branch == "HEAD" {
		return nil
	}
	for i := range lines {
		key := JiraKeyForIssue(&lines[i].Issue)
		if key == "" || !BranchContainsJiraKey(branch, key) {
			continue
		}
		sha := strings.TrimSpace(opts.GitCommitSHA)
		if sha == "" {
			sha = gitRevParseHead(workDir)
		}
		if sha != "" {
			opts.GitCommitSHA = sha
		}
		msg := strings.TrimSpace(gitLog1PrettyMessage(workDir))
		if msg == "" {
			msg = strings.TrimSpace(gitLog1Subject(workDir))
		}
		if lines[i].Issue.Metadata == nil {
			lines[i].Issue.Metadata = make(map[string]string)
		}
		if msg != "" {
			lines[i].Issue.Metadata[metaJiraPendingComment] = msg
		}
		if u := commitURLFromGitOrigin(workDir, sha); u != "" {
			lines[i].Issue.Metadata[metaGitCommitURL] = u
		}
		return []string{strings.ToUpper(key)}
	}
	return nil
}

func gitRevParseAbbrevRef(workDir string) string {
	out, err := exec.Command("git", "-C", workDir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitRevParseHead(workDir string) string {
	out, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitLog1PrettyMessage(workDir string) string {
	out, err := exec.Command("git", "-C", workDir, "log", "-1", "--pretty=%B").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitLog1Subject(workDir string) string {
	out, err := exec.Command("git", "-C", workDir, "log", "-1", "--pretty=%s").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitRemoteGetURLOrigin(workDir string) string {
	out, err := exec.Command("git", "-C", workDir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func commitURLFromGitOrigin(workDir, sha string) string {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return ""
	}
	remote := gitRemoteGetURLOrigin(workDir)
	base := normalizeGitRemoteToHTTPSBase(remote)
	if base == "" {
		return ""
	}
	base = strings.TrimSuffix(base, "/")
	return base + "/commit/" + sha
}

// normalizeGitRemoteToHTTPSBase turns common git remote forms into an https repo root
// (no trailing slash, no .git), or "" if unsupported.
func normalizeGitRemoteToHTTPSBase(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	low := strings.ToLower(remote)

	const gh = "git@github.com:"
	if len(remote) >= len(gh) && low[:len(gh)] == gh {
		return "https://github.com/" + trimGitSuffix(remote[len(gh):])
	}
	if strings.HasPrefix(low, "git@") {
		rest := remote[len("git@"):]
		at := strings.Index(rest, ":")
		if at <= 0 {
			return ""
		}
		host, path := rest[:at], rest[at+1:]
		if path == "" {
			return ""
		}
		return "https://" + host + "/" + trimGitSuffix(path)
	}
	if strings.HasPrefix(low, "ssh://") {
		u := remote[len("ssh://"):]
		u = strings.TrimPrefix(u, "git@")
		slash := strings.Index(u, "/")
		if slash <= 0 {
			return ""
		}
		host, path := u[:slash], u[slash+1:]
		if host == "" || path == "" {
			return ""
		}
		return "https://" + host + "/" + trimGitSuffix(path)
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return trimGitSuffix(remote)
	}
	return ""
}
