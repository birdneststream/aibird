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

func NewProcessingQueue() *ProcessingQueue {
	return &ProcessingQueue{
		Queue: &Queue{},
	}
}

func (pq *ProcessingQueue) Enqueue(item QueueItem) (string, error) { //nolint:gocyclo
	pq.Mutex.Lock()
	defer pq.Mutex.Unlock()

	// Check if this is a ComfyUI workflow by checking if the workflow file exists
	workflowFile := "comfyuijson/" + item.Model + ".json"
	if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
		// Not a ComfyUI workflow (e.g. llama.cpp AI request)
		logger.Debug("Enqueuing non-workflow command", "model", item.Model)

		if item.GPU == "" {
			item.GPU = meta.GPU4090
		}

		if item.User != nil && item.User.CanSkipQueue() {
			return pq.Queue.EnqueueFront(item, "")
		}
		return pq.Queue.Enqueue(item)
	}

	// ComfyUI workflow — run pre-flight checks
	metaData, err := comfyui.GetAibirdMeta(workflowFile)
	if err != nil {
		return "", errors.New("could not load workflow metadata for this model")
	}

	statusClient := status.NewClient(item.State.Config.AiBird)
	statusMeta := &meta.AibirdMeta{
		AccessLevel: metaData.AccessLevel,
	}
	if err := statusClient.CheckModelExecution(statusMeta, item.User); err != nil {
		return "", err
	}

	item.GPU = meta.GPU4090
	logger.Info("Enqueue GPU selection", "model", item.Model, "gpu", item.GPU)

	if item.User != nil && item.User.CanSkipQueue() {
		return pq.Queue.EnqueueFront(item, "")
	}
	return pq.Queue.Enqueue(item)
}

func (pq *ProcessingQueue) ProcessQueues(ctx context.Context) error {
	return pq.processQueue(ctx, pq.Queue, "Primary")
}

func (pq *ProcessingQueue) processQueue(ctx context.Context, queue *Queue, queueName string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Queue processing stopped due to context cancellation", "queue", queueName)
			return ctx.Err()
		case <-ticker.C:
			if !queue.isProcessing() && !queue.IsEmpty() {
				pq.processQueueItem(ctx, queue)
			}
		}
	}
}

func (pq *ProcessingQueue) processQueueItem(parentCtx context.Context, queue *Queue) {
	queue.setProcessing(true)
	item := queue.Dequeue()
	queue.setProcessingItem(item)
	if item != nil {
		logger.Debug("Processing queue item", "gpu", item.GPU, "action", item.State.Action())

		ctx, cancel := context.WithTimeout(parentCtx, 4*time.Minute)
		defer cancel()

		done := make(chan struct{})

		go func() {
			item.Function(ctx, item.State, item.GPU)
			close(done)
		}()

		select {
		case <-done:
			logger.Debug("Completed queue item", "gpu", item.GPU)
		case <-ctx.Done():
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

// Status methods
func (pq *ProcessingQueue) IsEmpty() bool {
	return pq.Queue.IsEmpty()
}

func (pq *ProcessingQueue) IsCurrentlyProcessing() bool {
	return pq.Queue.IsCurrentlyProcessing()
}

// Admin control methods
// ClearQueue removes all items from the queue.
func (pq *ProcessingQueue) ClearQueue() {
	pq.Mutex.Lock()
	defer pq.Mutex.Unlock()

	pq.Queue.Clear()
	logger.Info("Queue cleared by admin")
}

func (pq *ProcessingQueue) RemoveCurrentItem() bool {
	pq.Mutex.Lock()
	defer pq.Mutex.Unlock()

	return pq.Queue.RemoveCurrent()
}

func (pq *ProcessingQueue) GetDetailedStatus() *QueueStatus {
	pq.Mutex.Lock()
	defer pq.Mutex.Unlock()

	return &QueueStatus{
		QueueLength: pq.Queue.GetQueueLength(),
		Processing:  pq.Queue.IsCurrentlyProcessing(),
		QueueItems:  pq.Queue.GetActionList(),
	}
}

type QueueStatus struct {
	QueueLength int      `json:"queue_length"`
	Processing  bool     `json:"processing"`
	QueueItems  []string `json:"queue_items"`
}
