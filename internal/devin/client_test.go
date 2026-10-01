package devin_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
)

func newClient(srv *httptest.Server) *devin.Client {
	return devin.NewClient(devin.Credentials{APIKey: "key-123", APIURL: srv.URL}, srv.Client())
}

func TestSelf(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/self" {
			t.Errorf("path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key-123" {
			t.Errorf("auth %q", got)
		}
		fmt.Fprint(w, `{"user_id":"user-1","org_id":"org-other","devin_sessions_org_id":"org-s"}`)
	}))
	defer srv.Close()

	self, err := newClient(srv).Self(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if self.UserID != "user-1" || self.SessionsOrgID != "org-s" {
		t.Fatalf("got %+v", self)
	}
}

func TestListSessionsFiltersToSelfAndPaginates(t *testing.T) {
	var calls []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/organizations/org-s/sessions" {
			t.Errorf("path %q", r.URL.Path)
		}
		q := r.URL.Query()
		calls = append(calls, q)
		if q.Get("after") == "" {
			fmt.Fprint(w, `{"items":[{"session_id":"a","url":"https://devin.test/sessions/a","title":"A","status":"running","status_detail":"working","updated_at":100,"pull_requests":[]}],"end_cursor":"c1","has_next_page":true}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"session_id":"b","status":"exit","updated_at":50,"pull_requests":[{"pr_url":"https://github.com/o/r/pull/1","pr_state":"merged"}]}],"end_cursor":"c2","has_next_page":false}`)
	}))
	defer srv.Close()

	sessions, err := newClient(srv).ListSessions(context.Background(), devin.Self{UserID: "user-1", SessionsOrgID: "org-s"})

	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].ID != "a" || sessions[1].ID != "b" {
		t.Fatalf("got %+v", sessions)
	}
	if sessions[0].Title != "A" || sessions[0].StatusDetail != "working" || sessions[0].UpdatedAt != 100 {
		t.Fatalf("fields not decoded: %+v", sessions[0])
	}
	if pr := sessions[1].PullRequests; len(pr) != 1 || pr[0].URL != "https://github.com/o/r/pull/1" || pr[0].State != "merged" {
		t.Fatalf("prs: %+v", pr)
	}
	first := calls[0]
	if first.Get("user_ids") != "user-1" || first.Get("is_archived") != "false" || first.Get("first") != "200" {
		t.Fatalf("first query: %v", first)
	}
	if len(calls) != 2 || calls[1].Get("after") != "c1" {
		t.Fatalf("calls: %v", calls)
	}
}

func TestListSessionsStopsOnRepeatedCursor(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"items":[],"end_cursor":"same","has_next_page":true}`)
	}))
	defer srv.Close()

	_, err := newClient(srv).ListSessions(context.Background(), devin.Self{UserID: "u", SessionsOrgID: "o"})

	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestUnauthorizedTellsUserToLogIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()

	_, err := newClient(srv).Self(context.Background())

	var apiErr *devin.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "devin auth login") {
		t.Fatalf("message %q", err.Error())
	}
}

func TestSelfRejectsIncompleteIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"user_id":"user-1"}`)
	}))
	defer srv.Close()

	_, err := newClient(srv).Self(context.Background())

	if err == nil || !strings.Contains(err.Error(), "devin_sessions_org_id") {
		t.Fatalf("got %v", err)
	}
}

func TestDecodeErrorNamesTheEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()

	_, err := newClient(srv).Self(context.Background())

	if err == nil || !strings.Contains(err.Error(), "/v3/self") {
		t.Fatalf("got %v", err)
	}
}
