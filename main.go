package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

var (
	configPath = flag.String("config", "/etc/vsTaskViewer.toml", "Path to configuration file")
	port       = flag.Int("port", 8080, "Port to listen on")
)

func main() {
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override port from config if specified
	if config.Server.Port > 0 {
		*port = config.Server.Port
	}

	// Initialize task manager
	taskManager := NewTaskManager(config)

	// Setup HTTP server
	mux := http.NewServeMux()

	// API endpoint to start tasks
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		handleStartTask(w, r, taskManager, config)
	})

	// Viewer endpoint
	mux.HandleFunc("/viewer", func(w http.ResponseWriter, r *http.Request) {
		handleViewer(w, r, config)
	})

	// WebSocket endpoint
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, taskManager, config)
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	// Graceful shutdown
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("Starting server on port %d", *port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	// Validate config
	if config.Auth.Secret == "" {
		return nil, fmt.Errorf("auth.secret must be set in config")
	}

	if len(config.Tasks) == 0 {
		return nil, fmt.Errorf("at least one task must be defined in config")
	}

	return &config, nil
}

