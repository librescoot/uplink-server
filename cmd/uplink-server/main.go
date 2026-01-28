package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/librescoot/uplink-server/internal/auth"
	"github.com/librescoot/uplink-server/internal/handlers"
	"github.com/librescoot/uplink-server/internal/models"
	"github.com/librescoot/uplink-server/internal/storage"
)

const version = "1.0.0"

func main() {
	// Check if subcommand is provided
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add-client":
			addClientCommand()
			return
		case "init":
			initConfigCommand()
			return
		}
	}

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
	stateStore := storage.NewStateStore("data/state.json")
	eventStore := storage.NewEventStore(1000, "data/events.jsonl") // Keep last 1000 events per scooter

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
		webUIHandler := handlers.NewWebUIHandler(stateStore, eventStore, connMgr, authenticator, config.Auth.APIKey)
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

	server := &http.Server{Addr: wsAddr}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Printf("Server stopped")
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

// addClientCommand handles the add-client subcommand
func addClientCommand() {
	fs := flag.NewFlagSet("add-client", flag.ExitOnError)
	identifier := fs.String("identifier", "", "Client identifier (required)")
	name := fs.String("name", "", "Human-friendly name (optional)")
	configPath := fs.String("config", "config.yml", "Path to server configuration file")
	endpoint := fs.String("endpoint", "", "Server WebSocket endpoint (e.g., ws://localhost:8080/ws)")

	fs.Parse(os.Args[2:])

	if *identifier == "" {
		fmt.Fprintln(os.Stderr, "Error: -identifier is required")
		fs.PrintDefaults()
		os.Exit(1)
	}

	// Generate secure token (32 bytes = 64 hex chars)
	token, err := generateSecureToken(32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		os.Exit(1)
	}

	// Load server config
	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize tokens map if nil
	if config.Auth.Tokens == nil {
		config.Auth.Tokens = make(map[string]models.ScooterConfig)
	}

	// Check if identifier already exists
	if _, exists := config.Auth.Tokens[*identifier]; exists {
		fmt.Fprintf(os.Stderr, "Error: Identifier '%s' already exists in config\n", *identifier)
		os.Exit(1)
	}

	// Add new client to config
	config.Auth.Tokens[*identifier] = models.ScooterConfig{
		Token: token,
		Name:  *name,
	}

	// Write updated config back to file
	if err := saveConfig(*configPath, config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	// Create client config
	clientConfig := map[string]any{
		"uplink": map[string]any{
			"identifier": *identifier,
			"token":      token,
		},
	}

	// Add endpoint if provided, otherwise use placeholder
	if *endpoint != "" {
		clientConfig["uplink"].(map[string]any)["endpoint"] = *endpoint
	} else {
		clientConfig["uplink"].(map[string]any)["endpoint"] = "ws://CHANGEME:8080/ws"
	}

	// Marshal client config
	clientData, err := yaml.Marshal(clientConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling client config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Added client '%s' to %s\n\n", *identifier, *configPath)
	fmt.Printf("Client configuration:\n\n")
	fmt.Print(string(clientData))
}

// initConfigCommand handles the init subcommand
func initConfigCommand() {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configPath := fs.String("config", "config.yml", "Path for config file")

	fs.Parse(os.Args[2:])

	// Check if file already exists
	if _, err := os.Stat(*configPath); err == nil {
		fmt.Fprintf(os.Stderr, "Error: Config file '%s' already exists\n", *configPath)
		fmt.Fprintf(os.Stderr, "Use a different path with -config flag\n")
		os.Exit(1)
	}

	// Generate secure API key (32 bytes = 64 hex chars)
	apiKey, err := generateSecureToken(32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating API key: %v\n", err)
		os.Exit(1)
	}

	// Create default config
	config := &models.Config{
		Server: models.ServerConfig{
			WSPort:            8080,
			LPPort:            8081,
			SSEPort:           8082,
			EnableWebUI:       true,
			KeepaliveInterval: "5m",
			LPTimeout:         "24h",
		},
		Auth: models.AuthConfig{
			APIKey: apiKey,
			Tokens: make(map[string]models.ScooterConfig),
		},
		Storage: models.StorageConfig{
			Type: "memory",
		},
		Logging: models.LoggingConfig{
			Level:         "info",
			StatsInterval: "30s",
		},
	}

	// Write config to file
	if err := saveConfig(*configPath, config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Created config file: %s\n\n", *configPath)
	fmt.Printf("API Key (save this securely):\n\n")
	fmt.Printf("  %s\n\n", apiKey)
	fmt.Printf("Use this API key to authenticate web UI and REST API requests.\n")
	fmt.Printf("Add clients with: ./uplink-server add-client -identifier <id> -config %s\n", *configPath)
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// saveConfig saves the configuration to a file
func saveConfig(path string, config *models.Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
