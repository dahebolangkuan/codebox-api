// GoBox Server - HTTP API server for sandboxed code execution
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"gobox/pkg/config"
	"gobox/pkg/docker"
	"gobox/pkg/gobox"
	"gobox/pkg/server"
)

var (
	version = "1.0.0"

	// Server flags
	port         int
	host         string
	enableAuth   bool
	authToken    string
	enableCORS   bool
	readTimeout  int
	writeTimeout int

	// Sandbox flags
	driver   string
	image    string
	poolMin  int
	poolMax  int
	timeout  int
	memory   int64
	cpuQuota int64
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gobox-server",
	Short: "GoBox API Server - Sandboxed code execution service",
	Long: `GoBox Server provides an HTTP API for executing code in sandboxed environments.

It supports:
  - Streaming code execution
  - File upload and download
  - Package installation
  - Health monitoring

Examples:
  gobox-server                    # Start with default settings
  gobox-server --port 8080        # Start on port 8080
  gobox-server --driver docker    # Use Docker driver`,
	Version: version,
	RunE:    runServer,
}

func init() {
	// Server flags
	rootCmd.Flags().IntVarP(&port, "port", "p", 8069, "Server port")
	rootCmd.Flags().StringVar(&host, "host", "0.0.0.0", "Server host")
	rootCmd.Flags().BoolVar(&enableAuth, "auth", false, "Enable authentication")
	rootCmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token")
	rootCmd.Flags().BoolVar(&enableCORS, "cors", true, "Enable CORS")
	rootCmd.Flags().IntVar(&readTimeout, "read-timeout", 30, "Read timeout in seconds")
	rootCmd.Flags().IntVar(&writeTimeout, "write-timeout", 60, "Write timeout in seconds")

	// Sandbox flags
	rootCmd.Flags().StringVarP(&driver, "driver", "d", "docker", "Sandbox driver (docker)")
	rootCmd.Flags().StringVar(&image, "image", "", "Docker image to use")
	rootCmd.Flags().IntVar(&poolMin, "pool-min", 2, "Minimum container pool size")
	rootCmd.Flags().IntVar(&poolMax, "pool-max", 10, "Maximum container pool size")
	rootCmd.Flags().IntVarP(&timeout, "timeout", "t", 60, "Default execution timeout in seconds")
	rootCmd.Flags().Int64Var(&memory, "memory", 512*1024*1024, "Memory limit in bytes")
	rootCmd.Flags().Int64Var(&cpuQuota, "cpu-quota", 100000, "CPU quota")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("Starting GoBox Server",
		zap.String("version", version),
		zap.Int("port", port),
		zap.String("driver", driver),
	)

	// Load configuration
	cfg := config.Load()

	// Override with CLI flags
	if image != "" {
		cfg.DockerImage = image
	}
	cfg.PoolMinSize = poolMin
	cfg.PoolMaxSize = poolMax
	cfg.DefaultTimeout = time.Duration(timeout) * time.Second
	cfg.MemoryLimit = memory
	cfg.CPUQuota = cpuQuota
	cfg.ServerPort = port

	// Create sandbox
	sandbox, err := createSandbox(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer sandbox.Close()

	// Create server
	serverConfig := &server.Config{
		Port:           port,
		Host:           host,
		ReadTimeout:    time.Duration(readTimeout) * time.Second,
		WriteTimeout:   time.Duration(writeTimeout) * time.Second,
		IdleTimeout:    120 * time.Second,
		EnableAuth:     enableAuth,
		AuthToken:      authToken,
		EnableCORS:     enableCORS,
		AllowedOrigins: []string{"*"},
	}

	srv := server.New(sandbox, serverConfig, logger)

	// Run server (blocks until shutdown)
	return srv.Run()
}

func createSandbox(cfg *config.Config, logger *zap.Logger) (gobox.Sandbox, error) {
	switch driver {
	case "docker":
		logger.Info("Creating Docker sandbox",
			zap.String("image", cfg.DockerImage),
			zap.Int("pool_min", cfg.PoolMinSize),
			zap.Int("pool_max", cfg.PoolMaxSize),
		)

		return docker.New(
			docker.WithImage(cfg.DockerImage),
			docker.WithTimeout(cfg.DefaultTimeout),
			docker.WithMemory(cfg.MemoryLimit),
			docker.WithCPUQuota(cfg.CPUQuota),
			docker.WithPoolSize(cfg.PoolMinSize, cfg.PoolMaxSize),
		)

	default:
		return nil, fmt.Errorf("unknown driver: %s (only 'docker' is supported for server)", driver)
	}
}
