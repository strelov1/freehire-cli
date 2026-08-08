package selfupdate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.16.0", "v0.16.0", false},
		{"v0.16.0", "v0.15.9", false},
		{"v0.16.0", "v0.17.0", true},
		{"v0.16.0", "v1.0.0", true},
		{"v0.9.2", "v0.9.10", true}, // numeric compare, not lexical ("10" < "9" as strings)
		{"dev", "v0.1.0", true},     // an unstamped build never wins
		{"v0.16.0", "not-a-version", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func withFakeGitHubAPI(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	old := repoAPIURL
	repoAPIURL = srv.URL
	t.Cleanup(func() { repoAPIURL = old })
	return srv
}

func TestLatestReleaseParsesTagAndAssets(t *testing.T) {
	withFakeGitHubAPI(t, `{"tag_name":"v0.17.0","assets":[
		{"name":"freehire_darwin_arm64","browser_download_url":"https://example.test/freehire_darwin_arm64"},
		{"name":"freehire_linux_amd64","browser_download_url":"https://example.test/freehire_linux_amd64"}
	]}`, http.StatusOK)

	rel, err := LatestRelease(context.Background())
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if rel.Tag != "v0.17.0" {
		t.Errorf("Tag = %q, want v0.17.0", rel.Tag)
	}
	if rel.Assets["freehire_darwin_arm64"] != "https://example.test/freehire_darwin_arm64" {
		t.Errorf("asset URL = %q", rel.Assets["freehire_darwin_arm64"])
	}
}

func TestLatestReleaseSurfacesNon200(t *testing.T) {
	withFakeGitHubAPI(t, "", http.StatusNotFound)

	if _, err := LatestRelease(context.Background()); err == nil {
		t.Error("LatestRelease with a 404 should error")
	}
}

func TestAssetNameMatchesRuntime(t *testing.T) {
	want := fmt.Sprintf("freehire_%s_%s", runtime.GOOS, runtime.GOARCH)
	if got := AssetName(); got != want {
		t.Errorf("AssetName() = %q, want %q", got, want)
	}
}

func TestApplyReplacesTheBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new binary contents"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	execPath := filepath.Join(dir, "freehire")
	if err := os.WriteFile(execPath, []byte("old binary contents"), 0o755); err != nil {
		t.Fatalf("seed exec: %v", err)
	}

	if err := apply(context.Background(), execPath, srv.URL); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("read updated binary: %v", err)
	}
	if string(got) != "new binary contents" {
		t.Errorf("binary contents = %q, want %q", got, "new binary contents")
	}
	info, err := os.Stat(execPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("updated binary is not executable: mode %v", info.Mode())
	}
}

func TestApplySurfacesPermissionErrorWithHint(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission checks never fail")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new binary contents"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	execPath := filepath.Join(dir, "freehire")
	if err := os.WriteFile(execPath, []byte("old binary contents"), 0o755); err != nil {
		t.Fatalf("seed exec: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { // read+execute, no write
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) }) // let t.TempDir's own cleanup remove it

	err := apply(context.Background(), execPath, srv.URL)
	if err == nil {
		t.Fatal("apply into a read-only dir should error")
	}
	if !strings.Contains(err.Error(), "sudo") {
		t.Errorf("error = %q, want a sudo hint", err)
	}
}
