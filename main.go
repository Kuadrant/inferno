package main

import (
	"log"
	"os"
	"strconv"

	"github.com/kuadrant/inferno/internal/server"
)

func main() {
	cfg := server.DefaultConfig()

	// default ports from environment variables if provided
	if port := os.Getenv("SEMANTIC_CACHE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.SemanticCachePort = p
		}
	}

	if port := os.Getenv("PROMPT_GUARD_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.PromptGuardPort = p
		}
	}

	if port := os.Getenv("TOKEN_METRICS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.TokenMetricsPort = p
		}
	}

	srv := server.NewServer(cfg)
	log.Println("Starting Inferno ext_proc service")

	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
