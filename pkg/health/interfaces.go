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

package health

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// HealthStatus represents the health state of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "Healthy"
	HealthStatusDegraded  HealthStatus = "Degraded"
	HealthStatusUnhealthy HealthStatus = "Unhealthy"
	HealthStatusUnknown   HealthStatus = "Unknown"
)

// CheckResult contains the result of a health check
type CheckResult struct {
	Status    HealthStatus
	Message   string
	Timestamp time.Time
	Details   map[string]interface{}
}

// Checker defines the interface for health checking components
// Following ISP (Interface Segregation Principle) - single focused interface
type Checker interface {
	// Check performs a health check and returns the result
	Check(ctx context.Context, node *corev1.Node) (*CheckResult, error)

	// Name returns the unique name of this checker
	Name() string
}

// Provider aggregates multiple health checkers and provides overall health status
// Following SRP (Single Responsibility Principle) - manages checker orchestration
type Provider interface {
	// RegisterChecker adds a health checker to the provider
	RegisterChecker(checker Checker) error

	// CheckNode runs all registered checkers against a node
	CheckNode(ctx context.Context, node *corev1.Node) ([]*CheckResult, error)

	// GetOverallStatus computes the overall health status from individual check results
	GetOverallStatus(results []*CheckResult) HealthStatus
}
