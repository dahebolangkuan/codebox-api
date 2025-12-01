package gobox

import (
	"time"
)

// ExecConfig holds the configuration for a single code execution.
// It is built using functional options (ExecOption).
type ExecConfig struct {
	// Kernel specifies the execution kernel: "python" or "bash".
	// Default is "python".
	Kernel string
	// Timeout specifies the maximum duration for execution.
	// If not set, the driver's default timeout is used.
	Timeout time.Duration
	// Cwd specifies the working directory for execution.
	// If empty, the default working directory is used.
	Cwd string
	// Env contains additional environment variables for execution.
	Env map[string]string
}

// DefaultExecConfig returns an ExecConfig with default values.
func DefaultExecConfig() *ExecConfig {
	return &ExecConfig{
		Kernel:  "python",
		Timeout: 60 * time.Second,
		Cwd:     "",
		Env:     make(map[string]string),
	}
}

// ExecOption is a functional option for configuring code execution.
type ExecOption func(*ExecConfig)

// WithKernel sets the execution kernel.
// Supported kernels: "python", "bash".
func WithKernel(kernel string) ExecOption {
	return func(c *ExecConfig) {
		c.Kernel = kernel
	}
}

// WithTimeout sets the execution timeout.
func WithTimeout(d time.Duration) ExecOption {
	return func(c *ExecConfig) {
		c.Timeout = d
	}
}

// WithCwd sets the working directory for execution.
func WithCwd(cwd string) ExecOption {
	return func(c *ExecConfig) {
		c.Cwd = cwd
	}
}

// WithEnv sets additional environment variables for execution.
func WithEnv(env map[string]string) ExecOption {
	return func(c *ExecConfig) {
		c.Env = env
	}
}

// WithEnvVar sets a single environment variable for execution.
func WithEnvVar(key, value string) ExecOption {
	return func(c *ExecConfig) {
		if c.Env == nil {
			c.Env = make(map[string]string)
		}
		c.Env[key] = value
	}
}

// ApplyExecOptions applies the given options to an ExecConfig.
func ApplyExecOptions(opts ...ExecOption) *ExecConfig {
	config := DefaultExecConfig()
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// SandboxConfig holds the configuration for creating a sandbox.
type SandboxConfig struct {
	// Driver specifies which driver to use: "docker", "remote", "k8s".
	Driver string
	// Image specifies the container image to use (for docker/k8s drivers).
	Image string
	// Timeout is the default timeout for operations.
	Timeout time.Duration
	// Memory is the memory limit in bytes (for docker/k8s drivers).
	Memory int64
	// CPUQuota is the CPU quota (for docker/k8s drivers).
	CPUQuota int64
	// NetworkMode specifies the network mode (for docker driver).
	NetworkMode string
	// APIKey is the API key for remote driver authentication.
	APIKey string
	// BaseURL is the base URL for remote driver.
	BaseURL string
	// FactoryID is the factory ID for remote driver.
	FactoryID string
	// SessionID is an optional session ID for remote driver.
	SessionID string
	// PoolConfig holds pool-related configuration.
	PoolConfig *PoolConfig
}

// PoolConfig holds configuration for the container pool.
type PoolConfig struct {
	// MinSize is the minimum number of containers to keep in the pool.
	MinSize int
	// MaxSize is the maximum number of containers allowed in the pool.
	MaxSize int
	// IdleTimeout is how long a container can be idle before being removed.
	IdleTimeout time.Duration
	// WarmupCount is the number of containers to pre-warm on startup.
	WarmupCount int
}

// DefaultSandboxConfig returns a SandboxConfig with default values.
func DefaultSandboxConfig() *SandboxConfig {
	return &SandboxConfig{
		Driver:      "docker",
		Image:       "shroominic/codebox:latest",
		Timeout:     60 * time.Second,
		Memory:      512 * 1024 * 1024, // 512MB
		CPUQuota:    100000,            // 1 CPU
		NetworkMode: "bridge",
		BaseURL:     "https://codeboxapi.com/api/v2",
		PoolConfig: &PoolConfig{
			MinSize:     2,
			MaxSize:     10,
			IdleTimeout: 5 * time.Minute,
			WarmupCount: 2,
		},
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

// SandboxOption is a functional option for configuring a sandbox.
type SandboxOption func(*SandboxConfig)

// WithDriver sets the driver type.
func WithDriver(driver string) SandboxOption {
	return func(c *SandboxConfig) {
		c.Driver = driver
	}
}

// WithImage sets the container image.
func WithImage(image string) SandboxOption {
	return func(c *SandboxConfig) {
		c.Image = image
	}
}

// WithDefaultTimeout sets the default timeout for operations.
func WithDefaultTimeout(d time.Duration) SandboxOption {
	return func(c *SandboxConfig) {
		c.Timeout = d
	}
}

// WithMemory sets the memory limit.
func WithMemory(bytes int64) SandboxOption {
	return func(c *SandboxConfig) {
		c.Memory = bytes
	}
}

// WithCPUQuota sets the CPU quota.
func WithCPUQuota(quota int64) SandboxOption {
	return func(c *SandboxConfig) {
		c.CPUQuota = quota
	}
}

// WithNetworkMode sets the network mode.
func WithNetworkMode(mode string) SandboxOption {
	return func(c *SandboxConfig) {
		c.NetworkMode = mode
	}
}

// WithAPIKey sets the API key for remote driver.
func WithAPIKey(key string) SandboxOption {
	return func(c *SandboxConfig) {
		c.APIKey = key
	}
}

// WithBaseURL sets the base URL for remote driver.
func WithBaseURL(url string) SandboxOption {
	return func(c *SandboxConfig) {
		c.BaseURL = url
	}
}

// WithFactoryID sets the factory ID for remote driver.
func WithFactoryID(id string) SandboxOption {
	return func(c *SandboxConfig) {
		c.FactoryID = id
	}
}

// WithSessionID sets the session ID for remote driver.
func WithSessionID(id string) SandboxOption {
	return func(c *SandboxConfig) {
		c.SessionID = id
	}
}

// WithPoolConfig sets the pool configuration.
func WithPoolConfig(poolConfig *PoolConfig) SandboxOption {
	return func(c *SandboxConfig) {
		c.PoolConfig = poolConfig
	}
}

// WithPoolSize sets both min and max pool size.
func WithPoolSize(min, max int) SandboxOption {
	return func(c *SandboxConfig) {
		if c.PoolConfig == nil {
			c.PoolConfig = DefaultPoolConfig()
		}
		c.PoolConfig.MinSize = min
		c.PoolConfig.MaxSize = max
	}
}

// ApplySandboxOptions applies the given options to a SandboxConfig.
func ApplySandboxOptions(opts ...SandboxOption) *SandboxConfig {
	config := DefaultSandboxConfig()
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// Validate checks if the SandboxConfig is valid.
func (c *SandboxConfig) Validate() error {
	if c.Driver != "docker" && c.Driver != "remote" && c.Driver != "k8s" {
		return NewExecError("config", "invalid driver: must be 'docker', 'remote', or 'k8s'")
	}

	if c.Driver == "docker" || c.Driver == "k8s" {
		if c.Image == "" {
			return NewExecError("config", "image is required for docker/k8s driver")
		}
	}

	if c.Driver == "remote" {
		if c.APIKey == "" {
			return NewExecError("config", "api_key is required for remote driver")
		}
		if c.BaseURL == "" {
			return NewExecError("config", "base_url is required for remote driver")
		}
	}

	if c.PoolConfig != nil {
		if c.PoolConfig.MinSize < 0 {
			return NewExecError("config", "pool min_size must be non-negative")
		}
		if c.PoolConfig.MaxSize < c.PoolConfig.MinSize {
			return NewExecError("config", "pool max_size must be >= min_size")
		}
	}

	return nil
}
