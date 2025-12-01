// Pool example demonstrating container pool management with GoBox
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gobox/pkg/docker"
	"gobox/pkg/gobox"
)

func main() {
	fmt.Println("GoBox Container Pool Example")
	fmt.Println("=============================")

	// Create a Docker driver with container pool
	sandbox, err := docker.New(
		docker.WithImage("shroominic/codebox:latest"),
		docker.WithTimeout(60*time.Second),
		docker.WithPoolSize(3, 10), // Min 3, Max 10 containers
	)
	if err != nil {
		log.Fatalf("Failed to create sandbox: %v", err)
	}
	defer sandbox.Close()

	// Wait for pool to warm up
	fmt.Println("\nWaiting for pool to warm up...")
	time.Sleep(5 * time.Second)

	// Example 1: Concurrent execution
	fmt.Println("\n1. Concurrent Execution (10 parallel requests):")

	var wg sync.WaitGroup
	results := make(chan string, 10)

	start := time.Now()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			code := fmt.Sprintf(`
import time
import random

# Simulate some work
time.sleep(random.uniform(0.1, 0.5))
print(f"Task %d completed")
`, id)

			result, err := sandbox.Exec(ctx, code)
			if err != nil {
				results <- fmt.Sprintf("   Task %d: ERROR - %v", id, err)
			} else {
				results <- fmt.Sprintf("   Task %d: %s", id, result.Text())
			}
		}(i)
	}

	// Wait for all tasks to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		fmt.Print(result)
	}

	elapsed := time.Since(start)
	fmt.Printf("\n   Total time for 10 concurrent executions: %v\n", elapsed)

	// Example 2: Sequential vs Parallel comparison
	fmt.Println("\n2. Sequential vs Parallel Comparison:")

	// Sequential execution
	fmt.Println("\n   Sequential (5 tasks):")
	start = time.Now()

	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		code := fmt.Sprintf(`
import time
time.sleep(0.2)
print("Task %d done")
`, i)
		result, err := sandbox.Exec(ctx, code)
		cancel()

		if err != nil {
			fmt.Printf("   Task %d: ERROR - %v\n", i, err)
		} else {
			fmt.Printf("   %s", result.Text())
		}
	}

	seqTime := time.Since(start)
	fmt.Printf("   Sequential time: %v\n", seqTime)

	// Parallel execution
	fmt.Println("\n   Parallel (5 tasks):")
	start = time.Now()
	results = make(chan string, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			code := fmt.Sprintf(`
import time
time.sleep(0.2)
print("Task %d done")
`, id)

			result, err := sandbox.Exec(ctx, code)
			if err != nil {
				results <- fmt.Sprintf("   Task %d: ERROR - %v\n", id, err)
			} else {
				results <- fmt.Sprintf("   %s", result.Text())
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		fmt.Print(result)
	}

	parTime := time.Since(start)
	fmt.Printf("   Parallel time: %v\n", parTime)
	fmt.Printf("   Speedup: %.2fx\n", float64(seqTime)/float64(parTime))

	// Example 3: High-load simulation
	fmt.Println("\n3. High-Load Simulation (50 requests):")

	successCount := 0
	errorCount := 0
	var mu sync.Mutex

	start = time.Now()
	var wg2 sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg2.Add(1)
		go func(id int) {
			defer wg2.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			code := fmt.Sprintf("print(%d * %d)", id, id)
			_, err := sandbox.Exec(ctx, code)

			mu.Lock()
			if err != nil {
				errorCount++
			} else {
				successCount++
			}
			mu.Unlock()
		}(i)
	}

	wg2.Wait()
	elapsed = time.Since(start)

	fmt.Printf("   Success: %d\n", successCount)
	fmt.Printf("   Errors: %d\n", errorCount)
	fmt.Printf("   Total time: %v\n", elapsed)
	fmt.Printf("   Requests/second: %.2f\n", float64(50)/elapsed.Seconds())

	fmt.Println("\n=============================")
	fmt.Println("Example completed!")
}

// ExecWithRetry wraps Exec with retry logic for pool exhaustion scenarios
func ExecWithRetry(sandbox gobox.Sandbox, ctx context.Context, code string, maxRetries int) (*gobox.ExecResult, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		result, err := sandbox.Exec(ctx, code)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if gobox.IsPoolExhausted(err) {
			// Wait before retrying
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}

		// Non-retryable error
		return nil, err
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}
