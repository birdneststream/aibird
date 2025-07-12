package commands

import (
	"aibird/irc/state"
	"aibird/queue"
	"fmt"
	"strings"
)

func ShowQueueStatus(s state.State, q *queue.DualQueue) {
	status := q.GetDetailedStatus()

	// Get currently processing actions
	processing4090 := q.Queue4090.GetProcessingAction()
	processing2070 := q.Queue2070.GetProcessingAction()

	var messages []string

	// 4090 Queue Status
	if processing4090 != "" {
		if status.Queue4090Length > 0 {
			messages = append(messages, fmt.Sprintf("🟢 4090: Processing (%s) | 🟡 %d queued (%s)", processing4090, status.Queue4090Length, strings.Join(status.Queue4090Items, ", ")))
		} else {
			messages = append(messages, fmt.Sprintf("🟢 4090: Processing (%s)", processing4090))
		}
	} else if status.Queue4090Length > 0 {
		messages = append(messages, fmt.Sprintf("🟡 4090: %d queued (%s)", status.Queue4090Length, strings.Join(status.Queue4090Items, ", ")))
	} else {
		messages = append(messages, "⚪ 4090: Empty")
	}

	// 2070 Queue Status
	if processing2070 != "" {
		if status.Queue2070Length > 0 {
			messages = append(messages, fmt.Sprintf("🟢 2070: Processing (%s) | 🟡 %d queued (%s)", processing2070, status.Queue2070Length, strings.Join(status.Queue2070Items, ", ")))
		} else {
			messages = append(messages, fmt.Sprintf("🟢 2070: Processing (%s)", processing2070))
		}
	} else if status.Queue2070Length > 0 {
		messages = append(messages, fmt.Sprintf("🟡 2070: %d queued (%s)", status.Queue2070Length, strings.Join(status.Queue2070Items, ", ")))
	} else {
		messages = append(messages, "⚪ 2070: Empty")
	}

	if status.Queue4090Length == 0 && status.Queue2070Length == 0 && processing4090 == "" && processing2070 == "" {
		s.Send("Queue Status: All queues are empty")
		return
	}

	s.Send(fmt.Sprintf("Queue Status: %s", strings.Join(messages, " | ")))
}
