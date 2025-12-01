package utils

import (
	"context"
	"fmt"
	"time"
)

// TimeoutConfig holds configuration for timeout operations.
type TimeoutConfig struct {
	// Timeout is the maximum duration for the operation.
	Timeout time.Duration
	// OnTimeout is called when timeout occurs (optional).
	OnTimeout func()
}

// WithTimeout executes an operation with a timeout.
// If the operation completes before the timeout, its result is returned.
// If the timeout occurs first, an error is returned and the cleanup function is called.
func WithTimeout[T any](ctx context.Context, timeout time.Duration, operation func(ctx context.Context) (T, error)) (T, error) {
	var result T

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create channels for result and error
	resultCh := make(chan T, 1)
	errCh := make(chan error, 1)

	go func() {
		res, err := operation(ctx)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- res
	}()

	select {
	case result = <-resultCh:
		return result, nil
	case err := <-errCh:
		return result, err
	case <-ctx.Done():
		return result, fmt.Errorf("operation timed out after %v: %w", timeout, ctx.Err())
	}
}

// WithTimeoutVoid executes an operation without a return value with timeout.
func WithTimeoutVoid(ctx context.Context, timeout time.Duration, operation func(ctx context.Context) error) error {
	_, err := WithTimeout(ctx, timeout, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, operation(ctx)
	})
	return err
}

// RunWithDeadline runs an operation and returns whether it completed before the deadline.
func RunWithDeadline(ctx context.Context, deadline time.Time, operation func(ctx context.Context) error) (completed bool, err error) {
	timeout := time.Until(deadline)
	if timeout <= 0 {
		return false, context.DeadlineExceeded
	}

	err = WithTimeoutVoid(ctx, timeout, operation)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, err
		}
		return true, err
	}
	return true, nil
}

// TimeoutError is returned when an operation times out.
type TimeoutError struct {
	Operation       string
	TimeoutDuration time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s timed out after %v", e.Operation, e.TimeoutDuration)
}

// Timeout returns true, indicating this is a timeout error.
func (e *TimeoutError) Timeout() bool {
	return true
}

// IsTimeout checks if an error is a timeout error.
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if err == context.DeadlineExceeded {
		return true
	}
	if te, ok := err.(interface{ Timeout() bool }); ok {
		return te.Timeout()
	}
	return false
}

// Debounce creates a debounced version of a function.
// The function will only execute after it hasn't been called for the given duration.
func Debounce(ctx context.Context, duration time.Duration, fn func()) func() {
	var timer *time.Timer
	var timerMu = make(chan struct{}, 1)

	return func() {
		select {
		case timerMu <- struct{}{}:
		default:
		}

		if timer != nil {
			timer.Stop()
		}

		timer = time.AfterFunc(duration, func() {
			select {
			case <-ctx.Done():
				return
			default:
				fn()
			}
			<-timerMu
		})

		select {
		case <-timerMu:
		default:
		}
	}
}

// Throttle creates a throttled version of a function.
// The function will execute at most once per interval.
func Throttle(ctx context.Context, interval time.Duration, fn func()) func() {
	var lastRun time.Time
	var throttleMu = make(chan struct{}, 1)
	throttleMu <- struct{}{}

	return func() {
		select {
		case <-ctx.Done():
			return
		case <-throttleMu:
		}

		defer func() {
			throttleMu <- struct{}{}
		}()

		now := time.Now()
		if now.Sub(lastRun) >= interval {
			lastRun = now
			fn()
		}
	}
}
