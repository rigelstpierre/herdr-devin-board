package devin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const defaultAPIURL = "https://api.devin.ai"

var ErrNoCredentials = errors.New("no Devin credentials found — run `devin auth login`")

type Credentials struct {
	APIKey string
	APIURL string
}

func DefaultCredentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "devin", "credentials.toml"), nil
}

func LoadCredentials(path string, getenv func(string) string) (Credentials, error) {
	if key := getenv("DEVIN_API_KEY"); key != "" {
		return Credentials{APIKey: key, APIURL: orDefault(getenv("DEVIN_API_URL"), defaultAPIURL)}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrNoCredentials
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("read Devin credentials: %w", err)
	}
	var file struct {
		APIKey string `toml:"windsurf_api_key"`
		APIURL string `toml:"devin_api_url"`
	}
	if err := toml.Unmarshal(data, &file); err != nil {
		return Credentials{}, fmt.Errorf("parse Devin credentials: %w", err)
	}
	if file.APIKey == "" {
		return Credentials{}, ErrNoCredentials
	}
	return Credentials{APIKey: file.APIKey, APIURL: orDefault(file.APIURL, defaultAPIURL)}, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
