package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
	"github.com/rigelstpierre/herdr-devin-board/internal/ui"
)

const (
	ghParallelism = 4
	httpTimeout   = 20 * time.Second
)

type service struct {
	credentialsPath string
	archiveKeyPath  string
	gh              *github.Client

	mu     sync.Mutex
	client *devin.Client
	self   *devin.Self
}

func newService(credentialsPath, archiveKeyPath string, gh *github.Client) *service {
	return &service{credentialsPath: credentialsPath, archiveKeyPath: archiveKeyPath, gh: gh}
}

func (s *service) Load(ctx context.Context) (ui.Data, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureSelf(ctx); err != nil {
		return ui.Data{}, err
	}
	sessions, err := s.client.ListSessions(ctx, *s.self)
	if err != nil {
		return ui.Data{}, s.forgetOnAuthFailure(err)
	}
	return ui.Data{
		Sessions: sessions,
		Statuses: s.gh.Statuses(ctx, board.OpenPRURLs(sessions), ghParallelism),
	}, nil
}

func (s *service) Archive(ctx context.Context, sessionID string) error {
	key, err := devin.LoadArchiveKey(s.archiveKeyPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.ensureSelf(ctx)
	var orgID, apiURL string
	if err == nil {
		orgID, apiURL = s.self.SessionsOrgID, s.client.BaseURL()
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	archiver := devin.NewClient(devin.Credentials{APIKey: key, APIURL: apiURL}, &http.Client{Timeout: httpTimeout})
	return archiver.Archive(ctx, orgID, sessionID)
}

func (s *service) ensureSelf(ctx context.Context) error {
	if s.client == nil {
		creds, err := devin.LoadCredentials(s.credentialsPath, os.Getenv)
		if err != nil {
			return err
		}
		s.client = devin.NewClient(creds, &http.Client{Timeout: httpTimeout})
	}
	if s.self == nil {
		fetched, err := s.client.Self(ctx)
		if err != nil {
			return s.forgetOnAuthFailure(err)
		}
		s.self = &fetched
	}
	return nil
}

func (s *service) forgetOnAuthFailure(err error) error {
	var apiErr *devin.APIError
	if errors.As(err, &apiErr) && apiErr.IsAuth() {
		s.client, s.self = nil, nil
	}
	return err
}
