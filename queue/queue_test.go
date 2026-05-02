package queue

import (
	"log/slog"
	"os"
	"sync"
	"testing"

	"aibird/irc/state"
	"aibird/logger"
	"aibird/shared/meta"
)

func init() {
	// Initialize a minimal logger for tests (prevents nil pointer panics)
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
	_ = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// mockUser implements UserAccess for testing
type mockUser struct {
	accessLevel   int
	canUsePremium bool
	canSkip       bool
}

func (m *mockUser) GetAccessLevel() int    { return m.accessLevel }
func (m *mockUser) CanUsePremiumGPU() bool { return m.canUsePremium }
func (m *mockUser) CanSkipQueue() bool     { return m.canSkip }

func makeQueueItem(action string) QueueItem {
	return QueueItem{
		Item: Item{
			State: state.State{
				Command: state.Command{Action: action, Message: "test"},
			},
		},
		Model: action,
		User:  &mockUser{},
		GPU:   meta.GPU4090,
	}
}

func TestNewProcessingQueue(t *testing.T) {
	pq := NewProcessingQueue()
	if pq == nil {
		t.Fatal("NewProcessingQueue returned nil")
	}
	if pq.Queue == nil {
		t.Fatal("Queue is nil")
	}
}

func TestQueue_EnqueueDequeue(t *testing.T) {
	q := &Queue{}

	item := makeQueueItem("test-action")

	msg, err := q.Enqueue(item)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if msg != "" {
		t.Errorf("Expected no queue message for first item, got: %s", msg)
	}
	if q.Len() != 1 {
		t.Errorf("Expected queue length 1, got %d", q.Len())
	}

	dequeued := q.Dequeue()
	if dequeued == nil {
		t.Fatal("Dequeue returned nil")
	}
	if dequeued.Model != "test-action" {
		t.Errorf("Expected model 'test-action', got '%s'", dequeued.Model)
	}
	if !q.IsEmpty() {
		t.Error("Queue should be empty after dequeue")
	}
}

func TestQueue_EnqueueFull(t *testing.T) {
	q := &Queue{}

	// Fill the queue to max (maxQueueSize items allowed, checked at maxQueueSize+1)
	for i := 0; i < maxQueueSize; i++ {
		_, err := q.Enqueue(makeQueueItem("action"))
		if err != nil {
			t.Fatalf("Enqueue %d failed: %v", i, err)
		}
	}

	// One more should still succeed (limit is checked at maxQueueSize+1)
	_, err := q.Enqueue(makeQueueItem("tenth"))
	if err != nil {
		t.Errorf("Expected item %d to succeed: %v", maxQueueSize+1, err)
	}

	// Next enqueue should fail (now at maxQueueSize+1)
	_, err = q.Enqueue(makeQueueItem("overflow"))
	if err == nil {
		t.Error("Expected error when queue is full")
	}
}

func TestQueue_EnqueueQueueMessage(t *testing.T) {
	q := &Queue{}

	// First item should return empty message
	msg, _ := q.Enqueue(makeQueueItem("first"))
	if msg != "" {
		t.Errorf("First item should have no queue message, got: %s", msg)
	}

	// Second item should mention "1 item"
	msg, _ = q.Enqueue(makeQueueItem("second"))
	if msg == "" {
		t.Error("Second item should have a queue message")
	}
}

func TestQueue_EnqueueFront(t *testing.T) {
	q := &Queue{}

	_, _ = q.Enqueue(makeQueueItem("regular"))
	_, _ = q.Enqueue(makeQueueItem("regular2"))

	vip := makeQueueItem("vip")
	msg, err := q.EnqueueFront(vip, "VIP message")
	if err != nil {
		t.Fatalf("EnqueueFront failed: %v", err)
	}
	if msg == "" {
		t.Error("Expected VIP message since queue is busy")
	}

	peeked := q.Peek()
	if peeked == nil {
		t.Fatal("Peek returned nil")
	}
	if peeked.Model != "vip" {
		t.Errorf("Expected 'vip' at front, got '%s'", peeked.Model)
	}
}

func TestQueue_EnqueueFrontEmpty(t *testing.T) {
	q := &Queue{}

	vip := makeQueueItem("vip")
	msg, err := q.EnqueueFront(vip, "VIP message")
	if err != nil {
		t.Fatalf("EnqueueFront failed: %v", err)
	}
	if msg != "" {
		t.Errorf("Empty queue should return empty message, got: %s", msg)
	}
}

func TestQueue_DequeueEmpty(t *testing.T) {
	q := &Queue{}
	result := q.Dequeue()
	if result != nil {
		t.Error("Dequeue on empty queue should return nil")
	}
}

func TestQueue_PeekEmpty(t *testing.T) {
	q := &Queue{}
	result := q.Peek()
	if result != nil {
		t.Error("Peek on empty queue should return nil")
	}
}

func TestQueue_Peek(t *testing.T) {
	q := &Queue{}
	_, _ = q.Enqueue(makeQueueItem("first"))
	_, _ = q.Enqueue(makeQueueItem("second"))

	peeked := q.Peek()
	if peeked.Model != "first" {
		t.Errorf("Expected 'first', got '%s'", peeked.Model)
	}
	if q.Len() != 2 {
		t.Error("Peek should not remove items")
	}
}

func TestQueue_Clear(t *testing.T) {
	q := &Queue{}
	_, _ = q.Enqueue(makeQueueItem("a"))
	_, _ = q.Enqueue(makeQueueItem("b"))
	q.Clear()
	if !q.IsEmpty() {
		t.Error("Queue should be empty after Clear")
	}
}

func TestQueue_GetActionList(t *testing.T) {
	q := &Queue{}
	_, _ = q.Enqueue(makeQueueItem("action1"))
	_, _ = q.Enqueue(makeQueueItem("action2"))

	actions := q.GetActionList()
	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions))
	}
	if actions[0] != "action1" || actions[1] != "action2" {
		t.Errorf("Expected [action1, action2], got %v", actions)
	}
}

func TestQueue_HasOneOrEmpty(t *testing.T) {
	q := &Queue{}
	if !q.HasOneOrEmpty() {
		t.Error("Empty queue should return true")
	}
	_, _ = q.Enqueue(makeQueueItem("a"))
	if !q.HasOneOrEmpty() {
		t.Error("Queue with 1 item should return true")
	}
	_, _ = q.Enqueue(makeQueueItem("b"))
	if q.HasOneOrEmpty() {
		t.Error("Queue with 2 items should return false")
	}
}

func TestQueue_ProcessingState(t *testing.T) {
	q := &Queue{}

	if q.IsCurrentlyProcessing() {
		t.Error("Should not be processing initially")
	}

	q.setProcessing(true)
	if !q.IsCurrentlyProcessing() {
		t.Error("Should be processing after set")
	}

	if !q.RemoveCurrent() {
		t.Error("RemoveCurrent should return true when processing")
	}
	if q.IsCurrentlyProcessing() {
		t.Error("Should not be processing after remove")
	}
}

func TestQueue_ProcessingAction(t *testing.T) {
	q := &Queue{}
	if q.GetProcessingAction() != "" {
		t.Error("Empty queue should have empty processing action")
	}

	item := makeQueueItem("my-action")
	q.setProcessingItem(&item)
	if q.GetProcessingAction() != "my-action" {
		t.Errorf("Expected 'my-action', got '%s'", q.GetProcessingAction())
	}
}

func TestQueue_ConcurrentAccess(t *testing.T) {
	q := &Queue{}
	var wg sync.WaitGroup

	// Concurrent enqueues
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			q.Enqueue(makeQueueItem("concurrent"))
		}(i)
	}

	// Concurrent dequeues
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Dequeue()
		}()
	}

	wg.Wait()
	// If we get here without panic/deadlock, the test passes
}

func TestProcessingQueue_GetDetailedStatus(t *testing.T) {
	pq := NewProcessingQueue()

	status := pq.GetDetailedStatus()
	if status.QueueLength != 0 {
		t.Errorf("Expected 0 queue length, got %d", status.QueueLength)
	}
	if status.Processing {
		t.Error("Should not be processing")
	}
	if len(status.QueueItems) != 0 {
		t.Errorf("Expected empty queue items, got %v", status.QueueItems)
	}
}

func TestProcessingQueue_ClearQueue(t *testing.T) {
	pq := NewProcessingQueue()
	_, _ = pq.Queue.Enqueue(makeQueueItem("a"))
	_, _ = pq.Queue.Enqueue(makeQueueItem("b"))
	pq.ClearQueue()
	if !pq.IsEmpty() {
		t.Error("Queue should be empty after ClearQueue")
	}
}
