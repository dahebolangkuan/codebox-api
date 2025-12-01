// Basic example demonstrating simple code execution with GoBox
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
	fmt.Println("GoBox Basic Example")
	fmt.Println("====================")

	// Create a Docker-based sandbox
	sandbox, err := docker.New(
		docker.WithImage("shroominic/codebox:latest"),
		docker.WithTimeout(60*time.Second),
		docker.WithPoolSize(1, 3),
	)
	if err != nil {
		log.Fatalf("Failed to create sandbox: %v", err)
	}
	defer sandbox.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Example 1: Simple print
	fmt.Println("\n1. Simple Print:")
	result, err := sandbox.Exec(ctx, `print("Hello from GoBox!")`)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("   Output: %s", result.Text())
	}

	// Example 2: Math operations
	fmt.Println("\n2. Math Operations:")
	result, err = sandbox.Exec(ctx, `
import math
print(f"Pi = {math.pi}")
print(f"Square root of 2 = {math.sqrt(2)}")
print(f"2^10 = {2**10}")
`)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("   Output:\n%s", result.Text())
	}

	// Example 3: Using NumPy (if available)
	fmt.Println("\n3. NumPy Example:")
	result, err = sandbox.Exec(ctx, `
try:
    import numpy as np
    arr = np.array([1, 2, 3, 4, 5])
    print(f"Array: {arr}")
    print(f"Mean: {np.mean(arr)}")
    print(f"Sum: {np.sum(arr)}")
except ImportError:
    print("NumPy not available in this image")
`)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("   Output:\n%s", result.Text())
	}

	// Example 4: Error handling
	fmt.Println("\n4. Error Handling:")
	result, err = sandbox.Exec(ctx, `
x = 1 / 0  # This will raise a ZeroDivisionError
`)
	if err != nil {
		fmt.Printf("   Expected error: %v\n", err)
	} else {
		if result.HasError() {
			fmt.Printf("   Runtime error: %v\n", result.Errors())
		}
	}

	// Example 5: Using bash kernel
	fmt.Println("\n5. Bash Execution:")
	result, err = sandbox.Exec(ctx, `echo "Running bash command" && date`, gobox.WithKernel("bash"))
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("   Output:\n%s", result.Text())
	}

	fmt.Println("\n====================")
	fmt.Println("Example completed!")
}
