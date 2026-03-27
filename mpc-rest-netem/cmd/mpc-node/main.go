package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mphtlc-lgp-mpc/internal/config"
	"mphtlc-lgp-mpc/internal/httpapi"
	"mphtlc-lgp-mpc/internal/orchestrator"
	"mphtlc-lgp-mpc/internal/tssutil"
)

func main() {
	cfg, err := config.LoadHTTPConfigFromEnv()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	localPartyID := tssutil.BuildPartyID(cfg.NodeID)
	log.Printf(
		"node=%s listen=%s base=%s share=%s party_key=%s",
		cfg.NodeID,
		cfg.ListenAddr,
		cfg.BaseURL,
		cfg.SharePath,
		localPartyID.KeyInt().String(),
	)

	mgr := orchestrator.NewManager(cfg)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.NewServer(mgr).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = srv.Shutdown(ctx)
}