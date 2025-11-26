package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

var (
	configPathFlag   = flag.String("c", "", "Path to configuration file (optional)")
	templatesPathFlag = flag.String("t", "", "Path to templates/HTML directory (optional)")
	taskDirFlag      = flag.String("d", "", "Path to task output directory (optional)")
	execUserFlag     = flag.String("u", "", "User to run as (optional)")
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

  -d string    Path to task output directory (optional)
               Search order:
                 1. Path specified with -d flag
                 2. task_dir from config file
                 3. /var/vsTaskViewer

  -u string    User to run as (optional)
               Search order:
                 1. User specified with -u flag
                 2. exec_user from config file
                 3. www-data

  -p int       Port to listen on (default: 8080, can be overridden in config)
  -h           Show this help message

Examples:
  vsTaskViewer
  vsTaskViewer -c /path/to/config.toml
  vsTaskViewer -c /path/to/config.toml -t /path/to/html
  vsTaskViewer -c /path/to/config.toml -d /var/vsTaskViewer
  vsTaskViewer -c /path/to/config.toml -u www-data
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

	// Override task directory if -d flag is set, otherwise use search order
	if *taskDirFlag != "" {
		// Resolve relative paths to absolute
		taskDir, err := filepath.Abs(*taskDirFlag)
		if err != nil {
			log.Fatalf("Failed to resolve task directory path: %v", err)
		}
		config.Server.TaskDir = taskDir
		log.Printf("Using task directory from -d flag: %s", config.Server.TaskDir)
	} else if config.Server.TaskDir == "" {
		taskDir, err := findTaskDir()
		if err != nil {
			log.Fatalf("Failed to find task directory: %v", err)
		}
		config.Server.TaskDir = taskDir
		log.Printf("Using task directory: %s", config.Server.TaskDir)
	} else {
		// TaskDir is set in config, resolve relative paths
		if !filepath.IsAbs(config.Server.TaskDir) {
			taskDir, err := filepath.Abs(config.Server.TaskDir)
			if err != nil {
				log.Fatalf("Failed to resolve task directory path: %v", err)
			}
			config.Server.TaskDir = taskDir
		}
		log.Printf("Using task directory from config: %s", config.Server.TaskDir)
	}

	// Override exec user if -u flag is set, otherwise use search order
	if *execUserFlag != "" {
		config.Server.ExecUser = *execUserFlag
		log.Printf("Using exec user from -u flag: %s", config.Server.ExecUser)
	} else if config.Server.ExecUser == "" {
		config.Server.ExecUser = findExecUser()
		log.Printf("Using exec user: %s", config.Server.ExecUser)
	} else {
		log.Printf("Using exec user from config: %s", config.Server.ExecUser)
	}

	// Override port from config if specified
	if config.Server.Port > 0 {
		*port = config.Server.Port
	}

	// Load TLS files early (before dropping privileges, as they may require elevated rights)
	var tlsKeyData, tlsCertData []byte
	if config.Server.TLSKeyFile != "" && config.Server.TLSCertFile != "" {
		// Validate TLS files exist
		if _, err := os.Stat(config.Server.TLSKeyFile); os.IsNotExist(err) {
			log.Fatalf("TLS key file not found: %s", config.Server.TLSKeyFile)
		}
		if _, err := os.Stat(config.Server.TLSCertFile); os.IsNotExist(err) {
			log.Fatalf("TLS certificate file not found: %s", config.Server.TLSCertFile)
		}
		// Read TLS files into memory (before dropping privileges)
		var err error
		tlsKeyData, err = os.ReadFile(config.Server.TLSKeyFile)
		if err != nil {
			log.Fatalf("Failed to read TLS key file: %v", err)
		}
		tlsCertData, err = os.ReadFile(config.Server.TLSCertFile)
		if err != nil {
			log.Fatalf("Failed to read TLS certificate file: %v", err)
		}
		log.Printf("Loaded TLS files (key: %s, cert: %s)", config.Server.TLSKeyFile, config.Server.TLSCertFile)
	}

	// Drop privileges to exec user (after loading TLS files)
	if err := dropPrivileges(config.Server.ExecUser); err != nil {
		log.Fatalf("Failed to drop privileges: %v", err)
	}

	// Validate task directory (check/create, ownership, permissions) - now as exec user
	if err := validateTaskDir(config.Server.TaskDir); err != nil {
		log.Fatalf("Task directory validation failed: %v", err)
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
	if len(tlsKeyData) > 0 && len(tlsCertData) > 0 {
		// Write TLS data to temporary files (as exec user)
		tmpKeyFile, err := os.CreateTemp("", "vsTaskViewer-key-*.pem")
		if err != nil {
			log.Fatalf("Failed to create temporary TLS key file: %v", err)
		}
		defer os.Remove(tmpKeyFile.Name())
		if _, err := tmpKeyFile.Write(tlsKeyData); err != nil {
			tmpKeyFile.Close()
			log.Fatalf("Failed to write temporary TLS key file: %v", err)
		}
		tmpKeyFile.Close()
		os.Chmod(tmpKeyFile.Name(), 0600)

		tmpCertFile, err := os.CreateTemp("", "vsTaskViewer-cert-*.pem")
		if err != nil {
			log.Fatalf("Failed to create temporary TLS cert file: %v", err)
		}
		defer os.Remove(tmpCertFile.Name())
		if _, err := tmpCertFile.Write(tlsCertData); err != nil {
			tmpCertFile.Close()
			log.Fatalf("Failed to write temporary TLS cert file: %v", err)
		}
		tmpCertFile.Close()
		os.Chmod(tmpCertFile.Name(), 0644)

		log.Printf("Starting HTTPS server on port %d", *port)
		log.Printf("TLS key: %s", config.Server.TLSKeyFile)
		log.Printf("TLS cert: %s", config.Server.TLSCertFile)
		if err := server.ListenAndServeTLS(tmpCertFile.Name(), tmpKeyFile.Name()); err != nil && err != http.ErrServerClosed {
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

	// Validate task configurations including parameters
	for i, task := range config.Tasks {
		if task.Name == "" {
			return nil, fmt.Errorf("task at index %d has no name", i)
		}
		if task.Command == "" {
			return nil, fmt.Errorf("task '%s' has no command", task.Name)
		}

		// Validate parameter definitions
		paramNames := make(map[string]bool)
		for j, param := range task.Parameters {
			if param.Name == "" {
				return nil, fmt.Errorf("task '%s' has parameter at index %d with no name", task.Name, j)
			}
			if param.Type != "int" && param.Type != "string" {
				return nil, fmt.Errorf("task '%s' parameter '%s' has invalid type '%s' (must be 'int' or 'string')", task.Name, param.Name, param.Type)
			}
			// Check for duplicate parameter names
			if paramNames[param.Name] {
				return nil, fmt.Errorf("task '%s' has duplicate parameter name '%s'", task.Name, param.Name)
			}
			paramNames[param.Name] = true
		}
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

// findTaskDir searches for the task directory in the specified order
func findTaskDir() (string, error) {
	// Default to /var/vsTaskViewer
	defaultTaskDir := "/var/vsTaskViewer"
	if _, err := os.Stat(defaultTaskDir); err == nil {
		return defaultTaskDir, nil
	}

	return defaultTaskDir, nil // Return default even if it doesn't exist yet (will be created in validation)
}

// validateTaskDir validates the task directory:
// - Checks if directory exists or can be created
// - Verifies ownership matches process executor
// - Verifies permissions are 700
func validateTaskDir(taskDir string) error {
	// Get current user info
	currentUID := os.Getuid()
	currentGID := os.Getgid()

	// Check if directory exists
	info, err := os.Stat(taskDir)
	if os.IsNotExist(err) {
		// Try to create directory with 0700 permissions
		if err := os.MkdirAll(taskDir, 0700); err != nil {
			return fmt.Errorf("cannot create task directory %s: %w", taskDir, err)
		}
		log.Printf("Created task directory: %s", taskDir)
		// Re-stat to get info about newly created directory
		info, err = os.Stat(taskDir)
		if err != nil {
			return fmt.Errorf("failed to stat newly created directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("cannot access task directory %s: %w", taskDir, err)
	}

	// Check if it's actually a directory
	if !info.IsDir() {
		return fmt.Errorf("task directory path %s exists but is not a directory", taskDir)
	}

	// Check ownership (must match current user)
	sys := info.Sys()
	if sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			if int(stat.Uid) != currentUID {
				return fmt.Errorf("task directory %s is owned by UID %d, but process is running as UID %d", taskDir, stat.Uid, currentUID)
			}
			if int(stat.Gid) != currentGID {
				return fmt.Errorf("task directory %s is owned by GID %d, but process is running as GID %d", taskDir, stat.Gid, currentGID)
			}
		}
	}

	// Check permissions (must be 700)
	mode := info.Mode().Perm()
	expectedMode := os.FileMode(0700)
	if mode != expectedMode {
		return fmt.Errorf("task directory %s has permissions %o, but must be %o (700)", taskDir, mode, expectedMode)
	}

	log.Printf("Task directory validated: %s (UID: %d, GID: %d, Permissions: %o)", taskDir, currentUID, currentGID, mode)
	return nil
}

// findExecUser returns the default exec user
func findExecUser() string {
	return "www-data"
}

// lookupUser looks up a user by name and returns UID and GID
func lookupUser(username string) (uid, gid int, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, 0, fmt.Errorf("user lookup failed: %w", err)
	}

	uidInt, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid UID: %w", err)
	}

	gidInt, err := strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid GID: %w", err)
	}

	return uidInt, gidInt, nil
}

// dropPrivileges drops privileges to the specified user
func dropPrivileges(username string) error {
	// Get current user
	currentUID := os.Getuid()
	if currentUID != 0 {
		// Not running as root, check if we're already the target user
		currentUser, err := user.Current()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		if currentUser.Username == username {
			log.Printf("Already running as user %s (UID: %d)", username, currentUID)
			return nil
		}
		return fmt.Errorf("cannot drop privileges: not running as root (current UID: %d, target user: %s)", currentUID, username)
	}

	// Lookup target user
	uid, gid, err := lookupUser(username)
	if err != nil {
		return err
	}

	// Drop to target GID first
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("failed to set GID to %d: %w", gid, err)
	}

	// Drop to target UID
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("failed to set UID to %d: %w", uid, err)
	}

	// Verify the change
	if os.Getuid() != uid || os.Getgid() != gid {
		return fmt.Errorf("privilege drop verification failed: expected UID/GID %d/%d, got %d/%d", uid, gid, os.Getuid(), os.Getgid())
	}

	log.Printf("Dropped privileges to user %s (UID: %d, GID: %d)", username, uid, gid)
	return nil
}

