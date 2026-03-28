// Package notify provides webhook notification capabilities for rotation events.
// It supports Discord, Slack, and generic HTTP webhook formats.
package notify

import (
	"context"
	"errors"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
)

// Event represents a rotation event to be sent via webhook.
type Event struct {
	SecretName string
	Status     string // "success" or "failed"
	Details    string
	Timestamp  time.Time
}

// Notifier sends rotation event notifications to an external service.
type Notifier interface {
	Send(ctx context.Context, event Event) error
}

// Dispatcher fans out events to multiple notifiers, collecting all errors.
type Dispatcher struct {
	notifiers []Notifier
}

// NewDispatcher creates a Dispatcher from the given notifiers.
func NewDispatcher(notifiers ...Notifier) *Dispatcher {
	return &Dispatcher{notifiers: notifiers}
}

// Send dispatches the event to all registered notifiers.
// If multiple notifiers fail, errors are joined.
func (d *Dispatcher) Send(ctx context.Context, event Event) error {
	if len(d.notifiers) == 0 {
		return nil
	}

	var errs []error
	for _, n := range d.notifiers {
		if err := n.Send(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// NewNotifiersFromConfig creates Notifier implementations from configuration.
// Type mapping: "discord" -> DiscordNotifier, "slack" -> SlackNotifier,
// anything else -> GenericNotifier.
func NewNotifiersFromConfig(configs []config.NotifyConfig) []Notifier {
	notifiers := make([]Notifier, 0, len(configs))
	for _, c := range configs {
		switch c.Type {
		case "discord":
			notifiers = append(notifiers, &DiscordNotifier{webhookURL: c.URL})
		case "slack":
			notifiers = append(notifiers, &SlackNotifier{webhookURL: c.URL})
		default:
			notifiers = append(notifiers, &GenericNotifier{webhookURL: c.URL})
		}
	}
	return notifiers
}
