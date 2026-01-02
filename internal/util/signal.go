package util

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// CleanupFunc is a function to run during cleanup
type CleanupFunc func()

// SignalHandler manages graceful shutdown
type SignalHandler struct {
	cleanupFuncs []CleanupFunc
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewSignalHandler creates a new signal handler
func NewSignalHandler() *SignalHandler {
	ctx, cancel := context.WithCancel(context.Background())
	sh := &SignalHandler{
		ctx:    ctx,
		cancel: cancel,
	}
	sh.setup()
	return sh
}

// setup sets up signal handling
func (sh *SignalHandler) setup() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		sh.cleanup()
		os.Exit(1)
	}()
}

// Context returns the context that will be cancelled on signal
func (sh *SignalHandler) Context() context.Context {
	return sh.ctx
}

// RegisterCleanup registers a cleanup function to run on shutdown
func (sh *SignalHandler) RegisterCleanup(fn CleanupFunc) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.cleanupFuncs = append(sh.cleanupFuncs, fn)
}

// cleanup runs all registered cleanup functions
func (sh *SignalHandler) cleanup() {
	sh.cancel()
	sh.mu.Lock()
	defer sh.mu.Unlock()

	for i := len(sh.cleanupFuncs) - 1; i >= 0; i-- {
		sh.cleanupFuncs[i]()
	}
}

// Cleanup manually triggers cleanup
func (sh *SignalHandler) Cleanup() {
	sh.cleanup()
}
