package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

var (
	configPathFlag   = flag.String("c", "", "Path to configuration file (optional)")
	templatesPathFlag = flag.String("t", "", "Path to templates/HTML directory (optional)")
	port             = flag.Int("p", 8080, "Port to listen on")
	showHelp         = flag.Bool("h", false, "Show help message")
)

const usage = `vsTaskViewer - Task execution viewer with WebSocket support

Usage:
  vsTaskViewer [options]

Options:
  -c string    Path to configuration file (optional)
               Search order:
                 1. Path specified with -c flag
                 2. vsTaskViewer.toml in same directory as binary
                 3. /etc/vsTaskViewer/vsTaskViewer.toml

  -t string    Path to templates/HTML directory (optional)
               Search order:
                 1. Path specified with -t flag
                 2. html/ in same directory as binary
                 3. /etc/vsTaskViewer/html/

  -p int       Port to listen on (default: 8080, can be overridden in config)
  -h           Show this help message

Examples:
  vsTaskViewer
  vsTaskViewer -c /path/to/config.toml
  vsTaskViewer -c /path/to/config.toml -t /path/to/html
  vsTaskViewer -p 9090
`

func main() {
	flag.Parse()

	// Show help if requested
	if *showHelp {
		fmt.Print(usage)
		os.Exit(0)
	}

	// Find configuration file
	configPath, err := findConfigFile(*configPathFlag)
	if err != nil {
		log.Fatalf("Failed to find config file: %v", err)
	}
	log.Printf("Using config file: %s", configPath)

	// Load configuration
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override HTML directory if -t flag is set, otherwise use search order
	if *templatesPathFlag != "" {
		// Resolve relative paths to absolute
		htmlDir, err := filepath.Abs(*templatesPathFlag)
		if err != nil {
			log.Fatalf("Failed to resolve templates path: %v", err)
		}
		config.Server.HTMLDir = htmlDir
		log.Printf("Using templates directory from -t flag: %s", config.Server.HTMLDir)
	} else if config.Server.HTMLDir == "" {
		htmlDir, err := findTemplatesDir()
		if err != nil {
			log.Fatalf("Failed to find templates directory: %v", err)
		}
		config.Server.HTMLDir = htmlDir
		log.Printf("Using templates directory: %s", config.Server.HTMLDir)
	} else {
		// HTMLDir is set in config, resolve relative paths
		if !filepath.IsAbs(config.Server.HTMLDir) {
			htmlDir, err := filepath.Abs(config.Server.HTMLDir)
			if err != nil {
				log.Fatalf("Failed to resolve HTML directory path: %v", err)
			}
			config.Server.HTMLDir = htmlDir
		}
		log.Printf("Using templates directory from config: %s", config.Server.HTMLDir)
	}

	// Validate HTML directory exists (after resolving paths)
	if _, err := os.Stat(config.Server.HTMLDir); os.IsNotExist(err) {
		log.Fatalf("HTML directory does not exist: %s", config.Server.HTMLDir)
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

	// Note: HTML directory validation is done in main() after path resolution

	return &config, nil
}

// getBinaryDir returns the directory where the binary is located
func getBinaryDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	realPath, err := filepath.EvalSymlinks(execPath)
	if err == nil {
		execPath = realPath
	}

	return filepath.Dir(execPath), nil
}

// findConfigFile searches for the configuration file in the specified order
func findConfigFile(flagPath string) (string, error) {
	// 1. Path specified with -c flag
	if flagPath != "" {
		if _, err := os.Stat(flagPath); err == nil {
			return flagPath, nil
		}
		return "", fmt.Errorf("config file specified with -c flag not found: %s", flagPath)
	}

	// 2. vsTaskViewer.toml in same directory as binary
	binaryDir, err := getBinaryDir()
	if err == nil {
		localConfig := filepath.Join(binaryDir, "vsTaskViewer.toml")
		if _, err := os.Stat(localConfig); err == nil {
			return localConfig, nil
		}
	}

	// 3. /etc/vsTaskViewer/vsTaskViewer.toml
	systemConfig := "/etc/vsTaskViewer/vsTaskViewer.toml"
	if _, err := os.Stat(systemConfig); err == nil {
		return systemConfig, nil
	}

	return "", fmt.Errorf("config file not found in any of the search locations:\n  1. -c flag path\n  2. %s/vsTaskViewer.toml\n  3. %s", binaryDir, systemConfig)
}

// findTemplatesDir searches for the templates directory in the specified order
func findTemplatesDir() (string, error) {
	// 1. html/ in same directory as binary
	binaryDir, err := getBinaryDir()
	if err == nil {
		localTemplates := filepath.Join(binaryDir, "html")
		// Ensure absolute path
		localTemplates, err = filepath.Abs(localTemplates)
		if err == nil {
			if _, err := os.Stat(localTemplates); err == nil {
				return localTemplates, nil
			}
		}
	}

	// 2. /etc/vsTaskViewer/html/
	systemTemplates := "/etc/vsTaskViewer/html"
	if _, err := os.Stat(systemTemplates); err == nil {
		return systemTemplates, nil
	}

	return "", fmt.Errorf("templates directory not found in any of the search locations:\n  1. -t flag path\n  2. %s/html\n  3. %s", binaryDir, systemTemplates)
}

