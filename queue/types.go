package queue

import (
	"context"
	"sync"

	"aibird/irc/state"
)

type Item struct {
	State    state.State
	Function func(ctx context.Context, s state.State)
}

// ProcessingQueue manages a single GPU processing queue
type ProcessingQueue struct {
	Queue *Queue
	mutex sync.Mutex
}

type QueueItem struct {
	Item
	Model string
	User  UserAccess
}

// UserAccess interface for queue items
type UserAccess interface {
	GetAccessLevel() int
	CanSkipQueue() bool
}
