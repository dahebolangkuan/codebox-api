// Streaming example demonstrating real-time output with GoBox
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"gobox/pkg/docker"
	"gobox/pkg/gobox"
)

func main() {
	fmt.Println("GoBox Streaming Example")
	fmt.Println("========================")

	// Create a Docker-based sandbox
	sandbox, err := docker.New(
		docker.WithImage("shroominic/codebox:latest"),
		docker.WithTimeout(120*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create sandbox: %v", err)
	}
	defer sandbox.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Example 1: Streaming progress updates
	fmt.Println("\n1. Streaming Progress Updates:")
	code := `
import time
import sys

for i in range(1, 6):
    print(f"Processing step {i}/5...")
    sys.stdout.flush()
    time.sleep(0.5)

print("Done!")
`

	chunkChan, errChan := sandbox.StreamExec(ctx, code)

	// Process streaming output
	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				// Channel closed, check for errors
				select {
				case err := <-errChan:
					if err != nil {
						log.Printf("Error: %v", err)
					}
				default:
				}
				goto next1
			}

			switch chunk.Type {
			case gobox.ChunkTypeText:
				fmt.Printf("   [TEXT] %s", chunk.Content)
			case gobox.ChunkTypeError:
				fmt.Printf("   [ERROR] %s", chunk.Content)
			case gobox.ChunkTypeImage:
				fmt.Printf("   [IMAGE] (%d bytes)\n", len(chunk.Content))
			}

		case err := <-errChan:
			if err != nil {
				log.Printf("Error: %v", err)
			}
			goto next1

		case <-ctx.Done():
			log.Printf("Timeout: %v", ctx.Err())
			goto next1
		}
	}

next1:
	// Example 2: Long-running computation
	fmt.Println("\n2. Long-running Computation:")
	code = `
import time

def fibonacci(n):
    if n <= 1:
        return n
    return fibonacci(n-1) + fibonacci(n-2)

for n in [10, 15, 20, 25]:
    start = time.time()
    result = fibonacci(n)
    elapsed = time.time() - start
    print(f"fib({n}) = {result} (took {elapsed:.3f}s)")
`

	chunkChan, errChan = sandbox.StreamExec(ctx, code)

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				select {
				case err := <-errChan:
					if err != nil {
						log.Printf("Error: %v", err)
					}
				default:
				}
				goto next2
			}
			fmt.Printf("   %s", chunk.Content)

		case err := <-errChan:
			if err != nil {
				log.Printf("Error: %v", err)
			}
			goto next2
		}
	}

next2:
	// Example 3: Interactive-style output
	fmt.Println("\n3. Interactive-style Output:")
	code = `
import sys

def ask(prompt):
    print(prompt, end='', flush=True)

ask("Loading configuration... ")
print("OK")

ask("Connecting to database... ")
print("OK")

ask("Starting server... ")
print("OK")

print("\nServer ready!")
`

	chunkChan, errChan = sandbox.StreamExec(ctx, code)

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				select {
				case err := <-errChan:
					if err != nil {
						log.Printf("Error: %v", err)
					}
				default:
				}
				goto done
			}
			fmt.Printf("   %s", chunk.Content)

		case err := <-errChan:
			if err != nil {
				log.Printf("Error: %v", err)
			}
			goto done
		}
	}

done:
	fmt.Println("\n========================")
	fmt.Println("Example completed!")
}
