package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alexandrmotologa/sqlloom/internal/cache"
	"github.com/alexandrmotologa/sqlloom/internal/config"
	"github.com/alexandrmotologa/sqlloom/internal/parser"
	"github.com/alexandrmotologa/sqlloom/internal/proxy"
	"github.com/alexandrmotologa/sqlloom/internal/telemetry"
	"github.com/alexandrmotologa/sqlloom/internal/ui"
	"github.com/alexandrmotologa/sqlloom/internal/wire"
)

const Version = "1.0.0"

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "version" || arg == "--version" || arg == "-v" || arg == "-V" {
			fmt.Printf("sqlloom v%s\n", Version)
			return
		}
		if arg == "help" || arg == "--help" || arg == "-h" {
			printUsage()
			return
		}
	}

	if len(os.Args) > 1 && os.Args[1] == "explain" {
		query := ""
		if len(os.Args) > 2 {
			query = strings.Join(os.Args[2:], " ")
		}
		runExplain(query)
		return
	}

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
		cfg.ListenAddr,
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

func runExplain(query string) {
	if strings.TrimSpace(query) == "" {
		fmt.Println("Usage: sqlloom explain \"<SQL_STATEMENT>\"")
		os.Exit(1)
	}

	normSQL, directives := parser.NormalizeQuery(query)
	key := parser.ComputeCacheKey("default", normSQL, nil)
	decision := parser.EvaluateCacheability(query, wire.TxStatusIdle)

	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	shard := h.Sum32() % 16

	fmt.Println("=== sqlloom Query Diagnostics & Fingerprint ===")
	fmt.Printf("Raw SQL:              %s\n", query)
	fmt.Printf("Statement Kind:       %s\n", decision.Kind)
	if decision.Cacheable {
		fmt.Println("Cacheable:            YES (Eligible for < 0.5ms acceleration)")
	} else {
		fmt.Printf("Cacheable:            NO (Bypassed: %s)\n", decision.Reason)
	}
	fmt.Printf("Normalized Template:  %s\n", normSQL)
	fmt.Printf("Deterministic Key:    %s\n", key)
	fmt.Printf("Target LRU Shard:     Shard #%d (of 16 striped shards)\n", shard)
	fmt.Printf("Dependent Tables:     %v\n", decision.Tables)
	if len(directives) > 0 {
		fmt.Printf("SQL Directives:       %v\n", directives)
	}
	if decision.CustomTTL > 0 {
		fmt.Printf("Custom Rule TTL:      %d seconds\n", decision.CustomTTL)
	}
	fmt.Println("=================================================")
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
	fmt.Printf("Memory Limit: %d MB | Max Entries: %d | Default TTL: %ds | Grace Mode: %v\n", cfg.CacheMaxMemoryMB, cfg.CacheMaxEntries, cfg.DefaultTTLSec, cfg.GraceMode)
	fmt.Println("-------------------------------------------------------------------------")
}

func printUsage() {
	fmt.Printf(`sqlloom v%s - Transparent PostgreSQL Wire-Protocol Query Cache & Read Acceleration Proxy

Usage:
  sqlloom [command] [flags]

Commands:
  (default)  Start the PostgreSQL wire proxy daemon and web dashboard
  explain    Diagnose and fingerprint a SQL statement without connecting to a database
  version    Display version information
  help       Show this help message

Environment Variables:
  SQLLOOM_LISTEN_ADDR       Proxy listen address (default: 127.0.0.1:5433)
  SQLLOOM_UPSTREAM_ADDR     Upstream PostgreSQL address (default: 127.0.0.1:5432)
  SQLLOOM_DASHBOARD_ADDR    Telemetry dashboard address (default: :9191)
  SQLLOOM_CACHE_MAX_MEMORY  Max memory cache in MB (default: 128)
  SQLLOOM_CACHE_MAX_ENTRIES Max cache entries (default: 50000)
  SQLLOOM_DEFAULT_TTL_SEC   Default TTL in seconds (default: 60)
  SQLLOOM_GRACE_MODE        Serve stale on upstream failure (default: true)
`, Version)
}
