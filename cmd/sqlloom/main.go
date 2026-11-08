package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/config"
	"github.com/alexandrmotologa/sqlloom/internal/proxy"
	"github.com/alexandrmotologa/sqlloom/internal/telemetry"
	"github.com/alexandrmotologa/sqlloom/internal/ui"
)

func main() {
	cfg := config.LoadConfig()

	printBanner(cfg)

	// 1. Initialize Cache and Invalidator
	maxMemoryBytes := int64(cfg.CacheMaxMemoryMB) * 1024 * 1024
	cacheLRU := cache.NewShardedLRU(maxMemoryBytes, int64(cfg.CacheMaxEntries))
	invalidator := cache.NewInvalidator(cacheLRU)

	// 2. Telemetry and Event Hub
	metrics := telemetry.GlobalMetrics
	hub := telemetry.GlobalHub

	// 3. Embedded React 19 UI
	uiFS, err := ui.GetFS()
	if err != nil {
		log.Printf("[WARN] Failed to load embedded UI filesystem: %v", err)
	}

	// 4. Start HTTP Cockpit and Telemetry Server
	httpServer := telemetry.NewHTTPServer(
		cfg.DashboardAddr,
		metrics,
		hub,
		cacheLRU,
		invalidator,
		uiFS,
	)
	if err := httpServer.Start(); err != nil {
		log.Fatalf("[FATAL] Failed to start HTTP cockpit server on %s: %v", cfg.DashboardAddr, err)
	}
	log.Printf("[INFO] Query Cockpit and Telemetry listening on http://127.0.0.1%s", cfg.DashboardAddr)

	// 5. Start PostgreSQL Wire-Protocol Proxy Server
	proxyServer := proxy.NewServer(cfg, cacheLRU, invalidator, metrics, hub)
	if err := proxyServer.Start(); err != nil {
		log.Fatalf("[FATAL] Failed to bind proxy listener on %s: %v", cfg.ListenAddr, err)
	}
	log.Printf("[INFO] PostgreSQL wire proxy listening on %s (forwarding to %s)", cfg.ListenAddr, cfg.UpstreamAddr)

	// 6. Graceful shutdown handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("[INFO] Shutdown signal received, draining connections...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := proxyServer.Shutdown(ctx); err != nil {
		log.Printf("[WARN] Error during proxy shutdown: %v", err)
	}

	log.Println("[INFO] sqlloom proxy stopped cleanly.")
}

func printBanner(cfg *config.Config) {
	fmt.Println(`
   _____ ____    __    __                          
  / ___// __ \  / /   / /   ____  ____  ____ ___   
  \__ \/ / / / / /   / /   / __ \/ __ \/ __ ` + "`" + `__ \  
 ___/ / /_/ / / /___/ /___/ /_/ / /_/ / / / / / /  
/____/\___\_\/_____/_____/\____/\____/_/ /_/ /_/   
  `)
	fmt.Println("Transparent PostgreSQL Wire-Protocol Query Cache & Read Acceleration Proxy")
	fmt.Printf("Listen: %s | Upstream: %s | Dashboard: %s\n", cfg.ListenAddr, cfg.UpstreamAddr, cfg.DashboardAddr)
	fmt.Printf("Memory Limit: %d MB | Max Entries: %d | Default TTL: %ds\n", cfg.CacheMaxMemoryMB, cfg.CacheMaxEntries, cfg.DefaultTTLSec)
	fmt.Println("-------------------------------------------------------------------------")
}
