package beads

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// IssueJSONLLine is one line of issues.jsonl: the full JSON object as a map (preserving
// unknown keys such as native beads _type, created_at, close_reason) plus the same line
// unmarshaled into BeadsIssue for fields this tool understands.
type IssueJSONLLine struct {
	Raw   map[string]json.RawMessage
	Issue BeadsIssue
}

func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

// parseIssueJSONLLine parses one JSONL object into a raw key map (preserves all keys/values)
// and a BeadsIssue for fields this tool uses. Non-string metadata values are omitted from
// BeadsIssue.Metadata but remain in Raw["metadata"].
func parseIssueJSONLLine(rawBytes []byte) (IssueJSONLLine, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		return IssueJSONLLine{}, fmt.Errorf("raw map: %w", err)
	}
	work := cloneRawMap(raw)
	delete(work, "metadata")
	stripped, err := json.Marshal(work)
	if err != nil {
		return IssueJSONLLine{}, err
	}
	var issue BeadsIssue
	if err := json.Unmarshal(stripped, &issue); err != nil {
		return IssueJSONLLine{}, fmt.Errorf("issue fields: %w", err)
	}
	if mr, ok := raw["metadata"]; ok && len(mr) > 0 && string(bytes.TrimSpace(mr)) != "null" {
		var metaAny map[string]json.RawMessage
		if err := json.Unmarshal(mr, &metaAny); err != nil {
			return IssueJSONLLine{}, fmt.Errorf("metadata: %w", err)
		}
		issue.Metadata = make(map[string]string)
		for k, v := range metaAny {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				continue
			}
			issue.Metadata[k] = s
		}
		if len(issue.Metadata) == 0 {
			issue.Metadata = nil
		}
	}
	return IssueJSONLLine{Raw: raw, Issue: issue}, nil
}

// LoadIssuesJSONLinesPreserve reads issues.jsonl preserving every top-level JSON key on each line.
func LoadIssuesJSONLinesPreserve(path string) ([]IssueJSONLLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open issues jsonl: %w", err)
	}
	defer f.Close()

	var lines []IssueJSONLLine
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		rawBytes := append([]byte(nil), scanner.Bytes()...)
		line, err := parseIssueJSONLLine(rawBytes)
		if err != nil {
			return nil, fmt.Errorf("parse issues.jsonl line %d: %w", lineNum, err)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read issues jsonl: %w", err)
	}
	return lines, nil
}

// syncIssueMetadataToRaw writes issue.Metadata into raw["metadata"], starting from any
// existing metadata object in raw so unknown keys and non-string JSON values are kept.
// Keys jiraPendingComment, gitCommit, gitCommitUrl are removed from the stored metadata
// unless they are still present on issue.Metadata.
func syncIssueMetadataToRaw(raw map[string]json.RawMessage, issue BeadsIssue) error {
	meta := make(map[string]json.RawMessage)
	if mr, ok := raw["metadata"]; ok && len(mr) > 0 && string(bytes.TrimSpace(mr)) != "null" {
		if err := json.Unmarshal(mr, &meta); err != nil {
			return fmt.Errorf("metadata object: %w", err)
		}
	}
	trio := []string{"jiraPendingComment", "gitCommit", "gitCommitUrl"}
	for _, k := range trio {
		if issue.Metadata == nil {
			delete(meta, k)
			continue
		}
		if _, ok := issue.Metadata[k]; !ok {
			delete(meta, k)
		}
	}
	if issue.Metadata != nil {
		for k, v := range issue.Metadata {
			b, err := json.Marshal(v)
			if err != nil {
				return fmt.Errorf("metadata %q: %w", k, err)
			}
			meta[k] = b
		}
	}
	if len(meta) == 0 {
		delete(raw, "metadata")
		return nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	raw["metadata"] = b
	return nil
}

// SaveIssuesJSONLinesPreserve writes issues.jsonl from preserved lines, merging each line's
// Issue.Metadata into the raw JSON so native beads fields outside BeadsIssue are kept.
func SaveIssuesJSONLinesPreserve(path string, lines []IssueJSONLLine) error {
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create issues jsonl: %w", err)
	}

	abort := func(e error) error {
		_ = out.Close()
		_ = os.Remove(tmp)
		return e
	}

	for i := range lines {
		rawCopy := cloneRawMap(lines[i].Raw)
		if e := syncIssueMetadataToRaw(rawCopy, lines[i].Issue); e != nil {
			return abort(fmt.Errorf("issue %s: %w", lines[i].Issue.ID, e))
		}
		line, e := json.Marshal(rawCopy)
		if e != nil {
			return abort(fmt.Errorf("issue %s: %w", lines[i].Issue.ID, e))
		}
		if _, e := out.Write(append(line, '\n')); e != nil {
			return abort(fmt.Errorf("issue %s: %w", lines[i].Issue.ID, e))
		}
	}

	if err := out.Sync(); err != nil {
		return abort(fmt.Errorf("sync issues jsonl: %w", err))
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace issues jsonl: %w", err)
	}
	return nil
}
