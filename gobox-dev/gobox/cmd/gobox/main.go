// GoBox CLI - A command-line interface for code execution in sandboxed environments
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"gobox/pkg/config"
	"gobox/pkg/docker"
	"gobox/pkg/gobox"
	"gobox/pkg/remote"
)

var (
	// CLI version
	version = "1.0.0"

	// Global flags
	cfgDriver    string
	cfgImage     string
	cfgAPIKey    string
	cfgBaseURL   string
	cfgFactoryID string
	cfgTimeout   int
	cfgVerbose   bool
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// rootCmd is the root command
var rootCmd = &cobra.Command{
	Use:   "gobox",
	Short: "GoBox - Sandboxed code execution for LLM applications",
	Long: `GoBox is a high-performance sandbox code execution orchestrator.
It provides secure, isolated environments for running code.

Examples:
  gobox exec "print('Hello, World!')"
  gobox exec -f script.py
  gobox repl
  echo "print('Hello')" | gobox exec -`,
	Version: version,
}

// execCmd executes code
var execCmd = &cobra.Command{
	Use:   "exec [code]",
	Short: "Execute code in the sandbox",
	Long: `Execute Python or Bash code in a sandboxed environment.

The code can be provided as:
  - A command-line argument
  - Via stdin (use - as the argument)
  - From a file (use -f flag)

Examples:
  gobox exec "print('Hello')"
  gobox exec -k bash "echo Hello"
  gobox exec -f script.py
  echo "print('test')" | gobox exec -`,
	RunE: runExec,
}

// replCmd starts an interactive REPL
var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Start an interactive REPL session",
	Long: `Start an interactive Read-Eval-Print Loop (REPL) session.
Type code and press Enter to execute. Use Ctrl+D or type 'exit' to quit.`,
	RunE: runRepl,
}

// uploadCmd uploads files
var uploadCmd = &cobra.Command{
	Use:   "upload [files...]",
	Short: "Upload files to the sandbox",
	Long: `Upload one or more files to the sandbox environment.

Examples:
  gobox upload data.csv
  gobox upload file1.txt file2.txt`,
	Args: cobra.MinimumNArgs(1),
	RunE: runUpload,
}

// downloadCmd downloads files
var downloadCmd = &cobra.Command{
	Use:   "download [filename]",
	Short: "Download a file from the sandbox",
	Long: `Download a file from the sandbox environment to the current directory.

Examples:
  gobox download output.csv
  gobox download result.png -o my_result.png`,
	Args: cobra.ExactArgs(1),
	RunE: runDownload,
}

// listCmd lists files
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List files in the sandbox",
	Long:  `List all files in the sandbox working directory.`,
	RunE:  runList,
}

// installCmd installs packages
var installCmd = &cobra.Command{
	Use:   "install [packages...]",
	Short: "Install Python packages",
	Long: `Install one or more Python packages using pip.

Examples:
  gobox install numpy pandas
  gobox install -r requirements.txt`,
	Args: cobra.MinimumNArgs(1),
	RunE: runInstall,
}

// healthCmd checks sandbox health
var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check sandbox health status",
	RunE:  runHealth,
}

// versionCmd shows version
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("GoBox version %s\n", version)
	},
}

// Exec-specific flags
var (
	execKernel  string
	execFile    string
	execOutput  string
	execTimeout int
)

// Download-specific flags
var downloadOutput string

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgDriver, "driver", "d", "docker", "Driver to use (docker, remote)")
	rootCmd.PersistentFlags().StringVar(&cfgImage, "image", "", "Docker image to use")
	rootCmd.PersistentFlags().StringVar(&cfgAPIKey, "api-key", "", "API key for remote driver")
	rootCmd.PersistentFlags().StringVar(&cfgBaseURL, "base-url", "", "Base URL for remote driver")
	rootCmd.PersistentFlags().StringVar(&cfgFactoryID, "factory-id", "", "Factory ID for remote driver")
	rootCmd.PersistentFlags().IntVarP(&cfgTimeout, "timeout", "t", 60, "Default timeout in seconds")
	rootCmd.PersistentFlags().BoolVarP(&cfgVerbose, "verbose", "v", false, "Enable verbose output")

	// Exec flags
	execCmd.Flags().StringVarP(&execKernel, "kernel", "k", "python", "Execution kernel (python, bash)")
	execCmd.Flags().StringVarP(&execFile, "file", "f", "", "Read code from file")
	execCmd.Flags().IntVar(&execTimeout, "exec-timeout", 60, "Execution timeout in seconds")

	// Download flags
	downloadCmd.Flags().StringVarP(&downloadOutput, "output", "o", "", "Output filename")

	// Add commands
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(replCmd)
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(versionCmd)
}

// createSandbox creates a sandbox based on configuration.
func createSandbox() (gobox.Sandbox, error) {
	cfg := config.Load()

	// Override with CLI flags
	if cfgAPIKey != "" {
		cfg.APIKey = cfgAPIKey
	}
	if cfgBaseURL != "" {
		cfg.BaseURL = cfgBaseURL
	}
	if cfgImage != "" {
		cfg.DockerImage = cfgImage
	}

	switch cfgDriver {
	case "docker":
		opts := []docker.Option{
			docker.WithTimeout(time.Duration(cfgTimeout) * time.Second),
		}
		if cfg.DockerImage != "" {
			opts = append(opts, docker.WithImage(cfg.DockerImage))
		}
		return docker.New(opts...)

	case "remote":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key required for remote driver (set GOBOX_API_KEY or use --api-key)")
		}
		return remote.New(&remote.Config{
			BaseURL:   cfg.BaseURL,
			APIKey:    cfg.APIKey,
			FactoryID: cfgFactoryID,
			Timeout:   time.Duration(cfgTimeout) * time.Second,
		})

	default:
		return nil, fmt.Errorf("unknown driver: %s", cfgDriver)
	}
}

func runExec(cmd *cobra.Command, args []string) error {
	// Get code from args, file, or stdin
	var code string

	if execFile != "" {
		data, err := os.ReadFile(execFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		code = string(data)
	} else if len(args) > 0 {
		if args[0] == "-" {
			// Read from stdin
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			code = string(data)
		} else {
			code = args[0]
		}
	} else {
		return fmt.Errorf("no code provided")
	}

	// Create sandbox
	sandbox, err := createSandbox()
	if err != nil {
		return err
	}
	defer sandbox.Close()

	// Set up context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(execTimeout)*time.Second)
	defer cancel()

	// Handle interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Execute with streaming output
	chunkChan, errChan := sandbox.StreamExec(ctx, code, gobox.WithKernel(execKernel))

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				// Check for errors
				select {
				case err := <-errChan:
					if err != nil {
						return err
					}
				default:
				}
				return nil
			}

			switch chunk.Type {
			case gobox.ChunkTypeText:
				fmt.Print(chunk.Content)
			case gobox.ChunkTypeError:
				fmt.Fprintf(os.Stderr, "%s", chunk.Content)
			case gobox.ChunkTypeImage:
				if cfgVerbose {
					fmt.Printf("[Image: %d bytes]\n", len(chunk.Content))
				}
			}

		case err := <-errChan:
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func runRepl(cmd *cobra.Command, args []string) error {
	sandbox, err := createSandbox()
	if err != nil {
		return err
	}
	defer sandbox.Close()

	fmt.Println("GoBox REPL - Type 'exit' or Ctrl+D to quit")
	fmt.Println("-------------------------------------------")

	reader := bufio.NewReader(os.Stdin)
	var multiline strings.Builder
	inMultiline := false

	for {
		// Print prompt
		if inMultiline {
			fmt.Print("... ")
		} else {
			fmt.Print(">>> ")
		}

		// Read line
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			fmt.Println()
			return nil
		}
		if err != nil {
			return err
		}

		line = strings.TrimRight(line, "\n\r")

		// Check for exit
		if !inMultiline && (line == "exit" || line == "quit") {
			return nil
		}

		// Handle multiline input (lines ending with :)
		if strings.HasSuffix(strings.TrimSpace(line), ":") {
			inMultiline = true
			multiline.WriteString(line + "\n")
			continue
		}

		if inMultiline {
			if line == "" {
				// Empty line ends multiline input
				code := multiline.String()
				multiline.Reset()
				inMultiline = false

				if err := executeAndPrint(sandbox, code); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				}
			} else {
				multiline.WriteString(line + "\n")
			}
			continue
		}

		// Single line execution
		if line != "" {
			if err := executeAndPrint(sandbox, line); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	}
}

func executeAndPrint(sandbox gobox.Sandbox, code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfgTimeout)*time.Second)
	defer cancel()

	result, err := sandbox.Exec(ctx, code, gobox.WithKernel(execKernel))
	if err != nil {
		return err
	}

	// Print output
	fmt.Print(result.Text())

	// Print errors
	for _, errMsg := range result.Errors() {
		fmt.Fprintf(os.Stderr, "%s", errMsg)
	}

	// Print image count
	images := result.Images()
	if len(images) > 0 && cfgVerbose {
		fmt.Printf("[%d image(s) generated]\n", len(images))
	}

	return nil
}

func runUpload(cmd *cobra.Command, args []string) error {
	sandbox, err := createSandbox()
	if err != nil {
		return err
	}
	defer sandbox.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, filename := range args {
		file, err := os.Open(filename)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", filename, err)
		}

		goFile := gobox.File{
			Name:    filename,
			Content: file,
		}

		if err := sandbox.Upload(ctx, goFile); err != nil {
			file.Close()
			return fmt.Errorf("failed to upload %s: %w", filename, err)
		}

		file.Close()
		fmt.Printf("Uploaded: %s\n", filename)
	}

	return nil
}

func runDownload(cmd *cobra.Command, args []string) error {
	sandbox, err := createSandbox()
	if err != nil {
		return err
	}
	defer sandbox.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	filename := args[0]
	outputName := downloadOutput
	if outputName == "" {
		outputName = filename
	}

	reader, err := sandbox.Download(ctx, filename)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", filename, err)
	}
	defer reader.Close()

	outFile, err := os.Create(outputName)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", outputName, err)
	}
	defer outFile.Close()

	n, err := io.Copy(outFile, reader)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", outputName, err)
	}

	fmt.Printf("Downloaded: %s (%d bytes)\n", outputName, n)
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	sandbox, err := createSandbox()
	if err != nil {
		return err
	}
	defer sandbox.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	files, err := sandbox.ListFiles(ctx)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No files")
		return nil
	}

	for _, f := range files {
		typeChar := "-"
		if f.IsDir {
			typeChar = "d"
		}
		fmt.Printf("%s %10d %s\n", typeChar, f.Size, f.Path)
	}

	return nil
}

func runInstall(cmd *cobra.Command, args []string) error {
	sandbox, err := createSandbox()
	if err != nil {
		return err
	}
	defer sandbox.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Printf("Installing: %s\n", strings.Join(args, " "))

	if err := sandbox.Install(ctx, args...); err != nil {
		return err
	}

	fmt.Println("Installation complete")
	return nil
}

func runHealth(cmd *cobra.Command, args []string) error {
	sandbox, err := createSandbox()
	if err != nil {
		return err
	}
	defer sandbox.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := sandbox.HealthCheck(ctx); err != nil {
		fmt.Println("Status: UNHEALTHY")
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Status: HEALTHY")
	return nil
}
