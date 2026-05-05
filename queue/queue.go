// Package queue implements a thread-safe GPU processing queue with priority routing.
package queue

import (
	"errors"
	"fmt"
	"sync"

	"aibird/logger"
)

// Queue represents a queue data structure
// Now stores QueueItem instead of Item

type Queue struct {
	elements        []QueueItem
	mutex           sync.Mutex
	processing      bool
	processingMutex sync.Mutex
	processingItem  *QueueItem // currently processing item
}

// maxQueueSize is the maximum number of items allowed in the queue.
const maxQueueSize = 10

// Enqueue adds an element to the end of the queue
func (q *Queue) Enqueue(element QueueItem) (string, error) { //nolint:gocritic // element copied by value for goroutine isolation
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.hasElementsUnsafe(maxQueueSize + 1) {
		return "", errors.New("the queue is currently full (limit is 10), please try again in a few minutes")
	}

	// Add the new item to the queue
	q.elements = append(q.elements, element)

	// Determine the number of items ahead of the user
	itemsAhead := len(q.elements) - 1
	if q.isProcessing() {
		itemsAhead++
	}

	// If there are no items ahead, no message is needed
	if itemsAhead == 0 {
		return "", nil
	}

	// Formulate the queue message
	if itemsAhead == 1 {
		return "There is 1 item in the queue ahead of you. Your request will be processed shortly.", nil
	}

	return fmt.Sprintf("There are %d items in the queue ahead of you. Your request will be processed shortly.", itemsAhead), nil
}

// EnqueueFront adds an element to the start of the queue and returns a message if not empty.
// If msg is empty, uses the default VIP message.
func (q *Queue) EnqueueFront(element QueueItem, msg string) (string, error) { //nolint:gocritic // element copied by value for goroutine isolation
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// The queue is considered "busy" if it's processing an item or already has items.
	// A VIP user should get a message if they are skipping ahead of others, or if they still have to wait for the current item.
	isBusy := q.isProcessing() || len(q.elements) > 0

	q.elements = append([]QueueItem{element}, q.elements...)

	// Only send a message if the queue was busy. If it was completely idle,
	// the item will be processed immediately and the processing message will be sent.
	if !isBusy {
		return "", nil
	}

	if msg == "" {
		msg = "Your elite patreon status has put you at the front of the queue!"
	}
	return msg, nil
}

// Dequeue removes and returns the first element of the queue
func (q *Queue) Dequeue() *QueueItem {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if len(q.elements) == 0 {
		return nil
	}
	element := q.elements[0]
	q.elements = q.elements[1:]
	return &element
}

// IsEmpty checks if the queue is empty
func (q *Queue) IsEmpty() bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.elements) == 0
}

func (q *Queue) HasOneOrEmpty() bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.elements) <= 1
}

// HasElements checks if the queue has elements
func (q *Queue) HasElements(amount int) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.hasElementsUnsafe(amount)
}

// GetQueueLength returns the current number of items in the queue
func (q *Queue) GetQueueLength() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.elements)
}

// IsCurrentlyProcessing returns whether an item is actively being processed
func (q *Queue) IsCurrentlyProcessing() bool {
	return q.isProcessing()
}

// hasElementsUnsafe is an internal function that checks queue size without locking
// It should only be called when the mutex is already locked
func (q *Queue) hasElementsUnsafe(amount int) bool {
	return len(q.elements) >= amount
}

// setProcessing sets the processing flag to indicate a task is being processed
func (q *Queue) setProcessing(value bool) {
	q.processingMutex.Lock()
	defer q.processingMutex.Unlock()
	q.processing = value
}

// isProcessing checks if a task is currently being processed
func (q *Queue) isProcessing() bool {
	q.processingMutex.Lock()
	defer q.processingMutex.Unlock()
	return q.processing
}

// Len returns the current number of items in the queue
func (q *Queue) Len() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.elements)
}

// Peek returns the first element of the queue without removing it
func (q *Queue) Peek() *QueueItem {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if len(q.elements) == 0 {
		return nil
	}
	return &q.elements[0]
}

// GetActionList returns a slice of the action strings for each item in the queue
func (q *Queue) GetActionList() []string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	var actions []string
	for i := range q.elements {
		actions = append(actions, q.elements[i].State.Action())
	}
	return actions
}

// Clear removes all items from the queue
func (q *Queue) Clear() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.elements = nil
	logger.Info("Queue cleared")
}

// RemoveCurrent removes the currently processing item.
// Clears the processing flag and processingItem so GetProcessingAction()
// returns empty. Note: this does NOT cancel the running goroutine — full
// context cancellation requires passing cancel funcs through the queue
// (a future improvement).
func (q *Queue) RemoveCurrent() bool {
	q.processingMutex.Lock()
	defer q.processingMutex.Unlock()

	if q.processing {
		q.processing = false
		q.processingItem = nil
		logger.Info("Current processing item removed")
		return true
	}

	return false
}

// Set the currently processing item
func (q *Queue) setProcessingItem(item *QueueItem) {
	q.processingMutex.Lock()
	defer q.processingMutex.Unlock()
	q.processingItem = item
}

// GetProcessingAction returns the action name of the currently processing item,
// or an empty string if none.
func (q *Queue) GetProcessingAction() string {
	q.processingMutex.Lock()
	defer q.processingMutex.Unlock()
	if q.processingItem != nil {
		return q.processingItem.State.Action()
	}
	return ""
}
