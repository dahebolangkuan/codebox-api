// Package utils provides internal utility functions for GoBox.
package utils

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// RetryConfig holds configuration for retry operations.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts (including the initial attempt).
	MaxAttempts int
	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration
	// Multiplier is the factor by which the delay increases after each retry.
	Multiplier float64
	// Jitter adds randomness to the delay (0.0-1.0 range).
	Jitter float64
	// RetryIf determines if an error should be retried.
	RetryIf func(error) bool
}

// DefaultRetryConfig returns a default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryIf:      DefaultRetryIf,
	}
}

// DefaultRetryIf determines if an error should be retried.
// It returns false for context cancellation and deadline exceeded errors.
func DefaultRetryIf(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// Retry executes the operation with retry logic.
// It returns the result from the operation, or the last error if all retries fail.
func Retry[T any](ctx context.Context, config *RetryConfig, operation func() (T, error)) (T, error) {
	var result T
	var lastErr error

	if config == nil {
		config = DefaultRetryConfig()
	}

	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		result, lastErr = operation()
		if lastErr == nil {
			return result, nil
		}

		// Check if we should retry
		if config.RetryIf != nil && !config.RetryIf(lastErr) {
			return result, lastErr
		}

		// Check if this was the last attempt
		if attempt == config.MaxAttempts {
			break
		}

		// Calculate delay with jitter
		actualDelay := delay
		if config.Jitter > 0 {
			jitter := delay.Seconds() * config.Jitter * (rand.Float64()*2 - 1) // +/- jitter%
			actualDelay = time.Duration(float64(delay) + jitter*float64(time.Second))
		}

		// Wait before next attempt
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(actualDelay):
		}

		// Increase delay for next iteration
		delay = time.Duration(math.Min(
			float64(delay)*config.Multiplier,
			float64(config.MaxDelay),
		))
	}

	return result, lastErr
}

// RetrySimple executes the operation with default retry configuration.
func RetrySimple[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	return Retry(ctx, DefaultRetryConfig(), operation)
}

// RetryVoid retries an operation that doesn't return a value.
func RetryVoid(ctx context.Context, config *RetryConfig, operation func() error) error {
	_, err := Retry(ctx, config, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

// RetryWithBackoff retries with exponential backoff.
func RetryWithBackoff(ctx context.Context, maxAttempts int, initialDelay time.Duration, operation func() error) error {
	config := &RetryConfig{
		MaxAttempts:  maxAttempts,
		InitialDelay: initialDelay,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryIf:      DefaultRetryIf,
	}
	return RetryVoid(ctx, config, operation)
}
