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
	"time"
)

// Config represents the operator configuration
type Config struct {
	// Cluster configuration
	ClusterRegionHanoi     string `env:"CLUSTER_REGION_HANOI" envDefault:"hanoi"`
	ClusterRegionMelbourne string `env:"CLUSTER_REGION_MELBOURNE" envDefault:"melbourne"`

	// ZeroTier network configuration
	ZerotierNetworkID string `env:"ZEROTIER_NETWORK_ID"`
	ZerotierInterface string `env:"ZEROTIER_INTERFACE" envDefault:"zt0"`

	// Node IP mappings
	NodeX509fjIP  string `env:"NODE_X509FJ_IP"`
	NodeMacMiniIP string `env:"NODE_MACMINI_IP"`
	NodePi400IP   string `env:"NODE_PI400_IP"`

	// Health check configuration
	HealthCheckInterval time.Duration `env:"HEALTH_CHECK_INTERVAL" envDefault:"30s"`
	HealthCheckTimeout  time.Duration `env:"HEALTH_CHECK_TIMEOUT" envDefault:"10s"`
	FailoverThreshold   int           `env:"FAILOVER_THRESHOLD" envDefault:"3"`

	// Notification configuration
	NotificationEnabled bool   `env:"NOTIFICATION_ENABLED" envDefault:"true"`
	TelegramBotToken    string `env:"TELEGRAM_BOT_TOKEN"`
	TelegramChatID      string `env:"TELEGRAM_CHAT_ID"`
	DiscordWebhookURL   string `env:"DISCORD_WEBHOOK_URL"`

	// Placement configuration
	PlacementStrategy       string `env:"PLACEMENT_STRATEGY" envDefault:"geographic"`
	DefaultRegionPreference string `env:"DEFAULT_REGION_PREFERENCE" envDefault:"hanoi"`

	// Shared endpoint configuration
	SharedEndpointEnabled      bool   `env:"SHARED_ENDPOINT_ENABLED" envDefault:"true"`
	SharedEndpointMode         string `env:"SHARED_ENDPOINT_MODE" envDefault:"kube-vip"`
	SharedEndpointIP           string `env:"SHARED_ENDPOINT_IP" envDefault:"192.168.86.8"`
	SharedEndpointAutoFailback bool   `env:"SHARED_ENDPOINT_AUTO_FAILBACK" envDefault:"true"`

	// Metrics configuration
	MetricsBindAddress     string `env:"METRICS_BIND_ADDRESS" envDefault:":8080"`
	HealthProbeBindAddress string `env:"HEALTH_PROBE_BIND_ADDRESS" envDefault:":8081"`

	// Leader election
	LeaderElect bool `env:"LEADER_ELECT" envDefault:"true"`

	// Logging
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`
}

// HealthCheckConfig returns health check specific configuration
func (c *Config) HealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Interval:          c.HealthCheckInterval,
		Timeout:           c.HealthCheckTimeout,
		FailoverThreshold: c.FailoverThreshold,
		ZerotierInterface: c.ZerotierInterface,
	}
}

// NotificationConfig returns notification specific configuration
func (c *Config) NotificationConfig() NotificationConfig {
	return NotificationConfig{
		Enabled:           c.NotificationEnabled,
		TelegramBotToken:  c.TelegramBotToken,
		TelegramChatID:    c.TelegramChatID,
		DiscordWebhookURL: c.DiscordWebhookURL,
	}
}

// PlacementConfig returns placement specific configuration
func (c *Config) PlacementConfig() PlacementConfig {
	return PlacementConfig{
		Strategy:                c.PlacementStrategy,
		DefaultRegionPreference: c.DefaultRegionPreference,
	}
}

// HealthCheckConfig holds health check specific settings
type HealthCheckConfig struct {
	Interval          time.Duration
	Timeout           time.Duration
	FailoverThreshold int
	ZerotierInterface string
}

// NotificationConfig holds notification specific settings
type NotificationConfig struct {
	Enabled           bool
	TelegramBotToken  string
	TelegramChatID    string
	DiscordWebhookURL string
}

// PlacementConfig holds placement specific settings
type PlacementConfig struct {
	Strategy                string
	DefaultRegionPreference string
}

// SharedEndpointConfig holds shared endpoint defaults
type SharedEndpointConfig struct {
	Enabled      bool
	Mode         string
	IP           string
	AutoFailback bool
}

// SharedEndpointDefaults returns shared endpoint default settings
func (c *Config) SharedEndpointDefaults() SharedEndpointConfig {
	return SharedEndpointConfig{
		Enabled:      c.SharedEndpointEnabled,
		Mode:         c.SharedEndpointMode,
		IP:           c.SharedEndpointIP,
		AutoFailback: c.SharedEndpointAutoFailback,
	}
}
