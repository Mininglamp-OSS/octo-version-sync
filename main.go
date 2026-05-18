package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mininglamp-OSS/octo-version-sync/internal"
)

func main() {
	cfg := internal.Config{}

	flag.DurationVar(&cfg.Interval, "interval", 0, "sync interval (default 10m)")
	flag.StringVar(&cfg.StoreType, "store", "file", "storage type: file or cos")
	flag.StringVar(&cfg.OutputPath, "output", "", "output path for file store (default ./output/version.json)")
	flag.StringVar(&cfg.ListenAddr, "listen", "", "HTTP trigger listen address (default 127.0.0.1:8099)")
	flag.StringVar(&cfg.TriggerToken, "trigger-token", "", "Bearer token for /trigger endpoint (required for non-localhost)")
	flag.StringVar(&cfg.GitHubToken, "github-token", "", "GitHub personal access token (optional, increases rate limit)")
	flag.StringVar(&cfg.COSBucketURL, "cos-bucket-url", "", "COS bucket URL")
	flag.StringVar(&cfg.COSSecretID, "cos-secret-id", "", "COS secret ID")
	flag.StringVar(&cfg.COSSecretKey, "cos-secret-key", "", "COS secret key")
	flag.StringVar(&cfg.COSFolder, "cos-folder", "", "COS folder prefix (e.g. version-sync)")
	flag.StringVar(&cfg.ComponentsFile, "components", "", "path to components.json (default: built-in list)")
	flag.Parse()

	cfg.WithDefaults()

	syncer, err := internal.NewSyncer(cfg)
	if err != nil {
		log.Fatalf("[FATAL] %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("[INFO] shutting down...")
		cancel()
	}()

	log.Printf("[INFO] octo-version-sync starting (interval=%s, store=%s, listen=%s)", cfg.Interval, cfg.StoreType, cfg.ListenAddr)
	if err := syncer.Run(ctx); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}
}
