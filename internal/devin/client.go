package devin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(creds Credentials, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(creds.APIURL, "/"), apiKey: creds.APIKey, http: httpClient}
}

type Self struct {
	UserID        string `json:"user_id"`
	SessionsOrgID string `json:"devin_sessions_org_id"`
}

type PullRequest struct {
	URL   string `json:"pr_url"`
	State string `json:"pr_state"`
}

type Session struct {
	ID           string        `json:"session_id"`
	URL          string        `json:"url"`
	Title        string        `json:"title"`
	Status       string        `json:"status"`
	StatusDetail string        `json:"status_detail"`
	UpdatedAt    int64         `json:"updated_at"`
	PullRequests []PullRequest `json:"pull_requests"`
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.StatusCode == http.StatusUnauthorized {
		return "Devin API 401 — run `devin auth login`"
	}
	return fmt.Sprintf("Devin API %d: %s", e.StatusCode, e.Body)
}

func (c *Client) Self(ctx context.Context) (Self, error) {
	var self Self
	if err := c.get(ctx, "/v3/self", nil, &self); err != nil {
		return Self{}, err
	}
	if self.UserID == "" || self.SessionsOrgID == "" {
		return Self{}, errors.New("Devin /v3/self response is missing user_id or devin_sessions_org_id")
	}
	return self, nil
}

func (c *Client) ListSessions(ctx context.Context, self Self) ([]Session, error) {
	path := "/v3/organizations/" + url.PathEscape(self.SessionsOrgID) + "/sessions"
	var sessions []Session
	cursor := ""
	for {
		query := url.Values{"user_ids": {self.UserID}, "is_archived": {"false"}, "first": {"200"}}
		if cursor != "" {
			query.Set("after", cursor)
		}
		var page struct {
			Items       []Session `json:"items"`
			EndCursor   string    `json:"end_cursor"`
			HasNextPage bool      `json:"has_next_page"`
		}
		if err := c.get(ctx, path, query, &page); err != nil {
			return nil, err
		}
		sessions = append(sessions, page.Items...)
		if !page.HasNextPage || page.EndCursor == "" || page.EndCursor == cursor {
			return sessions, nil
		}
		cursor = page.EndCursor
	}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Devin API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode Devin %s: %w", path, err)
	}
	return nil
}
