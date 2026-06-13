// Package config resolves and persists the freehire CLI credentials: the API
// token and base URL. The token is stored in ~/.freehire/creds.json (mode 0600)
// and may be overridden by environment variables.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	// DefaultAPIURL is the production API base when none is configured.
	DefaultAPIURL = "https://freehire.dev"

	// EnvToken / EnvAPIURL override the stored credentials.
	EnvToken  = "FREEHIRE_TOKEN"
	EnvAPIURL = "FREEHIRE_API_URL"

	dirName  = ".freehire"
	fileName = "creds.json"
)

// ErrNoToken is returned by Resolve when no token is configured anywhere.
var ErrNoToken = errors.New("not authenticated: run `freehire auth login`")

// Creds is the persisted credential file (~/.freehire/creds.json).
type Creds struct {
	Token  string `json:"token"`
	APIURL string `json:"api_url"`
}

// Resolved is the effective token and API URL after applying precedence.
type Resolved struct {
	Token  string
	APIURL string
}

// Path returns the credentials file path (~/.freehire/creds.json).
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dirName, fileName), nil
}

// Load reads the credentials file. A missing file yields the zero Creds and no
// error, so a first run is not an error.
func Load() (Creds, error) {
	p, err := Path()
	if err != nil {
		return Creds{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Creds{}, nil
	}
	if err != nil {
		return Creds{}, err
	}
	var c Creds
	if err := json.Unmarshal(b, &c); err != nil {
		return Creds{}, err
	}
	return c, nil
}

// Save writes the credentials file with owner-only permissions (file 0600,
// directory 0700), since it holds a secret token.
func Save(c Creds) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// Remove deletes the credentials file; a missing file is not an error.
func Remove() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Resolve computes the effective token and API URL with precedence
// env > creds file > default. getenv is injected for testability (pass
// os.Getenv). The token is required; the URL falls back to DefaultAPIURL.
func Resolve(getenv func(string) string) (Resolved, error) {
	c, err := Load()
	if err != nil {
		return Resolved{}, err
	}
	token := firstNonEmpty(getenv(EnvToken), c.Token)
	if token == "" {
		return Resolved{}, ErrNoToken
	}
	apiURL := firstNonEmpty(getenv(EnvAPIURL), c.APIURL, DefaultAPIURL)
	return Resolved{Token: token, APIURL: apiURL}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
