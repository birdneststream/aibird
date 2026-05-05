package helpers

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGoSafe_NormalExecution(t *testing.T) {
	var called atomic.Bool
	GoSafe(func() {
		called.Store(true)
	})

	assert.Eventually(t, called.Load, time.Second, 10*time.Millisecond)
}

func TestGoSafe_PanicRecovery(t *testing.T) {
	// GoSafe should not panic even when the function panics
	var done atomic.Bool
	GoSafe(func() {
		panic("test panic in goroutine")
	})

	// Give the goroutine time to run and recover
	GoSafe(func() {
		done.Store(true)
	})
	assert.Eventually(t, done.Load, time.Second, 10*time.Millisecond)
}

func TestGoSafe_ConcurrentExecution(t *testing.T) {
	const count = 10
	var wg sync.WaitGroup
	var counter atomic.Int32

	for i := 0; i < count; i++ {
		wg.Add(1)
		GoSafe(func() {
			counter.Add(1)
			wg.Done()
		})
	}

	assert.Eventually(t, func() bool { return counter.Load() == count }, 2*time.Second, 10*time.Millisecond)
}
