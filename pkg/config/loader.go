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

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Loader loads configuration from environment variables
type Loader struct{}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	return &Loader{}
}

// Load reads configuration from environment variables
func (l *Loader) Load() (*Config, error) {
	cfg := &Config{}

	// Load string values
	cfg.ClusterRegionHanoi = getEnvOrDefault("CLUSTER_REGION_HANOI", "hanoi")
	cfg.ClusterRegionMelbourne = getEnvOrDefault("CLUSTER_REGION_MELBOURNE", "melbourne")
	cfg.ZerotierNetworkID = os.Getenv("ZEROTIER_NETWORK_ID")
	cfg.ZerotierInterface = getEnvOrDefault("ZEROTIER_INTERFACE", "zt0")
	cfg.NodeX509fjIP = os.Getenv("NODE_X509FJ_IP")
	cfg.NodeMacMiniIP = os.Getenv("NODE_MACMINI_IP")
	cfg.NodePi400IP = os.Getenv("NODE_PI400_IP")
	cfg.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	cfg.TelegramChatID = os.Getenv("TELEGRAM_CHAT_ID")
	cfg.DiscordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
	cfg.PlacementStrategy = getEnvOrDefault("PLACEMENT_STRATEGY", "geographic")
	cfg.DefaultRegionPreference = getEnvOrDefault("DEFAULT_REGION_PREFERENCE", "hanoi")
	cfg.SharedEndpointMode = getEnvOrDefault("SHARED_ENDPOINT_MODE", "kube-vip")
	cfg.SharedEndpointIP = getEnvOrDefault("SHARED_ENDPOINT_IP", "192.168.86.8")
	cfg.MetricsBindAddress = getEnvOrDefault("METRICS_BIND_ADDRESS", ":8080")
	cfg.HealthProbeBindAddress = getEnvOrDefault("HEALTH_PROBE_BIND_ADDRESS", ":8081")
	cfg.LogLevel = getEnvOrDefault("LOG_LEVEL", "info")
	cfg.LogFormat = getEnvOrDefault("LOG_FORMAT", "json")

	// Load duration values
	var err error
	cfg.HealthCheckInterval, err = parseDuration("HEALTH_CHECK_INTERVAL", "30s")
	if err != nil {
		return nil, err
	}

	cfg.HealthCheckTimeout, err = parseDuration("HEALTH_CHECK_TIMEOUT", "10s")
	if err != nil {
		return nil, err
	}

	// Load integer values
	cfg.FailoverThreshold, err = parseInt("FAILOVER_THRESHOLD", 3)
	if err != nil {
		return nil, err
	}

	// Load boolean values
	cfg.NotificationEnabled = parseBool("NOTIFICATION_ENABLED", true)
	cfg.SharedEndpointEnabled = parseBool("SHARED_ENDPOINT_ENABLED", true)
	cfg.SharedEndpointAutoFailback = parseBool("SHARED_ENDPOINT_AUTO_FAILBACK", true)
	cfg.LeaderElect = parseBool("LEADER_ELECT", true)

	return cfg, nil
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// parseDuration parses a duration from environment or returns default
func parseDuration(key, defaultValue string) (time.Duration, error) {
	val := getEnvOrDefault(key, defaultValue)
	duration, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s: %w", key, err)
	}
	return duration, nil
}

// parseInt parses an integer from environment or returns default
func parseInt(key string, defaultValue int) (int, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue, nil
	}

	intVal, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s: %w", key, err)
	}
	return intVal, nil
}

// parseBool parses a boolean from environment or returns default
func parseBool(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}

	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		// Try case-insensitive string matching
		switch strings.ToLower(val) {
		case "true", "yes", "1":
			return true
		case "false", "no", "0":
			return false
		default:
			return defaultValue
		}
	}
	return boolVal
}
