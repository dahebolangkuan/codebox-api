// Package config provides configuration management for GoBox.
// Configuration can be loaded from environment variables, files, or set programmatically.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all configuration for GoBox.
type Config struct {
	// API configuration for remote driver
	APIKey    string `json:"api_key" env:"GOBOX_API_KEY"`
	BaseURL   string `json:"base_url" env:"GOBOX_BASE_URL"`
	FactoryID string `json:"factory_id" env:"GOBOX_FACTORY_ID"`

	// Docker configuration
	DockerImage   string `json:"docker_image" env:"GOBOX_DOCKER_IMAGE"`
	DockerTimeout int    `json:"docker_timeout" env:"GOBOX_DOCKER_TIMEOUT"`
	DockerNetwork string `json:"docker_network" env:"GOBOX_DOCKER_NETWORK"`

	// Pool configuration
	PoolMinSize     int           `json:"pool_min_size" env:"GOBOX_POOL_MIN_SIZE"`
	PoolMaxSize     int           `json:"pool_max_size" env:"GOBOX_POOL_MAX_SIZE"`
	PoolIdleTimeout time.Duration `json:"pool_idle_timeout" env:"GOBOX_POOL_IDLE_TIMEOUT"`
	PoolWarmupCount int           `json:"pool_warmup_count" env:"GOBOX_POOL_WARMUP_COUNT"`

	// Execution configuration
	DefaultTimeout time.Duration `json:"default_timeout" env:"GOBOX_DEFAULT_TIMEOUT"`
	MaxFileSize    int64         `json:"max_file_size" env:"GOBOX_MAX_FILE_SIZE"`

	// Resource limits
	MemoryLimit int64 `json:"memory_limit" env:"GOBOX_MEMORY_LIMIT"`
	CPUQuota    int64 `json:"cpu_quota" env:"GOBOX_CPU_QUOTA"`

	// Server configuration
	ServerPort         int           `json:"server_port" env:"GOBOX_SERVER_PORT"`
	ServerReadTimeout  time.Duration `json:"server_read_timeout" env:"GOBOX_SERVER_READ_TIMEOUT"`
	ServerWriteTimeout time.Duration `json:"server_write_timeout" env:"GOBOX_SERVER_WRITE_TIMEOUT"`
	ServerIdleTimeout  time.Duration `json:"server_idle_timeout" env:"GOBOX_SERVER_IDLE_TIMEOUT"`

	// Security configuration
	EnableAuth     bool   `json:"enable_auth" env:"GOBOX_ENABLE_AUTH"`
	AuthToken      string `json:"auth_token" env:"GOBOX_AUTH_TOKEN"`
	RateLimitRPS   int    `json:"rate_limit_rps" env:"GOBOX_RATE_LIMIT_RPS"`
	RateLimitBurst int    `json:"rate_limit_burst" env:"GOBOX_RATE_LIMIT_BURST"`

	// Logging configuration
	LogLevel  string `json:"log_level" env:"GOBOX_LOG_LEVEL"`
	LogFormat string `json:"log_format" env:"GOBOX_LOG_FORMAT"`
}

// Default values
const (
	DefaultBaseURL        = "https://codeboxapi.com/api/v2"
	DefaultFactoryID      = "default"
	DefaultDockerImage    = "gobox-executor:latest"
	DefaultDockerTimeout  = 15
	DefaultDockerNetwork  = "bridge"
	DefaultPoolMinSize    = 2
	DefaultPoolMaxSize    = 10
	DefaultPoolWarmup     = 2
	DefaultTimeout        = 60 * time.Second
	DefaultMaxFileSize    = 100 * 1024 * 1024 // 100MB
	DefaultMemoryLimit    = 512 * 1024 * 1024 // 512MB
	DefaultCPUQuota       = 100000            // 1 CPU
	DefaultServerPort     = 8069
	DefaultReadTimeout    = 30 * time.Second
	DefaultWriteTimeout   = 60 * time.Second
	DefaultIdleTimeout    = 120 * time.Second
	DefaultRateLimitRPS   = 100
	DefaultRateLimitBurst = 200
	DefaultLogLevel       = "info"
	DefaultLogFormat      = "json"
)

// Load loads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		APIKey:             getEnv("GOBOX_API_KEY", ""),
		BaseURL:            getEnv("GOBOX_BASE_URL", DefaultBaseURL),
		FactoryID:          getEnv("GOBOX_FACTORY_ID", DefaultFactoryID),
		DockerImage:        getEnv("GOBOX_DOCKER_IMAGE", DefaultDockerImage),
		DockerTimeout:      getEnvInt("GOBOX_DOCKER_TIMEOUT", DefaultDockerTimeout),
		DockerNetwork:      getEnv("GOBOX_DOCKER_NETWORK", DefaultDockerNetwork),
		PoolMinSize:        getEnvInt("GOBOX_POOL_MIN_SIZE", DefaultPoolMinSize),
		PoolMaxSize:        getEnvInt("GOBOX_POOL_MAX_SIZE", DefaultPoolMaxSize),
		PoolIdleTimeout:    getEnvDuration("GOBOX_POOL_IDLE_TIMEOUT", 5*time.Minute),
		PoolWarmupCount:    getEnvInt("GOBOX_POOL_WARMUP_COUNT", DefaultPoolWarmup),
		DefaultTimeout:     getEnvDuration("GOBOX_DEFAULT_TIMEOUT", DefaultTimeout),
		MaxFileSize:        getEnvInt64("GOBOX_MAX_FILE_SIZE", DefaultMaxFileSize),
		MemoryLimit:        getEnvInt64("GOBOX_MEMORY_LIMIT", DefaultMemoryLimit),
		CPUQuota:           getEnvInt64("GOBOX_CPU_QUOTA", DefaultCPUQuota),
		ServerPort:         getEnvInt("GOBOX_SERVER_PORT", DefaultServerPort),
		ServerReadTimeout:  getEnvDuration("GOBOX_SERVER_READ_TIMEOUT", DefaultReadTimeout),
		ServerWriteTimeout: getEnvDuration("GOBOX_SERVER_WRITE_TIMEOUT", DefaultWriteTimeout),
		ServerIdleTimeout:  getEnvDuration("GOBOX_SERVER_IDLE_TIMEOUT", DefaultIdleTimeout),
		EnableAuth:         getEnvBool("GOBOX_ENABLE_AUTH", false),
		AuthToken:          getEnv("GOBOX_AUTH_TOKEN", ""),
		RateLimitRPS:       getEnvInt("GOBOX_RATE_LIMIT_RPS", DefaultRateLimitRPS),
		RateLimitBurst:     getEnvInt("GOBOX_RATE_LIMIT_BURST", DefaultRateLimitBurst),
		LogLevel:           getEnv("GOBOX_LOG_LEVEL", DefaultLogLevel),
		LogFormat:          getEnv("GOBOX_LOG_FORMAT", DefaultLogFormat),
	}
}

// LoadFromFile loads configuration from a JSON file.
// Environment variables take precedence over file values.
func LoadFromFile(path string) (*Config, error) {
	config := Load() // Start with env/defaults

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil // File doesn't exist, use defaults
		}
		return nil, err
	}

	// Parse JSON and merge with existing config
	var fileConfig Config
	if err := json.Unmarshal(data, &fileConfig); err != nil {
		return nil, err
	}

	// Merge: env vars take precedence
	config.mergeFrom(&fileConfig)

	return config, nil
}

// mergeFrom merges values from another config (only if current value is zero/empty).
func (c *Config) mergeFrom(other *Config) {
	if c.APIKey == "" && other.APIKey != "" {
		c.APIKey = other.APIKey
	}
	if c.BaseURL == DefaultBaseURL && other.BaseURL != "" {
		c.BaseURL = other.BaseURL
	}
	if c.FactoryID == DefaultFactoryID && other.FactoryID != "" {
		c.FactoryID = other.FactoryID
	}
	if c.DockerImage == DefaultDockerImage && other.DockerImage != "" {
		c.DockerImage = other.DockerImage
	}
	// Add more merge logic as needed...
}

// SaveToFile saves the configuration to a JSON file.
func (c *Config) SaveToFile(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.PoolMinSize < 0 {
		return &ConfigError{Field: "pool_min_size", Message: "must be non-negative"}
	}
	if c.PoolMaxSize < c.PoolMinSize {
		return &ConfigError{Field: "pool_max_size", Message: "must be >= pool_min_size"}
	}
	if c.DefaultTimeout <= 0 {
		return &ConfigError{Field: "default_timeout", Message: "must be positive"}
	}
	if c.MaxFileSize <= 0 {
		return &ConfigError{Field: "max_file_size", Message: "must be positive"}
	}
	if c.ServerPort < 0 || c.ServerPort > 65535 {
		return &ConfigError{Field: "server_port", Message: "must be between 0 and 65535"}
	}
	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config error: " + e.Field + ": " + e.Message
}

// Helper functions for reading environment variables

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		// Try parsing as duration string first (e.g., "30s", "5m")
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
		// Try parsing as seconds
		if secs, err := strconv.Atoi(value); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
