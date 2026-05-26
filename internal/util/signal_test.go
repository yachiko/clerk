package util

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSignalHandler_ContextInitiallyOpen(t *testing.T) {
	sh := NewSignalHandler()
	require.NotNil(t, sh)

	ctx := sh.Context()
	require.NotNil(t, ctx)

	select {
	case <-ctx.Done():
		t.Fatal("context should not be done before any signal or cleanup")
	default:
	}
}

func TestSignalHandler_Cleanup_RunsRegisteredFuncs_InLIFOOrder(t *testing.T) {
	sh := NewSignalHandler()

	var order []int
	var calls int32
	sh.RegisterCleanup(func() {
		atomic.AddInt32(&calls, 1)
		order = append(order, 1)
	})
	sh.RegisterCleanup(func() {
		atomic.AddInt32(&calls, 1)
		order = append(order, 2)
	})
	sh.RegisterCleanup(func() {
		atomic.AddInt32(&calls, 1)
		order = append(order, 3)
	})

	sh.Cleanup()

	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
	// LIFO: last registered runs first
	assert.Equal(t, []int{3, 2, 1}, order)

	// Context is cancelled after cleanup.
	select {
	case <-sh.Context().Done():
	default:
		t.Fatal("context should be done after Cleanup()")
	}
}
