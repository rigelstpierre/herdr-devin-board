package devin_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/devin"
)

func noEnv(string) string { return "" }

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadCredentialsReadsCLIFile(t *testing.T) {
	path := writeFile(t, "windsurf_api_key = \"key-1\"\ndevin_api_url = \"https://api.example.test\"\napi_server_url = \"ignored\"\n")

	creds, err := devin.LoadCredentials(path, noEnv)

	if err != nil {
		t.Fatal(err)
	}
	if creds.APIKey != "key-1" || creds.APIURL != "https://api.example.test" {
		t.Fatalf("got %+v", creds)
	}
}

func TestLoadCredentialsDefaultsAPIURL(t *testing.T) {
	path := writeFile(t, "windsurf_api_key = \"key-1\"\n")

	creds, err := devin.LoadCredentials(path, noEnv)

	if err != nil {
		t.Fatal(err)
	}
	if creds.APIURL != "https://api.devin.ai" {
		t.Fatalf("got %q", creds.APIURL)
	}
}

func TestLoadCredentialsEnvOverridesFile(t *testing.T) {
	env := map[string]string{"DEVIN_API_KEY": "env-key", "DEVIN_API_URL": "https://env.test"}

	creds, err := devin.LoadCredentials("/does/not/exist", func(k string) string { return env[k] })

	if err != nil {
		t.Fatal(err)
	}
	if creds.APIKey != "env-key" || creds.APIURL != "https://env.test" {
		t.Fatalf("got %+v", creds)
	}
}

func TestLoadCredentialsMissingFile(t *testing.T) {
	_, err := devin.LoadCredentials(filepath.Join(t.TempDir(), "nope.toml"), noEnv)

	if !errors.Is(err, devin.ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadCredentialsEmptyKey(t *testing.T) {
	path := writeFile(t, "devin_api_url = \"https://api.devin.ai\"\n")

	_, err := devin.LoadCredentials(path, noEnv)

	if !errors.Is(err, devin.ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}
