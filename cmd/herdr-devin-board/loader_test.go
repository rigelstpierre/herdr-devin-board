package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
	"github.com/rigelstpierre/herdr-devin-board/internal/github"
)

func TestLoaderFetchesSelfOnceAndEnrichesOpenPRs(t *testing.T) {
	selfCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/self" {
			selfCalls++
			fmt.Fprint(w, `{"user_id":"u","devin_sessions_org_id":"o"}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"session_id":"s","status":"running","updated_at":1,"pull_requests":[{"pr_url":"https://github.com/o/r/pull/1","pr_state":"open"}]}],"has_next_page":false}`)
	}))
	defer srv.Close()
	t.Setenv("DEVIN_API_KEY", "k")
	t.Setenv("DEVIN_API_URL", srv.URL)
	gh := github.NewClient(func(context.Context, ...string) ([]byte, error) {
		return []byte(`{"number":1,"state":"OPEN","statusCheckRollup":[]}`), nil
	})
	load := newLoader("/unused", gh)

	for range 2 {
		data, err := load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(data.Sessions) != 1 || data.Statuses["https://github.com/o/r/pull/1"].Number != 1 {
			t.Fatalf("data %+v", data)
		}
	}
	if selfCalls != 1 {
		t.Fatalf("self calls = %d", selfCalls)
	}
}

func TestLoaderReportsMissingCredentialsAndRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	load := newLoader(path, github.NewClient(nil))

	if _, err := load(context.Background()); !errors.Is(err, devin.ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}

	if err := os.WriteFile(path, []byte("windsurf_api_key = \"k\"\ndevin_api_url = \"http://127.0.0.1:1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(context.Background()); errors.Is(err, devin.ErrNoCredentials) {
		t.Fatal("loader should re-read credentials after they appear")
	}
}

func TestLoaderRereadsCredentialsAfterAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/v3/self" {
			fmt.Fprint(w, `{"user_id":"u","devin_sessions_org_id":"o"}`)
			return
		}
		fmt.Fprint(w, `{"items":[],"has_next_page":false}`)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "credentials.toml")
	writeCreds := func(key string) {
		body := fmt.Sprintf("windsurf_api_key = %q\ndevin_api_url = %q\n", key, srv.URL)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	load := newLoader(path, github.NewClient(nil))

	writeCreds("stale")
	if _, err := load(context.Background()); err == nil {
		t.Fatal("want auth error with stale key")
	}
	writeCreds("good")
	if _, err := load(context.Background()); err != nil {
		t.Fatalf("loader kept the stale key: %v", err)
	}
}
