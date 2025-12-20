package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/librescoot/uplink-server/internal/auth"
	"github.com/librescoot/uplink-server/internal/handlers"
	"github.com/librescoot/uplink-server/internal/models"
	"github.com/librescoot/uplink-server/internal/storage"
)

const version = "1.0.0"

func main() {
	configPath := flag.String("config", "config.yml", "Path to configuration file")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("Starting uplink-server v%s", version)

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize components
	authenticator := auth.NewAuthenticator(config)
	connMgr := storage.NewConnectionManager()

	// Start stats logger
	connMgr.StartStatsLogger(config.Logging.GetStatsInterval())

	// Initialize handlers
	wsHandler := handlers.NewWebSocketHandler(
		authenticator,
		connMgr,
		config.Server.GetKeepaliveInterval(),
	)

	// Setup WebSocket server
	http.HandleFunc("/ws", wsHandler.HandleConnection)

	// Start servers
	wsAddr := fmt.Sprintf(":%d", config.Server.WSPort)
	log.Printf("WebSocket server listening on %s", wsAddr)
	log.Printf("Keepalive interval: %s", config.Server.KeepaliveInterval)
	log.Printf("Configured scooters: %d", len(config.Auth.Tokens))

	if err := http.ListenAndServe(wsAddr, nil); err != nil {
		log.Fatalf("WebSocket server error: %v", err)
	}
}

func loadConfig(path string) (*models.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config models.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Server.WSPort == 0 {
		config.Server.WSPort = 8080
	}
	if config.Server.KeepaliveInterval == "" {
		config.Server.KeepaliveInterval = "5m"
	}
	if config.Logging.StatsInterval == "" {
		config.Logging.StatsInterval = "30s"
	}

	return &config, nil
}
