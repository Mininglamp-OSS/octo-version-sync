package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// versionRe extracts semver-like X.Y.Z from tag strings such as
// "v0.3.0", "rust-v0.128.0", "release-1.2.3", "2026.4.30".
var versionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

type Fetcher struct {
	httpClient    *http.Client
	githubToken   string
	githubBaseURL string
	npmBaseURL    string
}

func NewFetcher(githubToken string) *Fetcher {
	return &Fetcher{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		githubToken:   githubToken,
		githubBaseURL: "https://api.github.com",
		npmBaseURL:    "https://registry.npmjs.org",
	}
}

func (f *Fetcher) Fetch(ctx context.Context, comp ComponentDef) (*ComponentVersion, error) {
	if strings.HasPrefix(comp.Source, "github:") {
		parts := strings.SplitN(strings.TrimPrefix(comp.Source, "github:"), "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid github source: %s", comp.Source)
		}
		return f.fetchGitHubRelease(ctx, parts[0], parts[1], comp.Source)
	}
	if strings.HasPrefix(comp.Source, "npm:") {
		pkg := strings.TrimPrefix(comp.Source, "npm:")
		return f.fetchNpmLatest(ctx, pkg, comp.Source)
	}
	return nil, fmt.Errorf("unknown source type: %s", comp.Source)
}

func (f *Fetcher) fetchGitHubRelease(ctx context.Context, owner, repo, source string) (*ComponentVersion, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", f.githubBaseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if f.githubToken != "" {
		req.Header.Set("Authorization", "token "+f.githubToken)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github returned %d: %s", resp.StatusCode, string(body))
	}

	var release struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode github response: %w", err)
	}

	// 优先从 release name 提取版本号（name 通常比 tag 干净，
	// 例如 hermes: tag=v2026.4.30, name="Hermes Agent v0.12.0 (2026.4.30)" → 0.12.0；
	// codex: tag=rust-v0.128.0, name="0.128.0" → 0.128.0），
	// name 里找不到再回退到 tag。
	version := strings.TrimPrefix(release.TagName, "v")
	if m := versionRe.FindString(release.Name); m != "" {
		version = m
	} else if m := versionRe.FindString(release.TagName); m != "" {
		version = m
	}

	assets := make([]ReleaseAsset, 0, len(release.Assets))
	var checksumsURL string
	for _, a := range release.Assets {
		asset := ReleaseAsset{
			Name: a.Name,
			URL:  a.BrowserDownloadURL,
			Size: a.Size,
		}
		asset.OS, asset.Arch, asset.Kind = parseAssetPlatform(a.Name)
		assets = append(assets, asset)
		if isChecksumFile(a.Name) {
			checksumsURL = a.BrowserDownloadURL
		}
	}

	checksums := map[string]string{}
	if checksumsURL != "" {
		var checksumErr error
		checksums, checksumErr = f.fetchChecksums(ctx, checksumsURL)
		if checksumErr != nil {
			log.Printf("[WARN] %s/%s: checksums.txt download failed: %v", owner, repo, checksumErr)
		}
	}

	now := nowBeijing()
	return &ComponentVersion{
		LatestVersion: version,
		ReleaseMeta: &ReleaseMeta{
			Tag:       release.TagName,
			Assets:    assets,
			Checksums: checksums,
		},
		FetchedAt: now,
		Source:    source,
		Status:    "ok",
	}, nil
}

func (f *Fetcher) fetchChecksums(ctx context.Context, url string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if f.githubToken != "" {
		req.Header.Set("Authorization", "token "+f.githubToken)
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("checksums returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return ParseChecksums(string(body)), nil
}

func ParseChecksums(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 2 {
			result[parts[1]] = "sha256:" + parts[0]
		}
	}
	return result
}

func (f *Fetcher) fetchNpmLatest(ctx context.Context, pkg, source string) (*ComponentVersion, error) {
	url := fmt.Sprintf("%s/%s/latest", f.npmBaseURL, pkg)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("npm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("npm returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode npm response: %w", err)
	}

	now := nowBeijing()
	return &ComponentVersion{
		LatestVersion: result.Version,
		FetchedAt:     now,
		Source:        source,
		Status:        "ok",
	}, nil
}

func isChecksumFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "checksums") ||
		strings.Contains(lower, "checksum") ||
		strings.Contains(lower, "shasums") ||
		strings.Contains(lower, "sha256sum") ||
		strings.Contains(lower, "sha512sum")
}

func parseAssetPlatform(name string) (os, arch, kind string) {
	lower := strings.ToLower(name)
	ext := filepath.Ext(lower)

	switch {
	case isChecksumFile(name):
		kind = "checksum"
	case ext == ".yml" || ext == ".yaml":
		kind = "manifest"
	case ext == ".sig" || ext == ".asc":
		kind = "signature"
	case strings.HasSuffix(lower, ".tar.gz") || ext == ".zip" || ext == ".tgz":
		kind = "archive"
	case ext == ".msi" || ext == ".pkg" || ext == ".deb" || ext == ".rpm" || ext == ".dmg":
		kind = "installer"
	default:
		kind = "other"
	}

	switch {
	case strings.Contains(lower, "darwin") || strings.Contains(lower, "macos"):
		os = "darwin"
	case strings.Contains(lower, "linux"):
		os = "linux"
	case strings.Contains(lower, "windows") || strings.Contains(lower, "win"):
		os = "windows"
	}

	switch {
	case strings.Contains(lower, "arm64") || strings.Contains(lower, "aarch64"):
		arch = "arm64"
	case strings.Contains(lower, "amd64") || strings.Contains(lower, "x86_64") || strings.Contains(lower, "x64"):
		arch = "amd64"
	case strings.Contains(lower, "386") || strings.Contains(lower, "i686"):
		arch = "386"
	}

	return
}
