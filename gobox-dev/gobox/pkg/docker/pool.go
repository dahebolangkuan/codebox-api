package docker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gobox/pkg/gobox"
)

// Pool manages a pool of pre-warmed Docker containers for fast code execution.
// The pool maintains a set of ready containers and handles their lifecycle.
type Pool struct {
	config    *PoolConfig
	driver    *Driver
	available chan *Container  // Containers ready for use
	inUse     map[string]*Container
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closed    bool
}

// NewPool creates a new container pool.
func NewPool(driver *Driver, config *PoolConfig) (*Pool, error) {
	if config == nil {
		config = DefaultPoolConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Pool{
		config:    config,
		driver:    driver,
		available: make(chan *Container, config.MaxSize),
		inUse:     make(map[string]*Container),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start the pool maintenance routine
	p.wg.Add(1)
	go p.maintain()

	// Start initial warmup
	p.wg.Add(1)
	go p.warmup()

	return p, nil
}

// Get retrieves a container from the pool.
// It returns a ready-to-use container or an error if the pool is exhausted.
func (p *Pool) Get(ctx context.Context) (*Container, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, gobox.ErrSandboxClosed
	}
	p.mu.RUnlock()

	select {
	case container := <-p.available:
		// Verify container is still healthy
		if !container.IsHealthy() {
			// Try to destroy unhealthy container and get another
			go container.Destroy(context.Background())
			return p.Get(ctx)
		}

		p.mu.Lock()
		p.inUse[container.ID] = container
		p.mu.Unlock()

		return container, nil

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-time.After(30 * time.Second):
		// Timeout waiting for a container
		// Try to create a new one on-demand if we haven't reached max size
		p.mu.RLock()
		currentSize := len(p.available) + len(p.inUse)
		p.mu.RUnlock()

		if currentSize < p.config.MaxSize {
			container, err := p.createContainer()
			if err != nil {
				return nil, fmt.Errorf("%w: failed to create container: %v", gobox.ErrPoolExhausted, err)
			}

			p.mu.Lock()
			p.inUse[container.ID] = container
			p.mu.Unlock()

			return container, nil
		}

		return nil, gobox.ErrPoolExhausted
	}
}

// Release returns a container to the pool.
// If reuse is true and the container is healthy, it will be reused.
// Otherwise, it will be destroyed.
func (p *Pool) Release(container *Container, reuse bool) {
	p.mu.Lock()
	delete(p.inUse, container.ID)
	closed := p.closed
	p.mu.Unlock()

	if closed {
		go container.Destroy(context.Background())
		return
	}

	if reuse && container.IsHealthy() {
		// Reset container state before returning to pool
		if err := container.Reset(context.Background()); err != nil {
			// If reset fails, destroy the container
			go container.Destroy(context.Background())
			return
		}

		// Try to return to pool
		select {
		case p.available <- container:
			// Successfully returned to pool
		default:
			// Pool is full, destroy the container
			go container.Destroy(context.Background())
		}
	} else {
		// Container is not healthy or not to be reused
		go container.Destroy(context.Background())
	}
}

// Size returns the current pool size statistics.
func (p *Pool) Size() (available, inUse int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.available), len(p.inUse)
}

// Close shuts down the pool and destroys all containers.
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// Cancel the pool context to stop maintenance routines
	p.cancel()

	// Wait for maintenance routines to finish
	p.wg.Wait()

	// Destroy all available containers
	close(p.available)
	for container := range p.available {
		container.Destroy(context.Background())
	}

	// Destroy all in-use containers
	p.mu.Lock()
	for _, container := range p.inUse {
		container.Destroy(context.Background())
	}
	p.inUse = make(map[string]*Container)
	p.mu.Unlock()

	return nil
}

// maintain runs the pool maintenance routine.
// It ensures the pool stays within configured bounds and cleans up idle containers.
func (p *Pool) maintain() {
	defer p.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return

		case <-ticker.C:
			p.maintainPoolSize()
			p.cleanupIdleContainers()
		}
	}
}

// maintainPoolSize ensures the pool has at least MinSize containers available.
func (p *Pool) maintainPoolSize() {
	p.mu.RLock()
	currentAvailable := len(p.available)
	currentInUse := len(p.inUse)
	p.mu.RUnlock()

	// Create new containers if we're below minimum
	for currentAvailable+currentInUse < p.config.MinSize {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		container, err := p.createContainer()
		if err != nil {
			// Log error but continue
			break
		}

		select {
		case p.available <- container:
			currentAvailable++
		case <-p.ctx.Done():
			container.Destroy(context.Background())
			return
		default:
			// Pool is full
			container.Destroy(context.Background())
			break
		}
	}
}

// cleanupIdleContainers removes containers that have been idle too long.
func (p *Pool) cleanupIdleContainers() {
	// We can't easily iterate over the channel without modifying it,
	// so we'll skip this for now and rely on the size-based management.
	// In a production system, we'd use a different data structure.
}

// warmup creates the initial set of warm containers.
func (p *Pool) warmup() {
	defer p.wg.Done()

	for i := 0; i < p.config.WarmupCount; i++ {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		container, err := p.createContainer()
		if err != nil {
			continue
		}

		select {
		case p.available <- container:
		case <-p.ctx.Done():
			container.Destroy(context.Background())
			return
		default:
			container.Destroy(context.Background())
		}
	}
}

// createContainer creates a new container using the driver.
func (p *Pool) createContainer() (*Container, error) {
	ctx, cancel := context.WithTimeout(p.ctx, p.driver.config.Timeout)
	defer cancel()

	return p.driver.createContainer(ctx)
}

// Stats returns pool statistics.
type PoolStats struct {
	Available int
	InUse     int
	MinSize   int
	MaxSize   int
}

// Stats returns current pool statistics.
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PoolStats{
		Available: len(p.available),
		InUse:     len(p.inUse),
		MinSize:   p.config.MinSize,
		MaxSize:   p.config.MaxSize,
	}
}
