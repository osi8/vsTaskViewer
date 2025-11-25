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

	// Create WebSocket upgrader with CORS settings
	upgrader := createUpgrader(config.Server.AllowedOrigins)

	// Initialize rate limiter
	rateLimiter := NewRateLimiter(config.Server.RateLimitRPM)

	// Setup HTTP server with request size limits
	maxRequestSize := config.Server.MaxRequestSize
	if maxRequestSize == 0 {
		maxRequestSize = 10 * 1024 * 1024 // Default 10MB
	}

	mux := http.NewServeMux()

	// API endpoint to start tasks (with rate limiting)
	mux.HandleFunc("/api/start", RateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Enforce request size limit
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
		handleStartTask(w, r, taskManager, config)
	}, rateLimiter))

	// Viewer endpoint (with rate limiting)
	mux.HandleFunc("/viewer", RateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handleViewer(w, r, taskManager, config)
	}, rateLimiter))

	// WebSocket endpoint (with rate limiting)
	mux.HandleFunc("/ws", RateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, taskManager, config, upgrader)
	}, rateLimiter))

	// Health check endpoint (no rate limiting)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      mux,
		MaxHeaderBytes: 1 << 20, // 1MB max header size
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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

	// Start server with or without TLS
	if config.Server.TLSKeyFile != "" && config.Server.TLSCertFile != "" {
		// Validate TLS files exist
		if _, err := os.Stat(config.Server.TLSKeyFile); os.IsNotExist(err) {
			log.Fatalf("TLS key file not found: %s", config.Server.TLSKeyFile)
		}
		if _, err := os.Stat(config.Server.TLSCertFile); os.IsNotExist(err) {
			log.Fatalf("TLS certificate file not found: %s", config.Server.TLSCertFile)
		}
		log.Printf("Starting HTTPS server on port %d", *port)
		log.Printf("TLS key: %s", config.Server.TLSKeyFile)
		log.Printf("TLS cert: %s", config.Server.TLSCertFile)
		if err := server.ListenAndServeTLS(config.Server.TLSCertFile, config.Server.TLSKeyFile); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	} else {
		log.Printf("Starting HTTP server on port %d", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
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

	// Set default HTML directory if not specified
	if config.Server.HTMLDir == "" {
		config.Server.HTMLDir = "./html"
	}

	// Validate HTML directory exists
	if _, err := os.Stat(config.Server.HTMLDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("HTML directory does not exist: %s", config.Server.HTMLDir)
	}

	return &config, nil
}

