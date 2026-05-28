package jira

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type jiraUserRecord struct {
	AccountID    string `json:"accountId"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

// ResolveAssignee returns a Jira "assignee" field value for UpdateIssueFields.
// Jira Cloud uses accountId; Jira Server/Data Center uses name (username key).
func (c *Client) ResolveAssignee(issueKey, query string) (map[string]any, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("assignee query is empty")
	}

	if accountID, err := c.searchUserAccountIDV3(q); err == nil && accountID != "" {
		return map[string]any{"accountId": accountID}, nil
	}

	name, err := c.searchUserNameV2(issueKey, q)
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": name}, nil
}

// SearchUserAccountID finds an accountId using /rest/api/3/user/search (Jira Cloud).
func (c *Client) SearchUserAccountID(query string) (string, error) {
	return c.searchUserAccountIDV3(query)
}

func (c *Client) searchUserAccountIDV3(query string) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", fmt.Errorf("user search query is empty")
	}
	apiURL := fmt.Sprintf("%s/rest/api/3/user/search?query=%s&maxResults=10",
		c.baseURL, url.QueryEscape(q))

	body, status, err := c.getJSON(apiURL)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("user search: status %d", status)
	}
	if len(body) > 0 && body[0] == '<' {
		return "", fmt.Errorf("user search: non-JSON response")
	}

	var users []jiraUserRecord
	if err := json.Unmarshal(body, &users); err != nil {
		return "", fmt.Errorf("parse user search: %w", err)
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no Jira user found for query %q", query)
	}

	if strings.Contains(strings.ToLower(q), "@") {
		for _, u := range users {
			if strings.EqualFold(strings.TrimSpace(u.EmailAddress), q) {
				if u.AccountID != "" {
					return u.AccountID, nil
				}
			}
		}
	}
	if users[0].AccountID != "" {
		return users[0].AccountID, nil
	}
	return "", fmt.Errorf("no accountId in user search results for %q", query)
}

func (c *Client) searchUserNameV2(issueKey, query string) (string, error) {
	users, err := c.searchUsersV2(issueKey, query)
	if err != nil {
		return "", err
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no Jira user found for query %q", query)
	}

	chosen := pickUserRecord(users, query)
	name := chosen.Name
	if name == "" {
		name = chosen.Key
	}
	if name == "" {
		return "", fmt.Errorf("no assignable username for query %q", query)
	}
	return name, nil
}

func (c *Client) searchUsersV2(issueKey, query string) ([]jiraUserRecord, error) {
	q := strings.TrimSpace(query)
	candidates := uniqueNonEmptyStrings(assigneeSearchCandidates(q))

	var merged []jiraUserRecord
	seen := make(map[string]bool)

	addUsers := func(users []jiraUserRecord) {
		for _, u := range users {
			id := u.Key
			if id == "" {
				id = u.Name
			}
			if id == "" {
				id = u.EmailAddress
			}
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			merged = append(merged, u)
		}
	}

	for _, term := range candidates {
		if term == "" {
			continue
		}
		if issueKey != "" {
			if users, err := c.fetchV2UserList(
				fmt.Sprintf("%s/rest/api/2/user/assignable/search?issueKey=%s&username=%s",
					c.baseURL, url.PathEscape(issueKey), url.QueryEscape(term)),
			); err == nil && len(users) > 0 {
				addUsers(users)
			}
		}
		if users, err := c.fetchV2UserList(
			fmt.Sprintf("%s/rest/api/2/user/search?username=%s", c.baseURL, url.QueryEscape(term)),
		); err == nil && len(users) > 0 {
			addUsers(users)
		}
	}

	return merged, nil
}

func (c *Client) fetchV2UserList(apiURL string) ([]jiraUserRecord, error) {
	body, status, err := c.getJSON(apiURL)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("status %d", status)
	}
	if len(body) > 0 && body[0] == '<' {
		return nil, fmt.Errorf("non-JSON response")
	}
	var users []jiraUserRecord
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("parse user list: %w", err)
	}
	return users, nil
}

func pickUserRecord(users []jiraUserRecord, query string) jiraUserRecord {
	q := strings.TrimSpace(query)

	for _, u := range users {
		if q != "" && strings.EqualFold(strings.TrimSpace(u.EmailAddress), q) {
			return u
		}
	}
	for _, u := range users {
		if q != "" && strings.EqualFold(strings.TrimSpace(u.Name), q) {
			return u
		}
	}
	for _, u := range users {
		if q != "" && strings.EqualFold(strings.TrimSpace(u.Key), q) {
			return u
		}
	}
	for _, u := range users {
		if q != "" && strings.EqualFold(strings.TrimSpace(u.DisplayName), q) {
			return u
		}
	}
	return users[0]
}

func (c *Client) getJSON(apiURL string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func assigneeSearchCandidates(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	var out []string
	if at := strings.Index(q, "@"); at > 0 {
		out = append(out, q[:at])
	}
	out = append(out, q)
	if fields := strings.Fields(q); len(fields) > 1 {
		out = append(out, fields[0])
	}
	return out
}

func uniqueNonEmptyStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
