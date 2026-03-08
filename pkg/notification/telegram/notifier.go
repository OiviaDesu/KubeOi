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

package telegram

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

// notifier implements notification.Notifier for Telegram
type notifier struct {
	logger    logr.Logger
	botToken  string
	chatID    string
	enabled   bool
	apiURL    string
	httpClient *http.Client
}

// Config holds configuration for Telegram notifier
type Config struct {
	BotToken string
	ChatID   string
	Enabled  bool
}

// NewNotifier creates a new Telegram notifier
func NewNotifier(logger logr.Logger, cfg Config) notification.Notifier {
	return &notifier{
		logger:   logger,
		botToken: cfg.BotToken,
		chatID:   cfg.ChatID,
		enabled:  cfg.Enabled,
		apiURL:   fmt.Sprintf("https://api.telegram.org/bot%s", cfg.BotToken),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the unique name of this notifier
func (n *notifier) Name() string {
	return "telegram"
}

// IsEnabled checks if this notifier is enabled
func (n *notifier) IsEnabled() bool {
	return n.enabled && n.botToken != "" && n.chatID != ""
}

// Send sends a notification event via Telegram
func (n *notifier) Send(ctx context.Context, event *notification.Event) error {
	if !n.IsEnabled() {
		return fmt.Errorf("telegram notifier not enabled or not configured")
	}
	
	// Format message
	message := n.formatMessage(event)
	
	// Prepare request payload
	payload := map[string]interface{}{
		"chat_id":    n.chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %w", err)
	}
	
	// Send request
	url := fmt.Sprintf("%s/sendMessage", n.apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned non-OK status: %d", resp.StatusCode)
	}
	
	n.logger.V(1).Info("telegram notification sent",
		"title", event.Title,
		"severity", event.Severity)
	
	return nil
}

// formatMessage formats the event into a Telegram message
func (n *notifier) formatMessage(event *notification.Event) string {
	// Use appropriate icon based on severity
	icon := ""
	switch event.Severity {
	case notification.SeverityInfo:
		icon = "ℹ️"
	case notification.SeverityWarning:
		icon = "⚠️"
	case notification.SeverityCritical:
		icon = "🚨"
	}
	
	msg := fmt.Sprintf("%s *%s*\n\n%s\n\n", icon, event.Title, event.Message)
	msg += fmt.Sprintf("*Source:* %s\n", event.Source)
	msg += fmt.Sprintf("*Severity:* %s\n", event.Severity)
	msg += fmt.Sprintf("*Time:* %s", event.Timestamp.Format(time.RFC3339))
	
	return msg
}
