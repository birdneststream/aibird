package queue

import (
	"sync"

	"aibird/irc/state"
	"aibird/shared/meta"
)

type Item struct {
	State    state.State
	Function func(state.State, meta.GPUType)
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
