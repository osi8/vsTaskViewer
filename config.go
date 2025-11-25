package main

// Config represents the application configuration
type Config struct {
	Server ServerConfig `toml:"server"`
	Auth   AuthConfig   `toml:"auth"`
	Tasks  []TaskConfig `toml:"tasks"`
}

// ServerConfig contains server settings
type ServerConfig struct {
	Port int `toml:"port"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Secret string `toml:"secret"`
}

// TaskConfig defines a task that can be executed
type TaskConfig struct {
	Name        string `toml:"name"`
	Command     string `toml:"command"`
	Description string `toml:"description"`
}

