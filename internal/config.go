package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Interval       time.Duration
	StoreType      string // "file" or "cos"
	OutputPath     string
	ListenAddr     string
	TriggerToken   string
	GitHubToken    string
	COSBucketURL   string
	COSSecretID    string
	COSSecretKey   string
	COSFolder      string // COS 子目录，如 "version-sync"
	ComponentsFile string // path to components.json
}

func (c *Config) WithDefaults() {
	if c.Interval == 0 {
		c.Interval = 10 * time.Minute
	}
	if c.StoreType == "" {
		c.StoreType = "file"
	}
	if c.OutputPath == "" {
		c.OutputPath = "./output/version.json"
	}
	if c.ListenAddr == "" {
		c.ListenAddr = "127.0.0.1:8099"
	}
}

func LoadComponents(path string) ([]ComponentDef, error) {
	if path == "" {
		return DefaultComponents, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read components file: %w", err)
	}
	var comps []ComponentDef
	if err := json.Unmarshal(data, &comps); err != nil {
		return nil, fmt.Errorf("parse components file: %w", err)
	}
	if len(comps) == 0 {
		return nil, fmt.Errorf("components file is empty")
	}
	return comps, nil
}
