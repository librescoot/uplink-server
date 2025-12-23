package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

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
	responseStore := storage.NewResponseStore(1 * time.Hour)
	stateStore := storage.NewStateStore()
	eventStore := storage.NewEventStore(1000) // Keep last 1000 events per scooter

	// Start stats logger
	connMgr.StartStatsLogger(config.Logging.GetStatsInterval())

	// Initialize handlers
	wsHandler := handlers.NewWebSocketHandler(
		authenticator,
		connMgr,
		responseStore,
		stateStore,
		eventStore,
		config.Server.GetKeepaliveInterval(),
	)

	apiHandler := handlers.NewAPIHandler(wsHandler, connMgr, responseStore, stateStore, eventStore, config.Auth.APIKey)

	// Setup routes
	if config.Server.EnableWebUI {
		http.HandleFunc("/", serveWebUI)
		http.HandleFunc("/images/", serveImages)

		// WebSocket for web UI real-time updates
		webUIHandler := handlers.NewWebUIHandler(stateStore, eventStore, connMgr, config.Auth.APIKey)
		http.HandleFunc("/ws/web", webUIHandler.HandleWebConnection)

		log.Printf("Web UI enabled at /")
	}
	http.HandleFunc("/ws", wsHandler.HandleConnection)
	http.HandleFunc("/api/commands", apiHandler.HandleCommands)
	http.HandleFunc("/api/commands/", apiHandler.HandleCommandResponse)
	http.HandleFunc("/api/scooters", apiHandler.HandleScooters)
	http.HandleFunc("/api/scooters/", apiHandler.HandleScooterDetail)

	// Start server
	wsAddr := fmt.Sprintf(":%d", config.Server.WSPort)
	log.Printf("Server listening on %s", wsAddr)
	log.Printf("  WebSocket endpoint: /ws")
	if config.Server.EnableWebUI {
		log.Printf("  Web UI WebSocket: /ws/web")
	}
	log.Printf("  REST API endpoints: /api/commands, /api/scooters")
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

func serveWebUI(w http.ResponseWriter, r *http.Request) {
	// Only serve index.html at root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, "web/index.html")
}

func serveImages(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web"+r.URL.Path)
}
