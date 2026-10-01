package devin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const sessionIDPrefix = "devin-"

var ErrNoArchiveKey = errors.New("archiving needs a Devin API key — see the README")

func LoadArchiveKey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoArchiveKey
	}
	if err != nil {
		return "", fmt.Errorf("read Devin API key: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", ErrNoArchiveKey
	}
	return key, nil
}

func (c *Client) Archive(ctx context.Context, orgID, sessionID string) error {
	if !strings.HasPrefix(sessionID, sessionIDPrefix) {
		sessionID = sessionIDPrefix + sessionID
	}
	path := "/v3/organizations/" + url.PathEscape(orgID) + "/sessions/" + url.PathEscape(sessionID) + "/archive"
	err := c.do(ctx, http.MethodPost, path, nil, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.IsAuth() {
		return &ArchiveAuthError{Err: apiErr}
	}
	return err
}

type ArchiveAuthError struct {
	Err *APIError
}

func (e *ArchiveAuthError) Error() string {
	return fmt.Sprintf("Devin API key can't archive (HTTP %d) — it needs a cog_ key (service user or PAT) with ManageOrgSessions; see the README", e.Err.StatusCode)
}

func (e *ArchiveAuthError) Unwrap() error {
	return e.Err
}
