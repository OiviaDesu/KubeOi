/*
Copyright 2026 oiviadesu.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package notification

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
)

// manager implements the Manager interface
// Following Facade pattern for simplified notification API
type manager struct {
	notifiers []Notifier
	mu        sync.RWMutex
	logger    logr.Logger
}

// NewManager creates a new notification manager
func NewManager(logger logr.Logger) Manager {
	return &manager{
		notifiers: make([]Notifier, 0),
		logger:    logger,
	}
}

// RegisterNotifier adds a notifier to the manager
func (m *manager) RegisterNotifier(notifier Notifier) error {
	if notifier == nil {
		return fmt.Errorf("notifier cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate notifier names
	for _, existing := range m.notifiers {
		if existing.Name() == notifier.Name() {
			return fmt.Errorf("notifier with name %s already registered", notifier.Name())
		}
	}

	m.notifiers = append(m.notifiers, notifier)
	m.logger.Info("registered notifier", "notifier", notifier.Name())
	return nil
}

// Notify sends an event to all enabled notifiers
func (m *manager) Notify(ctx context.Context, event *Event) error {
	return m.NotifyWithFilter(ctx, event, func(n Notifier) bool {
		return n.IsEnabled()
	})
}

// NotifyWithFilter sends an event only to notifiers matching the filter
func (m *manager) NotifyWithFilter(ctx context.Context, event *Event, filter func(Notifier) bool) error {
	m.mu.RLock()
	notifiers := make([]Notifier, len(m.notifiers))
	copy(notifiers, m.notifiers)
	m.mu.RUnlock()

	var lastErr error
	sentCount := 0

	for _, notifier := range notifiers {
		if filter != nil && !filter(notifier) {
			continue
		}

		if err := notifier.Send(ctx, event); err != nil {
			m.logger.Error(err, "failed to send notification",
				"notifier", notifier.Name(),
				"event", event.Title)
			lastErr = err
			continue
		}

		sentCount++
		m.logger.V(1).Info("notification sent",
			"notifier", notifier.Name(),
			"severity", event.Severity,
			"title", event.Title)
	}

	if sentCount == 0 && lastErr != nil {
		return fmt.Errorf("all notifiers failed: %w", lastErr)
	}

	return nil
}
