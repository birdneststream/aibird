package queue

import (
	"aibird/image/comfyui"
	"aibird/logger"
	"aibird/shared/meta"
	"aibird/status"
	"context"
	"errors"
	"os"
	"time"
)

func NewDualQueue() *DualQueue {
	return &DualQueue{
		Queue4090: &Queue{},
		Queue2070: &Queue{},
	}
}

func (dq *DualQueue) Enqueue(item QueueItem) (string, error) {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	// Check if this is a ComfyUI workflow by checking if the workflow file exists
	workflowFile := "comfyuijson/" + item.Model + ".json"
	if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
		// Not a ComfyUI workflow, treat as text command or other non-workflow command
		logger.Debug("Enqueuing non-workflow command", "model", item.Model)

		// For non-workflow commands, use 2070 by default (they don't need GPU)
		item.GPU = meta.GPU2070
		if item.User != nil && item.User.CanSkipQueue() {
			return dq.Queue2070.EnqueueFront(item, "")
		}
		return dq.Queue2070.Enqueue(item)
	}

	// Handle ComfyUI workflows
	metaData, err := comfyui.GetAibirdMeta(workflowFile)
	if err != nil {
		return "", errors.New("could not load workflow metadata for this model")
	}

	statusClient := status.NewClient(item.State.Config.AiBird)
	statusMeta := &meta.AibirdMeta{
		AccessLevel: metaData.AccessLevel,
		BigModel:    metaData.BigModel,
	}
	use4090, err := statusClient.CheckModelExecution(item.Model, statusMeta, item.User, item.State.User.NickName)
	if err != nil {
		return "", err
	}

	// Big model: strict 4090 only
	if metaData.BigModel {
		item.GPU = meta.GPU4090
		if item.User != nil && item.User.CanSkipQueue() {
			return dq.Queue4090.EnqueueFront(item, "")
		}
		return dq.Queue4090.Enqueue(item)
	}

	// Small model logic with defer to 2070 if 4090 is busy
	if use4090 {
		if !dq.Queue4090.IsCurrentlyProcessing() {
			item.GPU = meta.GPU4090
			if item.User != nil && item.User.CanSkipQueue() {
				return dq.Queue4090.EnqueueFront(item, "")
			}
			return dq.Queue4090.Enqueue(item)
		} else {
			item.GPU = meta.GPU2070
			msg := "4090 is busy, your request is being processed on the 2070 instead."
			if item.User != nil && item.User.CanSkipQueue() {
				return dq.Queue2070.EnqueueFront(item, msg)
			}
			return dq.Queue2070.Enqueue(item)
		}
	} else {
		item.GPU = meta.GPU2070
		if item.User != nil && item.User.CanSkipQueue() {
			return dq.Queue2070.EnqueueFront(item, "")
		}
		return dq.Queue2070.Enqueue(item)
	}
}

func (dq *DualQueue) ProcessQueues() {
	// Process 4090 queue
	go func() {
		for {
			if !dq.Queue4090.isProcessing() && !dq.Queue4090.IsEmpty() {
				dq.processQueueItem(dq.Queue4090)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Process 2070 queue
	go func() {
		for {
			if !dq.Queue2070.isProcessing() && !dq.Queue2070.IsEmpty() {
				dq.processQueueItem(dq.Queue2070)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
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
			item.State.SendError("An unknown error occurred, your request has been cancelled. Please try again later.")
		}

		queue.setProcessing(false)
		queue.setProcessingItem(nil)
	} else {
		queue.setProcessing(false)
		queue.setProcessingItem(nil)
	}
}

// Status methods
func (dq *DualQueue) IsEmpty() bool {
	return dq.Queue4090.IsEmpty() && dq.Queue2070.IsEmpty()
}

func (dq *DualQueue) IsCurrentlyProcessing() bool {
	return dq.Queue4090.IsCurrentlyProcessing() || dq.Queue2070.IsCurrentlyProcessing()
}

func (dq *DualQueue) GetQueueLengths() (int, int) {
	return dq.Queue4090.GetQueueLength(), dq.Queue2070.GetQueueLength()
}

func (dq *DualQueue) GetActionLists() ([]string, []string) {
	return dq.Queue4090.GetActionList(), dq.Queue2070.GetActionList()
}

// Admin control methods
func (dq *DualQueue) ClearAllQueues() {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	dq.Queue4090.Clear()
	dq.Queue2070.Clear()
	logger.Info("All queues cleared by admin")
}

func (dq *DualQueue) ClearQueue(gpuType meta.GPUType) {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	switch gpuType {
	case meta.GPU4090:
		dq.Queue4090.Clear()
		logger.Info("4090 queue cleared by admin")
	case meta.GPU2070:
		dq.Queue2070.Clear()
		logger.Info("2070 queue cleared by admin")
	}
}

func (dq *DualQueue) RemoveCurrentItem() bool {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	removed4090 := dq.Queue4090.RemoveCurrent()
	removed2070 := dq.Queue2070.RemoveCurrent()

	return removed4090 || removed2070
}

func (dq *DualQueue) GetDetailedStatus() *QueueStatus {
	dq.Mutex.Lock()
	defer dq.Mutex.Unlock()

	return &QueueStatus{
		Queue4090Length: dq.Queue4090.GetQueueLength(),
		Queue2070Length: dq.Queue2070.GetQueueLength(),
		Processing4090:  dq.Queue4090.IsCurrentlyProcessing(),
		Processing2070:  dq.Queue2070.IsCurrentlyProcessing(),
		Queue4090Items:  dq.Queue4090.GetActionList(),
		Queue2070Items:  dq.Queue2070.GetActionList(),
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
