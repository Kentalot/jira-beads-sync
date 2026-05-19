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

// Transition is a workflow transition from Jira REST API v2.
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		StatusCategory struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"statusCategory"`
	} `json:"to"`
}

type transitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

type transitionPostBody struct {
	Transition struct {
		ID string `json:"id"`
	} `json:"transition"`
}

// ListTransitions returns available workflow transitions for an issue.
func (c *Client) ListTransitions(issueKey string) ([]Transition, error) {
	if err := ValidateIssueKey(issueKey); err != nil {
		return nil, err
	}
	apiURL := fmt.Sprintf("%s/rest/api/2/issue/%s/transitions", c.baseURL, url.PathEscape(issueKey))

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list transitions: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read transitions: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list transitions for %s: status %d: %s", issueKey, resp.StatusCode, string(body))
	}

	var out transitionsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse transitions: %w", err)
	}
	return out.Transitions, nil
}

// DoTransition applies a workflow transition by id.
func (c *Client) DoTransition(issueKey, transitionID string) error {
	if err := ValidateIssueKey(issueKey); err != nil {
		return err
	}
	if transitionID == "" {
		return fmt.Errorf("transition id is empty")
	}
	apiURL := fmt.Sprintf("%s/rest/api/2/issue/%s/transitions", c.baseURL, url.PathEscape(issueKey))

	var body transitionPostBody
	body.Transition.ID = transitionID

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("transition issue: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("transition %s on %s: status %d: %s", transitionID, issueKey, resp.StatusCode, string(respBody))
	}
	return nil
}

// UpdateIssueFields sends a partial field update (REST API v2 PUT /issue/{key}).
func (c *Client) UpdateIssueFields(issueKey string, fields map[string]any) error {
	if err := ValidateIssueKey(issueKey); err != nil {
		return err
	}
	if len(fields) == 0 {
		return nil
	}
	apiURL := fmt.Sprintf("%s/rest/api/2/issue/%s", c.baseURL, url.PathEscape(issueKey))

	payload := map[string]any{"fields": fields}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, apiURL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("update issue: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update issue %s: status %d: %s", issueKey, resp.StatusCode, string(respBody))
	}
	return nil
}

// PriorityOption is an allowed priority value from editmeta.
type PriorityOption struct {
	ID   string
	Name string
}

// ListEditablePriorities returns allowed priority ids/names for an issue (from editmeta).
func (c *Client) ListEditablePriorities(issueKey string) ([]PriorityOption, error) {
	if err := ValidateIssueKey(issueKey); err != nil {
		return nil, err
	}
	apiURL := fmt.Sprintf("%s/rest/api/2/issue/%s/editmeta", c.baseURL, url.PathEscape(issueKey))

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("editmeta: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read editmeta: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("editmeta for %s: status %d: %s", issueKey, resp.StatusCode, string(body))
	}

	var meta struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("parse editmeta: %w", err)
	}

	rawPri, ok := meta.Fields["priority"]
	if !ok || len(rawPri) == 0 {
		return nil, nil
	}

	var priField struct {
		AllowedValues []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"allowedValues"`
	}
	if err := json.Unmarshal(rawPri, &priField); err != nil {
		return nil, fmt.Errorf("parse priority editmeta: %w", err)
	}

	out := make([]PriorityOption, 0, len(priField.AllowedValues))
	for _, v := range priField.AllowedValues {
		out = append(out, PriorityOption{ID: v.ID, Name: v.Name})
	}
	return out, nil
}

// SearchUserAccountID finds an accountId for assignee updates using /rest/api/3/user/search.
func (c *Client) SearchUserAccountID(query string) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", fmt.Errorf("user search query is empty")
	}
	apiURL := fmt.Sprintf("%s/rest/api/3/user/search?query=%s&maxResults=10",
		c.baseURL, url.QueryEscape(q))

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("user search: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read user search: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("user search: status %d: %s", resp.StatusCode, string(body))
	}

	var users []struct {
		AccountID    string `json:"accountId"`
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return "", fmt.Errorf("parse user search: %w", err)
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no Jira user found for query %q", query)
	}

	lower := strings.ToLower(q)
	if strings.Contains(lower, "@") {
		for _, u := range users {
			if strings.EqualFold(strings.TrimSpace(u.EmailAddress), q) {
				return u.AccountID, nil
			}
		}
	}
	return users[0].AccountID, nil
}
