package queue

import (
	"aibird/logger"
	"errors"
	"fmt"
	"sync"
	"time"
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

// Enqueue adds an element to the end of the queue
func (q *Queue) Enqueue(element QueueItem) (string, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.hasElementsUnsafe(11) {
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
func (q *Queue) EnqueueFront(element QueueItem, msg string) (string, error) {
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

// ProcessQueue continuously processes elements in the queue
func (q *Queue) ProcessQueue() {
	for {
		// Only process if we're not already processing and there are items in the queue
		if !q.isProcessing() && !q.IsEmpty() {
			logger.Debug("Queue: Starting to process next item", "queue_length", q.Len())

			// Mark as processing before dequeuing to prevent race conditions
			q.setProcessing(true)

			// Get the next item from the queue
			element := q.Dequeue()

			// Set the currently processing item
			q.setProcessingItem(element)

			// Process the item if it's a valid function
			if element != nil {
				// Execute the function in a goroutine but maintain processing flag
				go func() {
					logger.Debug("Queue: Executing function")
					// Execute the function
					element.Function(element.State, element.GPU)

					// Mark as not processing when done
					q.setProcessing(false)
					q.setProcessingItem(nil)
					logger.Debug("Queue: Function completed", "queue_length", q.Len())
				}()
			} else {
				// If not a valid function, reset processing flag
				q.setProcessing(false)
				q.setProcessingItem(nil)
				logger.Debug("Queue: Dequeued item was not a function")
			}
		}

		// Sleep to prevent CPU hogging
		time.Sleep(100 * time.Millisecond)
	}
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
	for _, item := range q.elements {
		actions = append(actions, item.State.Action())
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

// RemoveCurrent removes the currently processing item
func (q *Queue) RemoveCurrent() bool {
	q.processingMutex.Lock()
	defer q.processingMutex.Unlock()

	if q.processing {
		// Send cancellation message to user if we have access to the current item
		// For now, just mark as not processing
		q.processing = false
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

// Get the currently processing action (or empty string if none)
func (q *Queue) GetProcessingAction() string {
	q.processingMutex.Lock()
	defer q.processingMutex.Unlock()
	if q.processingItem != nil {
		return q.processingItem.State.Action()
	}
	return ""
}
