package queue

import (
	"aibird/irc/state"
	"aibird/shared/meta"
	"sync"
)

type Item struct {
	State    state.State
	Function func(state.State, meta.GPUType)
}

// Dual Queue Types
type DualQueue struct {
	Queue4090 *Queue
	Queue2070 *Queue
	Mutex     sync.Mutex
}

type QueueItem struct {
	Item
	Model string
	User  UserAccess
	GPU   meta.GPUType // Explicit GPU routing
}

// UserAccess interface for queue items
type UserAccess interface {
	GetAccessLevel() int
	CanUse4090() bool
	CanSkipQueue() bool
}
