package queue

import (
	"context"
	"errors"
	"os"
	"time"

	"aibird/image/comfyui"
	"aibird/logger"
	"aibird/shared/meta"
	"aibird/status"
)

func NewDualQueue() *DualQueue {
	return &DualQueue{
		Queue4090: &Queue{},
		Queue2070: &Queue{},
	}
}

func (dq *DualQueue) Enqueue(item QueueItem) (string, error) { //nolint:gocyclo
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	// Check if this is a ComfyUI workflow by checking if the workflow file exists
	workflowFile := "comfyuijson/" + item.Model + ".json"
	if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
		// Not a ComfyUI workflow (could be Ollama AI request or other)
		logger.Debug("Enqueuing non-workflow command", "model", item.Model)

		// Default GPU type for non-workflow items (not really used, but set for consistency)
		if item.GPU == "" {
			item.GPU = meta.GPU5090 // Default to primary GPU (5090/cuda:1)
		}

		// All items go to the primary queue now
		if item.User != nil && item.User.CanSkipQueue() {
			return dq.Queue4090.EnqueueFront(item, "")
		}
		return dq.Queue4090.Enqueue(item)
	}

	// Handle ComfyUI workflows - determine GPU based on status
	metaData, err := comfyui.GetAibirdMeta(workflowFile)
	if err != nil {
		return "", errors.New("could not load workflow metadata for this model")
	}

	statusClient := status.NewClient(item.State.Config.AiBird)
	statusMeta := &meta.AibirdMeta{
		AccessLevel: metaData.AccessLevel,
	}
	use5090, err := statusClient.CheckModelExecution(item.Model, statusMeta, item.User, item.State.User.NickName)
	if err != nil {
		return "", err
	}

	// Set GPU type based on status check
	if use5090 {
		item.GPU = meta.GPU5090 // 5090 (cuda:1)
	} else {
		item.GPU = meta.GPU4090 // 4090 (cuda:0)
	}

	logger.Info("Enqueue GPU selection", "model", item.Model, "use5090", use5090, "gpu", item.GPU)

	// All items go to the single primary queue
	if item.User != nil && item.User.CanSkipQueue() {
		return dq.Queue4090.EnqueueFront(item, "")
	}
	return dq.Queue4090.Enqueue(item)
}

func (dq *DualQueue) ProcessQueues(ctx context.Context) error {
	// Single queue processing now - all items go through Queue4090 (primary queue)
	return dq.processQueue(ctx, dq.Queue4090, "Primary")
}

func (dq *DualQueue) processQueue(ctx context.Context, queue *Queue, queueName string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Queue processing stopped due to context cancellation", "queue", queueName)
			return ctx.Err()
		case <-ticker.C:
			if !queue.isProcessing() && !queue.IsEmpty() {
				dq.processQueueItem(queue)
			}
		}
	}
}

func (dq *DualQueue) processQueueItem(queue *Queue) {
	queue.setProcessing(true)
	item := queue.Dequeue()
	queue.setProcessingItem(item)
	if item != nil {
		logger.Debug("Processing queue item", "gpu", item.GPU, "action", item.State.Action())

		// Create a context with a 4-minute timeout
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		// Create a channel to signal when the function is complete
		done := make(chan struct{})

		go func() {
			item.Function(item.State, item.GPU)
			close(done)
		}()

		select {
		case <-done:
			// Function completed successfully
			logger.Debug("Completed queue item", "gpu", item.GPU)
		case <-ctx.Done():
			// Timeout occurred
			logger.Error("Queue item timed out", "gpu", item.GPU, "action", item.State.Action())
			item.State.SendError("An unknown error occurred, your request has been canceled. Please try again later.")
		}

		queue.setProcessing(false)
		queue.setProcessingItem(nil)
	} else {
		queue.setProcessing(false)
		queue.setProcessingItem(nil)
	}
}

// Status methods - single queue
func (dq *DualQueue) IsEmpty() bool {
	return dq.Queue4090.IsEmpty()
}

func (dq *DualQueue) IsCurrentlyProcessing() bool {
	return dq.Queue4090.IsCurrentlyProcessing()
}

func (dq *DualQueue) GetQueueLengths() (int, int) {
	// Return (primary_length, 0) for compatibility
	return dq.Queue4090.GetQueueLength(), 0
}

func (dq *DualQueue) GetActionLists() ([]string, []string) {
	// Return (primary_actions, empty) for compatibility
	return dq.Queue4090.GetActionList(), []string{}
}

// Admin control methods
func (dq *DualQueue) ClearAllQueues() {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	dq.Queue4090.Clear()
	logger.Info("Queue cleared by admin")
}

func (dq *DualQueue) ClearQueue(gpuType meta.GPUType) {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	// All clears now affect the single queue
	dq.Queue4090.Clear()
	logger.Info("Queue cleared by admin", "requested_gpu", gpuType)
}

func (dq *DualQueue) RemoveCurrentItem() bool {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	return dq.Queue4090.RemoveCurrent()
}

func (dq *DualQueue) GetDetailedStatus() *QueueStatus {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	return &QueueStatus{
		Queue4090Length: dq.Queue4090.GetQueueLength(),
		Queue2070Length: 0, // Always 0 - single queue now
		Processing4090:  dq.Queue4090.IsCurrentlyProcessing(),
		Processing2070:  false, // Always false - single queue now
		Queue4090Items:  dq.Queue4090.GetActionList(),
		Queue2070Items:  []string{}, // Always empty - single queue now
	}
}

type QueueStatus struct {
	Queue4090Length int      `json:"queue_4090_length"`
	Queue2070Length int      `json:"queue_2070_length"`
	Processing4090  bool     `json:"processing_4090"`
	Processing2070  bool     `json:"processing_2070"`
	Queue4090Items  []string `json:"queue_4090_items"`
	Queue2070Items  []string `json:"queue_2070_items"`
}
