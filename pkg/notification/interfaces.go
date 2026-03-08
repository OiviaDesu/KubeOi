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
	"time"
)

// Severity represents the severity level of a notification
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Event represents a notification event
type Event struct {
	Title     string
	Message   string
	Severity  Severity
	Timestamp time.Time
	Source    string
	Metadata  map[string]interface{}
}

// Notifier defines the interface for notification providers
// Following LSP (Liskov Substitution Principle) - all notifiers are interchangeable
type Notifier interface {
	// Send sends a notification event
	Send(ctx context.Context, event *Event) error
	
	// Name returns the unique name of this notifier
	Name() string
	
	// IsEnabled checks if this notifier is enabled
	IsEnabled() bool
}

// Manager coordinates multiple notification providers
// Following SRP (Single Responsibility Principle) - manages notifier orchestration
type Manager interface {
	// RegisterNotifier adds a notifier to the manager
	RegisterNotifier(notifier Notifier) error
	
	// Notify sends an event to all enabled notifiers
	Notify(ctx context.Context, event *Event) error
	
	// NotifyWithFilter sends an event only to notifiers matching the filter
	NotifyWithFilter(ctx context.Context, event *Event, filter func(Notifier) bool) error
}
