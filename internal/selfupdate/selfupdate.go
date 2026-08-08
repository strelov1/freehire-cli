// Package selfupdate checks GitHub for newer freehire-cli releases and
// installs them over the running binary. It talks to the GitHub REST API
// directly, not the freehire API, so it has no dependency on internal/client.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// repoAPIURL is the GitHub API endpoint for this repo's latest release.
// A var, not a const, so tests can point it at an httptest.Server.
var repoAPIURL = "https://api.github.com/repos/strelov1/freehire-cli/releases/latest"

// Release is a GitHub release: its tag, and its assets by filename.
type Release struct {
	Tag    string
	Assets map[string]string // asset filename -> download URL
}

// ghAsset and ghRelease decode the subset of the GitHub releases API
// response this package needs.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// LatestRelease fetches this repo's latest GitHub release.
func LatestRelease(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, repoAPIURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("checking latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("checking latest release: unexpected status %d", resp.StatusCode)
	}

	var gh ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return Release{}, fmt.Errorf("checking latest release: %w", err)
	}

	assets := make(map[string]string, len(gh.Assets))
	for _, a := range gh.Assets {
		assets[a.Name] = a.BrowserDownloadURL
	}
	return Release{Tag: gh.TagName, Assets: assets}, nil
}

// IsNewer reports whether latest is a newer version than current. Both are
// read as vMAJOR.MINOR.PATCH (a leading "v" is stripped from each) and
// compared numerically component by component — a plain string compare would
// rank "v0.9.10" below "v0.9.2". A non-numeric or missing component reads as
// 0, so a malformed tag never outranks a well-formed one.
func IsNewer(current, latest string) bool {
	c := versionParts(current)
	l := versionParts(latest)
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func versionParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	fields := strings.SplitN(v, ".", 3)
	var parts [3]int
	for i := 0; i < len(fields) && i < 3; i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			continue // leaves parts[i] at its zero value
		}
		parts[i] = n
	}
	return parts
}
