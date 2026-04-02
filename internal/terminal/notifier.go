package terminal

import (
	"context"
	"fmt"
	"io"
	"sync"

	"baize/internal/reminder"
)

type Notifier struct {
	output io.Writer
	mu     sync.Mutex
}

func NewNotifier(output io.Writer) *Notifier {
	return &Notifier{output: output}
}

func (n *Notifier) Notify(_ context.Context, item reminder.Reminder) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, err := fmt.Fprintf(n.output, "\n[notice #%s] %s\nbaize> ", shortID(item.ID), item.Message)
	return err
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
