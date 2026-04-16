package commands

import (
	"fmt"
	"strings"

	"aibird/irc/state"
	"aibird/queue"
)

func ShowQueueStatus(s state.State, q *queue.DualQueue) string {
	status := q.GetDetailedStatus()

	// Get currently processing action (single queue now)
	processingAction := q.Queue4090.GetProcessingAction()

	var messages []string

	// Single Queue Status
	if processingAction != "" {
		if status.Queue4090Length > 0 {
			messages = append(messages, fmt.Sprintf("🟢 GPU: Processing (%s) | 🟡 %d queued (%s)", processingAction, status.Queue4090Length, strings.Join(status.Queue4090Items, ", ")))
		} else {
			messages = append(messages, fmt.Sprintf("🟢 GPU: Processing (%s)", processingAction))
		}
	} else if status.Queue4090Length > 0 {
		messages = append(messages, fmt.Sprintf("🟡 GPU: %d queued (%s)", status.Queue4090Length, strings.Join(status.Queue4090Items, ", ")))
	} else {
		messages = append(messages, "⚪ GPU: Queue empty")
	}

	if status.Queue4090Length == 0 && processingAction == "" {
		return "Queue Status: Queue is empty"
	}

	return fmt.Sprintf("Queue Status: %s", strings.Join(messages, " | "))
}
