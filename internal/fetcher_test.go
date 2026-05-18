package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestGitHubServer() *httptest.Server {
	mux := http.NewServeMux()
	var serverURL string

	mux.HandleFunc("/repos/test-org/test-repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v1.2.3",
			"assets": []map[string]interface{}{
				{"name": "app-darwin-arm64.tar.gz", "browser_download_url": serverURL + "/dl/app-darwin-arm64.tar.gz", "size": 5000000},
				{"name": "app-linux-amd64.tar.gz", "browser_download_url": serverURL + "/dl/app-linux-amd64.tar.gz", "size": 4800000},
				{"name": "checksums.txt", "browser_download_url": serverURL + "/dl/checksums.txt", "size": 256},
			},
		})
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc123def456  app-darwin-arm64.tar.gz\nfed789abc012  app-linux-amd64.tar.gz\n"))
	})
	mux.HandleFunc("/repos/no-release/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"Not Found"}`))
	})

	server := httptest.NewServer(mux)
	serverURL = server.URL
	return server
}

func newTestFetcher(githubURL, npmURL string) *Fetcher {
	f := NewFetcher("")
	f.githubBaseURL = githubURL
	f.npmBaseURL = npmURL
	return f
}

func TestFetchGitHubRelease(t *testing.T) {
	server := newTestGitHubServer()
	defer server.Close()

	f := newTestFetcher(server.URL, "")
	cv, err := f.fetchGitHubRelease(context.Background(), "test-org", "test-repo", "github:test-org/test-repo")
	if err != nil {
		t.Fatal(err)
	}
	if cv.LatestVersion != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %s", cv.LatestVersion)
	}
	if cv.Status != "ok" {
		t.Fatalf("expected status ok, got %s", cv.Status)
	}
	if cv.ReleaseMeta == nil {
		t.Fatal("expected release_meta")
	}
	if len(cv.ReleaseMeta.Assets) != 3 {
		t.Fatalf("expected 3 assets, got %d", len(cv.ReleaseMeta.Assets))
	}
	// Verify asset platform parsing
	for _, a := range cv.ReleaseMeta.Assets {
		switch a.Name {
		case "app-darwin-arm64.tar.gz":
			if a.OS != "darwin" || a.Arch != "arm64" || a.Kind != "archive" {
				t.Errorf("darwin-arm64: got os=%s arch=%s kind=%s", a.OS, a.Arch, a.Kind)
			}
		case "app-linux-amd64.tar.gz":
			if a.OS != "linux" || a.Arch != "amd64" || a.Kind != "archive" {
				t.Errorf("linux-amd64: got os=%s arch=%s kind=%s", a.OS, a.Arch, a.Kind)
			}
		case "checksums.txt":
			if a.Kind != "checksum" {
				t.Errorf("checksums.txt: got kind=%s", a.Kind)
			}
		}
	}
	// Verify checksums were downloaded and parsed
	if len(cv.ReleaseMeta.Checksums) != 2 {
		t.Fatalf("expected 2 checksums, got %d", len(cv.ReleaseMeta.Checksums))
	}
	if cv.ReleaseMeta.Checksums["app-darwin-arm64.tar.gz"] != "sha256:abc123def456" {
		t.Fatalf("wrong checksum: %s", cv.ReleaseMeta.Checksums["app-darwin-arm64.tar.gz"])
	}
}

func TestFetchGitHubRelease404(t *testing.T) {
	server := newTestGitHubServer()
	defer server.Close()

	f := newTestFetcher(server.URL, "")
	_, err := f.fetchGitHubRelease(context.Background(), "no-release", "repo", "github:no-release/repo")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestFetchNpmLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openclaw/latest" {
			json.NewEncoder(w).Encode(map[string]string{"version": "2026.5.1"})
			return
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	f := newTestFetcher("", server.URL)
	cv, err := f.fetchNpmLatest(context.Background(), "openclaw", "npm:openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if cv.LatestVersion != "2026.5.1" {
		t.Fatalf("expected 2026.5.1, got %s", cv.LatestVersion)
	}
	if cv.Status != "ok" {
		t.Fatalf("expected status ok, got %s", cv.Status)
	}
	if cv.ReleaseMeta != nil {
		t.Fatal("npm should not have release_meta")
	}
}

func TestFetchNpmLatest404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	f := newTestFetcher("", server.URL)
	_, err := f.fetchNpmLatest(context.Background(), "nonexistent", "npm:nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestFetchDispatch(t *testing.T) {
	githubServer := newTestGitHubServer()
	defer githubServer.Close()
	npmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "1.0.0"})
	}))
	defer npmServer.Close()

	f := newTestFetcher(githubServer.URL, npmServer.URL)

	// Test github dispatch
	cv, err := f.Fetch(context.Background(), ComponentDef{Name: "test", Source: "github:test-org/test-repo"})
	if err != nil {
		t.Fatal(err)
	}
	if cv.LatestVersion != "1.2.3" {
		t.Fatalf("github dispatch: expected 1.2.3, got %s", cv.LatestVersion)
	}

	// Test npm dispatch
	cv, err = f.Fetch(context.Background(), ComponentDef{Name: "test", Source: "npm:something"})
	if err != nil {
		t.Fatal(err)
	}
	if cv.LatestVersion != "1.0.0" {
		t.Fatalf("npm dispatch: expected 1.0.0, got %s", cv.LatestVersion)
	}

	// Test unknown source
	_, err = f.Fetch(context.Background(), ComponentDef{Name: "test", Source: "pypi:something"})
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestParseChecksums(t *testing.T) {
	content := `abc123def456  octo-daemon-darwin-arm64.tar.gz
fed789abc012  octo-daemon-linux-amd64.tar.gz
111222333444  octo-daemon-windows-amd64.zip
`
	result := ParseChecksums(content)
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}
	if result["octo-daemon-darwin-arm64.tar.gz"] != "sha256:abc123def456" {
		t.Fatalf("wrong checksum: %s", result["octo-daemon-darwin-arm64.tar.gz"])
	}
}

func TestParseChecksumsEmpty(t *testing.T) {
	if len(ParseChecksums("")) != 0 {
		t.Fatal("expected 0 entries")
	}
}

func TestParseAssetPlatform(t *testing.T) {
	tests := []struct {
		name         string
		expectedOS   string
		expectedArch string
		expectedKind string
	}{
		{"octo-daemon-darwin-arm64.tar.gz", "darwin", "arm64", "archive"},
		{"octo-daemon-linux-amd64.tar.gz", "linux", "amd64", "archive"},
		{"octo-daemon-windows-amd64.zip", "windows", "amd64", "archive"},
		{"checksums.txt", "", "", "checksum"},
		{"SHASUMS256.txt", "", "", "checksum"},
		{"SHA256SUMS", "", "", "checksum"},
		{"sha512sum.txt", "", "", "checksum"},
		{"app-darwin-aarch64.dmg", "darwin", "arm64", "installer"},
		{"config.yml", "", "", "manifest"},
		{"release.sig", "", "", "signature"},
		{"app-linux-x86_64.tar.gz", "linux", "amd64", "archive"},
		{"app-macos-arm64.pkg", "darwin", "arm64", "installer"},
		{"README.md", "", "", "other"},
	}
	for _, tc := range tests {
		os, arch, kind := parseAssetPlatform(tc.name)
		if os != tc.expectedOS || arch != tc.expectedArch || kind != tc.expectedKind {
			t.Errorf("parseAssetPlatform(%q) = (%q,%q,%q), want (%q,%q,%q)",
				tc.name, os, arch, kind, tc.expectedOS, tc.expectedArch, tc.expectedKind)
		}
	}
}

func TestVersionRe(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"v0.3.0", "0.3.0"},
		{"0.3.0", "0.3.0"},
		{"rust-v0.128.0", "0.128.0"},
		{"release-1.2.3", "1.2.3"},
		{"2026.4.30", "2026.4.30"},
		{"v2.1.132-beta", "2.1.132"},
		{"unknown", ""},
	}
	for _, tc := range tests {
		got := versionRe.FindString(tc.tag)
		if got != tc.want {
			t.Errorf("versionRe.FindString(%q) = %q, want %q", tc.tag, got, tc.want)
		}
	}
}
