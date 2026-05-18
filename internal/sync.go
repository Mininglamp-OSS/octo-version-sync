package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Syncer struct {
	fetcher    *Fetcher
	store      VersionStore
	components []ComponentDef
	interval   time.Duration
	triggerCh  chan struct{}
	listen     string
	token      string
}

func NewSyncer(cfg Config) (*Syncer, error) {
	cfg.WithDefaults()

	host, _, _ := net.SplitHostPort(cfg.ListenAddr)
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && cfg.TriggerToken == "" {
		return nil, fmt.Errorf("--trigger-token is required when listening on %s (non-localhost)", cfg.ListenAddr)
	}

	var store VersionStore
	switch cfg.StoreType {
	case "file":
		store = NewFileStore(cfg.OutputPath)
	case "cos":
		if cfg.COSBucketURL == "" || cfg.COSSecretID == "" || cfg.COSSecretKey == "" {
			return nil, fmt.Errorf("--store cos requires --cos-bucket-url, --cos-secret-id, --cos-secret-key")
		}
		store = NewCOSStore(cfg.COSBucketURL, cfg.COSSecretID, cfg.COSSecretKey, cfg.COSFolder)
	default:
		return nil, fmt.Errorf("unknown store type: %s", cfg.StoreType)
	}

	comps, err := LoadComponents(cfg.ComponentsFile)
	if err != nil {
		return nil, fmt.Errorf("load components: %w", err)
	}
	log.Printf("[INFO] monitoring %d component(s)", len(comps))

	return &Syncer{
		fetcher:    NewFetcher(cfg.GitHubToken),
		store:      store,
		components: comps,
		interval:   cfg.Interval,
		triggerCh:  make(chan struct{}, 1),
		listen:     cfg.ListenAddr,
		token:      cfg.TriggerToken,
	}, nil
}

func (s *Syncer) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.listen)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listen, err)
	}
	go s.serveHTTP(ln)

	s.syncOnce(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.syncOnce(ctx)
		case <-s.triggerCh:
			s.syncOnce(ctx)
		}
	}
}

func (s *Syncer) syncOnce(ctx context.Context) {
	log.Println("[INFO] starting sync...")

	existing := s.loadExisting(ctx)

	type result struct {
		name string
		ver  *ComponentVersion
		err  error
	}
	ch := make(chan result, len(s.components))
	var wg sync.WaitGroup

	for _, comp := range s.components {
		wg.Add(1)
		go func(c ComponentDef) {
			defer wg.Done()
			ver, err := s.fetcher.Fetch(ctx, c)
			ch <- result{name: c.Name, ver: ver, err: err}
		}(comp)
	}

	wg.Wait()
	close(ch)

	if existing.Components == nil {
		existing.Components = make(map[string]*ComponentVersion)
	}

	changed := 0
	for r := range ch {
		if r.err != nil {
			log.Printf("[WARN] %s fetch failed: %v", r.name, r.err)
			prev, exists := existing.Components[r.name]
			if exists && prev.Status == "ok" {
				prev.Status = "stale"
				prev.Error = r.err.Error()
			} else if !exists {
				existing.Components[r.name] = &ComponentVersion{
					Source: s.findSource(r.name),
					Status: "error",
					Error:  r.err.Error(),
				}
			} else {
				prev.Error = r.err.Error()
			}
			continue
		}
		prev := existing.Components[r.name]
		if prev == nil || prev.LatestVersion != r.ver.LatestVersion {
			if prev != nil {
				log.Printf("[INFO] %s updated: %s → %s", r.name, prev.LatestVersion, r.ver.LatestVersion)
			} else {
				log.Printf("[INFO] %s fetched: %s", r.name, r.ver.LatestVersion)
			}
			changed++
		}
		existing.Components[r.name] = r.ver
	}

	// 清理 components.json 里已移除的组件，避免残留"僵尸条目"。
	configured := make(map[string]struct{}, len(s.components))
	for _, c := range s.components {
		configured[c.Name] = struct{}{}
	}
	for name := range existing.Components {
		if _, ok := configured[name]; !ok {
			log.Printf("[INFO] %s removed from components.json, pruning from output", name)
			delete(existing.Components, name)
		}
	}

	existing.UpdatedAt = nowBeijing()

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		log.Printf("[ERROR] marshal version.json: %v", err)
		return
	}

	if err := s.store.Write(ctx, data); err != nil {
		log.Printf("[ERROR] write version.json: %v", err)
		return
	}

	log.Printf("[INFO] sync complete, %d component(s) updated", changed)
}

func (s *Syncer) loadExisting(ctx context.Context) VersionFile {
	data, err := s.store.Read(ctx)
	if err != nil {
		return VersionFile{Components: make(map[string]*ComponentVersion)}
	}
	var vf VersionFile
	if err := json.Unmarshal(data, &vf); err != nil {
		return VersionFile{Components: make(map[string]*ComponentVersion)}
	}
	return vf
}

func (s *Syncer) serveHTTP(ln net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/trigger", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.token != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+s.token {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		select {
		case s.triggerCh <- struct{}{}:
			w.Write([]byte(`{"status":"queued"}`))
		default:
			w.Write([]byte(`{"status":"already_queued"}`))
		}
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	log.Printf("[INFO] HTTP trigger listening on %s", s.listen)
	if err := http.Serve(ln, mux); err != nil {
		log.Printf("[ERROR] HTTP server: %v", err)
	}
}

func (s *Syncer) findSource(name string) string {
	for _, c := range s.components {
		if c.Name == name {
			return c.Source
		}
	}
	return ""
}
