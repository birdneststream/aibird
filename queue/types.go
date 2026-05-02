package queue

import (
	"context"
	"sync"

	"aibird/irc/state"
	"aibird/shared/meta"
)

type Item struct {
	State    state.State
	Function func(ctx context.Context, s state.State, gpu meta.GPUType)
}

// ProcessingQueue manages a single GPU processing queue
type ProcessingQueue struct {
	Queue *Queue
	Mutex sync.Mutex
}

type QueueItem struct {
	Item
	Model string
	User  UserAccess
	GPU   meta.GPUType
}

// UserAccess interface for queue items
type UserAccess interface {
	GetAccessLevel() int
	CanUsePremiumGPU() bool
	CanSkipQueue() bool
}
