package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/rigelstpierre/herdr-devin-board/internal/board"
	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
	"github.com/rigelstpierre/herdr-devin-board/internal/ui"
)

const ghParallelism = 4

func newLoader(credentialsPath string, gh *github.Client) func(context.Context) (ui.Data, error) {
	var client *devin.Client
	var self *devin.Self
	return func(ctx context.Context) (ui.Data, error) {
		if client == nil {
			creds, err := devin.LoadCredentials(credentialsPath, os.Getenv)
			if err != nil {
				return ui.Data{}, err
			}
			client = devin.NewClient(creds, &http.Client{Timeout: 20 * time.Second})
		}
		if self == nil {
			fetched, err := client.Self(ctx)
			if err != nil {
				return ui.Data{}, err
			}
			self = &fetched
		}
		sessions, err := client.ListSessions(ctx, *self)
		if err != nil {
			return ui.Data{}, err
		}
		return ui.Data{
			Sessions: sessions,
			Statuses: gh.Statuses(ctx, board.OpenPRURLs(sessions), ghParallelism),
		}, nil
	}
}
