package gobox

import (
	"context"
	"io"
)

// Sandbox defines the standard interface for a sandbox execution environment.
// A sandbox provides a secure, isolated environment for running code.
//
// All operations are context-aware and support cancellation and timeouts.
// Implementations must be safe for concurrent use from multiple goroutines.
type Sandbox interface {
	// Exec executes code and returns the complete result.
	// This method blocks until execution completes or the context is cancelled.
	//
	// Example:
	//   result, err := sandbox.Exec(ctx, "print('Hello, World!')")
	//   if err != nil {
	//       return err
	//   }
	//   fmt.Println(result.Text())
	Exec(ctx context.Context, code string, opts ...ExecOption) (*ExecResult, error)

	// StreamExec executes code and returns streaming output.
	// Output is delivered as it's produced via the chunk channel.
	// The error channel will receive any errors that occur during execution.
	// Both channels are closed when execution completes.
	//
	// Example:
	//   chunkCh, errCh := sandbox.StreamExec(ctx, "for i in range(10): print(i)")
	//   for chunk := range chunkCh {
	//       fmt.Print(chunk.Content)
	//   }
	//   if err := <-errCh; err != nil {
	//       return err
	//   }
	StreamExec(ctx context.Context, code string, opts ...ExecOption) (<-chan ExecChunk, <-chan error)

	// Upload uploads one or more files to the sandbox environment.
	// Files are placed in the sandbox's working directory by default.
	//
	// Example:
	//   file := gobox.File{Name: "data.csv", Content: strings.NewReader("a,b,c\n1,2,3")}
	//   if err := sandbox.Upload(ctx, file); err != nil {
	//       return err
	//   }
	Upload(ctx context.Context, files ...File) error

	// Download retrieves a file from the sandbox environment.
	// The caller is responsible for closing the returned ReadCloser.
	//
	// Example:
	//   rc, err := sandbox.Download(ctx, "output.csv")
	//   if err != nil {
	//       return err
	//   }
	//   defer rc.Close()
	//   data, _ := io.ReadAll(rc)
	Download(ctx context.Context, filename string) (io.ReadCloser, error)

	// ListFiles lists files in the sandbox environment.
	// By default, this lists files in the working directory.
	//
	// Example:
	//   files, err := sandbox.ListFiles(ctx)
	//   for _, f := range files {
	//       fmt.Printf("%s: %d bytes\n", f.Path, f.Size)
	//   }
	ListFiles(ctx context.Context) ([]FileInfo, error)

	// Install installs Python packages into the sandbox environment.
	// This uses pip under the hood.
	//
	// Example:
	//   if err := sandbox.Install(ctx, "numpy", "pandas"); err != nil {
	//       return err
	//   }
	Install(ctx context.Context, packages ...string) error

	// HealthCheck verifies that the sandbox is healthy and ready for use.
	// Returns nil if healthy, error otherwise.
	HealthCheck(ctx context.Context) error

	// Restart restarts the sandbox execution environment.
	// This clears all state including files and installed packages.
	Restart(ctx context.Context) error

	// Close releases all resources associated with the sandbox.
	// After Close is called, the sandbox should not be used.
	Close() error
}

// Driver represents a specific implementation of the Sandbox interface.
// This is an alias for Sandbox but emphasizes the implementation aspect.
type Driver = Sandbox

// DriverFactory creates sandbox instances.
// Factories can be registered and looked up by name.
type DriverFactory interface {
	// Name returns the driver name (e.g., "docker", "remote", "k8s").
	Name() string

	// Create creates a new sandbox instance with the given configuration.
	Create(config *SandboxConfig) (Sandbox, error)
}

// registry holds registered driver factories.
var registry = make(map[string]DriverFactory)

// RegisterDriver registers a driver factory.
// This should be called from driver init() functions.
func RegisterDriver(factory DriverFactory) {
	registry[factory.Name()] = factory
}

// GetDriver retrieves a registered driver factory by name.
func GetDriver(name string) (DriverFactory, bool) {
	factory, ok := registry[name]
	return factory, ok
}

// ListDrivers returns the names of all registered drivers.
func ListDrivers() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// New creates a new sandbox with the given options.
// This is the primary entry point for creating sandboxes.
//
// Example:
//
//	sandbox, err := gobox.New(
//	    gobox.WithDriver("docker"),
//	    gobox.WithImage("shroominic/codebox:latest"),
//	)
//	if err != nil {
//	    return err
//	}
//	defer sandbox.Close()
func New(opts ...SandboxOption) (Sandbox, error) {
	config := ApplySandboxOptions(opts...)

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Look up driver factory
	factory, ok := GetDriver(config.Driver)
	if !ok {
		return nil, NewExecError("config", "driver not found: "+config.Driver)
	}

	// Create sandbox
	return factory.Create(config)
}

// ExecSimple is a convenience function for simple code execution.
// It creates a temporary sandbox, executes the code, and returns the result.
//
// Example:
//
//	result, err := gobox.ExecSimple(ctx, "print('Hello!')", gobox.WithDriver("docker"))
func ExecSimple(ctx context.Context, code string, opts ...SandboxOption) (*ExecResult, error) {
	sandbox, err := New(opts...)
	if err != nil {
		return nil, err
	}
	defer sandbox.Close()

	return sandbox.Exec(ctx, code)
}
