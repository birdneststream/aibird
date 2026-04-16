package commands

import (
	"fmt"
	"strings"

	"aibird/irc/state"
	"aibird/queue"
)

func ShowQueueStatus(s state.State, q *queue.ProcessingQueue) string {
	status := q.GetDetailedStatus()
	processingAction := q.Queue.GetProcessingAction()

	var messages []string

	if processingAction != "" {
		if status.QueueLength > 0 {
			messages = append(messages, fmt.Sprintf("🟢 GPU: Processing (%s) | 🟡 %d queued (%s)", processingAction, status.QueueLength, strings.Join(status.QueueItems, ", ")))
		} else {
			messages = append(messages, fmt.Sprintf("🟢 GPU: Processing (%s)", processingAction))
		}
	} else if status.QueueLength > 0 {
		messages = append(messages, fmt.Sprintf("🟡 GPU: %d queued (%s)", status.QueueLength, strings.Join(status.QueueItems, ", ")))
	} else {
		messages = append(messages, "⚪ GPU: Queue empty")
	}

	if status.QueueLength == 0 && processingAction == "" {
		return "Queue Status: Queue is empty"
	}

	return fmt.Sprintf("Queue Status: %s", strings.Join(messages, " | "))
}
