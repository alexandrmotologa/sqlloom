package config

import (
	"flag"
	"os"
	"strconv"
)

// Config holds runtime configuration options for sqlloom.
type Config struct {
	ListenAddr       string
	UpstreamAddr     string
	DashboardAddr    string
	CacheMaxMemoryMB int
	CacheMaxEntries  int
	DefaultTTLSec    int
	Verbose          bool
	DisableCache     bool
}

// LoadConfig parses CLI flags and environment variables.
func LoadConfig() *Config {
	cfg := &Config{
		ListenAddr:       getEnv("SQLLOOM_LISTEN", ":5433"),
		UpstreamAddr:     getEnv("SQLLOOM_UPSTREAM", "127.0.0.1:5432"),
		DashboardAddr:    getEnv("SQLLOOM_DASHBOARD", ":8080"),
		CacheMaxMemoryMB: getEnvInt("SQLLOOM_CACHE_MAX_MEMORY_MB", 32),
		CacheMaxEntries:  getEnvInt("SQLLOOM_CACHE_MAX_ENTRIES", 10000),
		DefaultTTLSec:    getEnvInt("SQLLOOM_DEFAULT_TTL_SEC", 3600),
		Verbose:          getEnvBool("SQLLOOM_VERBOSE", false),
		DisableCache:     getEnvBool("SQLLOOM_DISABLE_CACHE", false),
	}

	flag.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "TCP address to listen on for client PostgreSQL connections")
	flag.StringVar(&cfg.UpstreamAddr, "upstream", cfg.UpstreamAddr, "Upstream PostgreSQL server address")
	flag.StringVar(&cfg.DashboardAddr, "dashboard", cfg.DashboardAddr, "HTTP address for query cockpit dashboard and metrics")
	flag.IntVar(&cfg.CacheMaxMemoryMB, "cache-mem-mb", cfg.CacheMaxMemoryMB, "Maximum cache memory limit in megabytes")
	flag.IntVar(&cfg.CacheMaxEntries, "cache-entries", cfg.CacheMaxEntries, "Maximum number of cached query entries")
	flag.IntVar(&cfg.DefaultTTLSec, "ttl", cfg.DefaultTTLSec, "Default cache entry TTL in seconds (0 for indefinite until invalidated)")
	flag.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "Enable verbose debug logging")
	flag.BoolVar(&cfg.DisableCache, "disable-cache", cfg.DisableCache, "Disable caching engine (pure pass-through proxy mode)")

	flag.Parse()
	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return fallback
}
