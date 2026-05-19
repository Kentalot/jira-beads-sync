package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// AddIssueComment posts a plain-text comment on the issue.
// It tries REST API v2 (wiki-style string body) first, then v3 with minimal ADF if v2 fails.
func (c *Client) AddIssueComment(issueKey, plainText string) error {
	if err := ValidateIssueKey(issueKey); err != nil {
		return err
	}
	plainText = strings.TrimSpace(plainText)
	if plainText == "" {
		return fmt.Errorf("comment body is empty")
	}

	v2URL := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", c.baseURL, url.PathEscape(issueKey))
	v2Payload := map[string]any{"body": plainText}
	raw, err := json.Marshal(v2Payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, v2URL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("create comment request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}

	// Retry with v3 + ADF (common on Jira Cloud when v2 rejects plain body)
	v3URL := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", c.baseURL, url.PathEscape(issueKey))
	v3Payload := map[string]any{
		"body": map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []any{
				map[string]any{
					"type":    "paragraph",
					"content": []any{map[string]any{"type": "text", "text": plainText}},
				},
			},
		},
	}
	raw3, err := json.Marshal(v3Payload)
	if err != nil {
		return err
	}
	req3, err := http.NewRequest(http.MethodPost, v3URL, bytes.NewReader(raw3))
	if err != nil {
		return fmt.Errorf("create v3 comment request: %w", err)
	}
	c.setAuthHeader(req3)
	req3.Header.Set("Accept", "application/json")
	req3.Header.Set("Content-Type", "application/json")

	resp3, err := c.httpClient.Do(req3)
	if err != nil {
		return fmt.Errorf("post v3 comment: %w", err)
	}
	body3, _ := io.ReadAll(resp3.Body)
	_ = resp3.Body.Close()
	if resp3.StatusCode == http.StatusCreated || resp3.StatusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf("add comment on %s: v2 status %d: %s; v3 status %d: %s",
		issueKey, resp.StatusCode, string(body), resp3.StatusCode, string(body3))
}
