package main

import (
	"log"
	"os"
	"strconv"

	"github.com/kuadrant/inferno/internal/server"
)

func main() {
	cfg := server.DefaultConfig()

	if port := os.Getenv("EXT_PROC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.ExtProcPort = p
		}
	}

	srv := server.NewServer(cfg)
	log.Println("Starting Inferno ext_proc service")

	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
