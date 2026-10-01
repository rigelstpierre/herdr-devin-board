package devin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
)

func TestArchivePostsToPrefixedSessionPath(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotAuth = r.URL.Path, r.Method, r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := devin.NewClient(devin.Credentials{APIKey: "apk", APIURL: srv.URL}, srv.Client())

	if err := client.Archive(context.Background(), "org-s", "abc123"); err != nil {
		t.Fatal(err)
	}

	if gotMethod != http.MethodPost || gotPath != "/v3/organizations/org-s/sessions/devin-abc123/archive" || gotAuth != "Bearer apk" {
		t.Fatalf("got %s %s auth=%q", gotMethod, gotPath, gotAuth)
	}
}

func TestArchiveKeepsAnExistingPrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer srv.Close()
	client := devin.NewClient(devin.Credentials{APIKey: "apk", APIURL: srv.URL}, srv.Client())

	if err := client.Archive(context.Background(), "o", "devin-abc"); err != nil {
		t.Fatal(err)
	}

	if gotPath != "/v3/organizations/o/sessions/devin-abc/archive" {
		t.Fatalf("path %q", gotPath)
	}
}

func TestArchiveSurfacesAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	client := devin.NewClient(devin.Credentials{APIKey: "apk", APIURL: srv.URL}, srv.Client())

	err := client.Archive(context.Background(), "o", "abc")

	var apiErr *devin.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("got %v", err)
	}
}

func TestLoadArchiveKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api_key")

	if _, err := devin.LoadArchiveKey(path); !errors.Is(err, devin.ErrNoArchiveKey) {
		t.Fatalf("missing file: got %v", err)
	}
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := devin.LoadArchiveKey(path); !errors.Is(err, devin.ErrNoArchiveKey) {
		t.Fatalf("blank file: got %v", err)
	}
	if err := os.WriteFile(path, []byte("apk_123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := devin.LoadArchiveKey(path)
	if err != nil || key != "apk_123" {
		t.Fatalf("got %q, %v", key, err)
	}
}
