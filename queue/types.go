package queue

import (
	"context"
	"sync"
	"time"

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
	// Timeout, when > 0, overrides the queue's default per-item timeout. Used by
	// long-running commands such as MiniMax H3 video generation (~8 minutes).
	Timeout time.Duration
}

// UserAccess interface for queue items
type UserAccess interface {
	GetAccessLevel() int
	CanSkipQueue() bool
}
