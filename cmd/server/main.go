package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"distributed-kvstore/internal/config"
	"distributed-kvstore/internal/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.Parse()

	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		printUsage()
		return
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	srv, err := server.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func printUsage() {
	fmt.Printf(`Distributed Key-Value Store Server

Usage:
  %s [options]

Options:
  -config string
        Path to configuration file (default "config.yaml")
  -h, --help
        Show this help message

Environment Variables:
  Configuration can be overridden using environment variables with KV_ prefix.
  See docs/configuration.md for details.

Examples:
  # Start with default config
  %s

  # Start with custom config file
  %s -config /path/to/config.yaml

  # Start with environment override
  KV_SERVER_PORT=9000 %s

For more information, see:
  - Configuration: docs/configuration.md
  - API Reference: docs/api.md
  - Architecture: docs/architecture.md
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}