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
	return c.do(ctx, http.MethodPost, path, nil, nil)
}
