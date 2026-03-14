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

package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	"github.com/oiviadesu/oiviak3s-operator/pkg/health"
	corev1 "k8s.io/api/core/v1"
)

// checker implements health.Checker for network connectivity
// Checks ZeroTier interface and node reachability
type checker struct {
	logger            logr.Logger
	zerotierInterface string
	pingTimeout       time.Duration
}

// Config holds configuration for network checker
type Config struct {
	ZerotierInterface string
	PingTimeout       time.Duration
}

// NewChecker creates a new network health checker
func NewChecker(logger logr.Logger, cfg Config) health.Checker {
	if cfg.ZerotierInterface == "" {
		cfg.ZerotierInterface = "zt0"
	}
	if cfg.PingTimeout == 0 {
		cfg.PingTimeout = 5 * time.Second
	}

	return &checker{
		logger:            logger,
		zerotierInterface: cfg.ZerotierInterface,
		pingTimeout:       cfg.PingTimeout,
	}
}

// Name returns the unique name of this checker
func (c *checker) Name() string {
	return "network"
}

// Check performs network connectivity check
func (c *checker) Check(ctx context.Context, node *corev1.Node) (*health.CheckResult, error) {
	result := &health.CheckResult{
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Get node internal IP
	nodeIP := getNodeInternalIP(node)
	if nodeIP == "" {
		result.Status = health.HealthStatusUnknown
		result.Message = "Node internal IP not found"
		return result, nil
	}

	result.Details["nodeIP"] = nodeIP

	// Check if IP is reachable (basic connectivity check)
	reachable, latency := c.checkReachability(ctx, nodeIP)
	result.Details["reachable"] = reachable

	if !reachable {
		result.Status = health.HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Node IP %s not reachable", nodeIP)
		return result, nil
	}

	result.Details["latency"] = latency.String()

	// Check latency thresholds
	if latency > 500*time.Millisecond {
		result.Status = health.HealthStatusDegraded
		result.Message = fmt.Sprintf("High network latency: %s", latency)
		return result, nil
	}

	if latency > 200*time.Millisecond {
		result.Status = health.HealthStatusDegraded
		result.Message = fmt.Sprintf("Elevated network latency: %s", latency)
		return result, nil
	}

	result.Status = health.HealthStatusHealthy
	result.Message = fmt.Sprintf("Network healthy, latency: %s", latency)
	return result, nil
}

// getNodeInternalIP extracts the internal IP address from node
func getNodeInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// checkReachability tests basic network reachability to an IP
func (c *checker) checkReachability(ctx context.Context, ip string) (bool, time.Duration) {
	start := time.Now()

	// Create context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, c.pingTimeout)
	defer cancel()

	// Attempt TCP connection to kubelet port (10250)
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(dialCtx, "tcp", fmt.Sprintf("%s:10250", ip))
	if err != nil {
		c.logger.V(1).Info("network check failed", "ip", ip, "error", err)
		return false, 0
	}
	defer conn.Close()

	latency := time.Since(start)
	return true, latency
}
