package main

// Config represents the application configuration
type Config struct {
	Server ServerConfig `toml:"server"`
	Auth   AuthConfig   `toml:"auth"`
	Tasks  []TaskConfig `toml:"tasks"`
}

// ServerConfig contains server settings
type ServerConfig struct {
	Port            int      `toml:"port"`
	HTMLDir         string   `toml:"html_dir"`
	TaskDir         string   `toml:"task_dir"`         // Path to task output directory
	AllowedOrigins  []string `toml:"allowed_origins"` // For WebSocket CORS
	RateLimitRPM    int      `toml:"rate_limit_rpm"`  // Requests per minute per IP (0 = disabled)
	MaxRequestSize  int64    `toml:"max_request_size"` // Max request body size in bytes (0 = default 10MB)
	TLSKeyFile      string   `toml:"tls_key_file"`     // Path to TLS private key file
	TLSCertFile     string   `toml:"tls_cert_file"`    // Path to TLS certificate file (fullchain)
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Secret string `toml:"secret"`
}

// TaskConfig defines a task that can be executed
type TaskConfig struct {
	Name            string           `toml:"name"`
	Command         string           `toml:"command"`
	Description     string           `toml:"description"`
	MaxExecutionTime int             `toml:"max_execution_time"` // Maximum execution time in seconds (0 = no limit)
	Parameters      []ParameterConfig `toml:"parameters"`        // Parameter definitions for the task
}

// ParameterConfig defines a parameter for a task
type ParameterConfig struct {
	Name     string `toml:"name"`     // Parameter name
	Type     string `toml:"type"`     // Parameter type: "int" or "string"
	Optional bool   `toml:"optional"` // Whether the parameter is optional
}

