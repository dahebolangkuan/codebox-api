// Package docker provides the Docker-based driver for GoBox.
// This driver manages code execution within Docker containers.
package docker

import (
	"time"
)

// Config holds configuration for the Docker driver.
type Config struct {
	// Image is the Docker image to use for containers.
	Image string
	// Timeout is the default timeout for operations.
	Timeout time.Duration
	// Memory is the memory limit in bytes.
	Memory int64
	// CPUQuota is the CPU quota (e.g., 100000 = 1 CPU).
	CPUQuota int64
	// NetworkMode is the Docker network mode.
	NetworkMode string
	// Privileged indicates if containers should run in privileged mode.
	Privileged bool
	// AutoRemove indicates if containers should be removed after stopping.
	AutoRemove bool
	// WorkDir is the working directory inside the container.
	WorkDir string
	// Env contains environment variables to set in containers.
	Env map[string]string
	// Labels contains labels to apply to containers.
	Labels map[string]string
	// PoolConfig holds pool-related configuration.
	PoolConfig *PoolConfig
}

// PoolConfig holds configuration for the container pool.
type PoolConfig struct {
	// MinSize is the minimum number of containers to keep ready.
	MinSize int
	// MaxSize is the maximum number of containers allowed.
	MaxSize int
	// IdleTimeout is how long a container can be idle before being destroyed.
	IdleTimeout time.Duration
	// WarmupCount is the number of containers to pre-warm on startup.
	WarmupCount int
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Image:       "shroominic/codebox:latest",
		Timeout:     60 * time.Second,
		Memory:      512 * 1024 * 1024, // 512MB
		CPUQuota:    100000,            // 1 CPU
		NetworkMode: "bridge",
		Privileged:  false,
		AutoRemove:  true,
		WorkDir:     "/app",
		Env:         make(map[string]string),
		Labels: map[string]string{
			"app": "gobox",
		},
		PoolConfig: DefaultPoolConfig(),
	}
}

// DefaultPoolConfig returns a PoolConfig with default values.
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MinSize:     2,
		MaxSize:     10,
		IdleTimeout: 5 * time.Minute,
		WarmupCount: 2,
	}
}

// Option is a functional option for configuring the Docker driver.
type Option func(*Config)

// WithImage sets the Docker image.
func WithImage(image string) Option {
	return func(c *Config) {
		c.Image = image
	}
}

// WithTimeout sets the default timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

// WithMemory sets the memory limit.
func WithMemory(bytes int64) Option {
	return func(c *Config) {
		c.Memory = bytes
	}
}

// WithCPUQuota sets the CPU quota.
func WithCPUQuota(quota int64) Option {
	return func(c *Config) {
		c.CPUQuota = quota
	}
}

// WithNetworkMode sets the network mode.
func WithNetworkMode(mode string) Option {
	return func(c *Config) {
		c.NetworkMode = mode
	}
}

// WithPrivileged sets privileged mode.
func WithPrivileged(privileged bool) Option {
	return func(c *Config) {
		c.Privileged = privileged
	}
}

// WithAutoRemove sets auto-remove mode.
func WithAutoRemove(autoRemove bool) Option {
	return func(c *Config) {
		c.AutoRemove = autoRemove
	}
}

// WithWorkDir sets the working directory.
func WithWorkDir(workDir string) Option {
	return func(c *Config) {
		c.WorkDir = workDir
	}
}

// WithEnv sets environment variables.
func WithEnv(env map[string]string) Option {
	return func(c *Config) {
		c.Env = env
	}
}

// WithLabels sets container labels.
func WithLabels(labels map[string]string) Option {
	return func(c *Config) {
		c.Labels = labels
	}
}

// WithPoolConfig sets the pool configuration.
func WithPoolConfig(poolConfig *PoolConfig) Option {
	return func(c *Config) {
		c.PoolConfig = poolConfig
	}
}

// WithPoolSize sets the pool min and max size.
func WithPoolSize(min, max int) Option {
	return func(c *Config) {
		if c.PoolConfig == nil {
			c.PoolConfig = DefaultPoolConfig()
		}
		c.PoolConfig.MinSize = min
		c.PoolConfig.MaxSize = max
	}
}

// ApplyOptions applies the given options to a Config.
func ApplyOptions(opts ...Option) *Config {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	return config
}
