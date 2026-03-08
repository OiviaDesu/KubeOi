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

package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/oiviadesu/oiviak3s-operator/pkg/notification"
)

// notifier implements notification.Notifier for Discord
type notifier struct {
	logger     logr.Logger
	webhookURL string
	enabled    bool
	httpClient *http.Client
}

// Config holds configuration for Discord notifier
type Config struct {
	WebhookURL string
	Enabled    bool
}

// NewNotifier creates a new Discord notifier
func NewNotifier(logger logr.Logger, cfg Config) notification.Notifier {
	return &notifier{
		logger:     logger,
		webhookURL: cfg.WebhookURL,
		enabled:    cfg.Enabled,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the unique name of this notifier
func (n *notifier) Name() string {
	return "discord"
}

// IsEnabled checks if this notifier is enabled
func (n *notifier) IsEnabled() bool {
	return n.enabled && n.webhookURL != ""
}

// Send sends a notification event via Discord webhook
func (n *notifier) Send(ctx context.Context, event *notification.Event) error {
	if !n.IsEnabled() {
		return fmt.Errorf("discord notifier not enabled or not configured")
	}
	
	// Create Discord embed
	embed := n.createEmbed(event)
	
	// Prepare webhook payload
	payload := map[string]interface{}{
		"embeds": []interface{}{embed},
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal discord payload: %w", err)
	}
	
	// Send request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create discord request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send discord webhook: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord webhook returned non-OK status: %d", resp.StatusCode)
	}
	
	n.logger.V(1).Info("discord notification sent",
		"title", event.Title,
		"severity", event.Severity)
	
	return nil
}

// createEmbed creates a Discord embed from the event
func (n *notifier) createEmbed(event *notification.Event) map[string]interface{} {
	// Determine color based on severity
	color := 0x3498db // Blue for info
	switch event.Severity {
	case notification.SeverityWarning:
		color = 0xf39c12 // Orange
	case notification.SeverityCritical:
		color = 0xe74c3c // Red
	}
	
	fields := []map[string]interface{}{
		{
			"name":   "Source",
			"value":  event.Source,
			"inline": true,
		},
		{
			"name":   "Severity",
			"value":  string(event.Severity),
			"inline": true,
		},
	}
	
	// Add metadata fields if present
	if event.Metadata != nil {
		for key, value := range event.Metadata {
			fields = append(fields, map[string]interface{}{
				"name":   key,
				"value":  fmt.Sprintf("%v", value),
				"inline": true,
			})
		}
	}
	
	embed := map[string]interface{}{
		"title":       event.Title,
		"description": event.Message,
		"color":       color,
		"fields":      fields,
		"timestamp":   event.Timestamp.Format(time.RFC3339),
		"footer": map[string]interface{}{
			"text": "Oiviak3s Operator",
		},
	}
	
	return embed
}
