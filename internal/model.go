package internal

type VersionFile struct {
	UpdatedAt  string                       `json:"updated_at"`
	Components map[string]*ComponentVersion `json:"components"`
}

type ComponentVersion struct {
	LatestVersion string       `json:"latest_version"`
	ReleaseMeta   *ReleaseMeta `json:"release_meta,omitempty"`
	FetchedAt     string       `json:"fetched_at"`
	Source        string       `json:"source"`
	Status        string       `json:"status"`
	Error         string       `json:"error,omitempty"`
}

type ReleaseMeta struct {
	Tag       string            `json:"tag"`
	Assets    []ReleaseAsset    `json:"assets"`
	Checksums map[string]string `json:"checksums"`
}

type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
	OS   string `json:"os,omitempty"`
	Arch string `json:"arch,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type ComponentDef struct {
	Name   string
	Source string // "github:owner/repo" or "npm:package"
}

var DefaultComponents = []ComponentDef{
	{Name: "octo-daemon", Source: "github:Mininglamp-OSS/octo-daemon-cli"},
	{Name: "octo-version-sync", Source: "github:Mininglamp-OSS/octo-version-sync"},
	{Name: "claude", Source: "github:anthropics/claude-code"},
	{Name: "hermes", Source: "github:NousResearch/hermes-agent"},
	{Name: "openclaw", Source: "npm:openclaw"},
	{Name: "octo", Source: "github:Mininglamp-OSS/openclaw-channel-octo"},
}
