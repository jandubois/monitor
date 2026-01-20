package notify

import (
	"context"
	"fmt"

	"github.com/jandubois/monitor/internal/probe"
)

// Channel is a notification channel.
type Channel interface {
	Send(ctx context.Context, msg *Message) error
	Type() string
}

// Message contains notification details.
type Message struct {
	Title    string
	Body     string
	Priority Priority
	Tags     []string
}

// Priority levels for notifications.
type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityUrgent
)

// StatusChange represents a probe status transition.
type StatusChange struct {
	ProbeName  string
	OldStatus  probe.Status
	NewStatus  probe.Status
	Message    string
}

// FormatStatusChange creates a notification message for a status change.
func FormatStatusChange(change *StatusChange) *Message {
	priority := PriorityNormal
	switch change.NewStatus {
	case probe.StatusCritical:
		priority = PriorityUrgent
	case probe.StatusWarning:
		priority = PriorityHigh
	case probe.StatusOK:
		if change.OldStatus == probe.StatusCritical || change.OldStatus == probe.StatusWarning {
			priority = PriorityNormal // Recovery
		}
	}

	title := fmt.Sprintf("[%s] %s", change.NewStatus, change.ProbeName)
	body := change.Message
	if change.OldStatus != "" {
		body = fmt.Sprintf("%s → %s: %s", change.OldStatus, change.NewStatus, change.Message)
	}

	tags := []string{string(change.NewStatus)}
	if change.OldStatus != "" && change.NewStatus == probe.StatusOK {
		tags = append(tags, "recovery")
	}

	return &Message{
		Title:    title,
		Body:     body,
		Priority: priority,
		Tags:     tags,
	}
}
