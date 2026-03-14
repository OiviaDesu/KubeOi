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
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// registry implements the Provider interface
// Following OCP (Open-Closed Principle) - open for extension via RegisterChecker
type registry struct {
	checkers []Checker
	mu       sync.RWMutex
	logger   logr.Logger
}

// NewRegistry creates a new health check provider
func NewRegistry(logger logr.Logger) Provider {
	return &registry{
		checkers: make([]Checker, 0),
		logger:   logger,
	}
}

// RegisterChecker adds a health checker to the registry
func (r *registry) RegisterChecker(checker Checker) error {
	if checker == nil {
		return fmt.Errorf("checker cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate checker names
	for _, existing := range r.checkers {
		if existing.Name() == checker.Name() {
			return fmt.Errorf("checker with name %s already registered", checker.Name())
		}
	}

	r.checkers = append(r.checkers, checker)
	r.logger.Info("registered health checker", "checker", checker.Name())
	return nil
}

// CheckNode runs all registered checkers against a node
func (r *registry) CheckNode(ctx context.Context, node *corev1.Node) ([]*CheckResult, error) {
	r.mu.RLock()
	checkers := make([]Checker, len(r.checkers))
	copy(checkers, r.checkers)
	r.mu.RUnlock()

	results := make([]*CheckResult, 0, len(checkers))

	for _, checker := range checkers {
		result, err := checker.Check(ctx, node)
		if err != nil {
			r.logger.Error(err, "health check failed", "checker", checker.Name(), "node", node.Name)
			// Create a failed result instead of returning error (graceful degradation)
			result = &CheckResult{
				Status:    HealthStatusUnknown,
				Message:   fmt.Sprintf("check failed: %v", err),
				Timestamp: time.Now(),
				Details:   map[string]interface{}{"error": err.Error(), "checker": checker.Name()},
			}
		}
		result = normalizeCheckResult(result, checker.Name())
		results = append(results, result)
	}

	return results, nil
}

// GetOverallStatus computes the overall health status from individual check results
func (r *registry) GetOverallStatus(results []*CheckResult) HealthStatus {
	if len(results) == 0 {
		return HealthStatusUnknown
	}

	hasUnhealthy := false
	hasDegraded := false
	hasUnknown := false

	for _, result := range results {
		if result == nil {
			hasUnknown = true
			continue
		}

		switch result.Status {
		case HealthStatusUnhealthy:
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		case HealthStatusUnknown:
			hasUnknown = true
		}
	}

	// Worst status wins
	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}
	if hasUnknown {
		return HealthStatusUnknown
	}

	return HealthStatusHealthy
}

func normalizeCheckResult(result *CheckResult, checkerName string) *CheckResult {
	if result == nil {
		return &CheckResult{
			Status:    HealthStatusUnknown,
			Message:   "checker returned nil result",
			Timestamp: time.Now(),
			Details:   map[string]interface{}{"checker": checkerName},
		}
	}

	if result.Timestamp.IsZero() {
		result.Timestamp = time.Now()
	}
	if result.Details == nil {
		result.Details = make(map[string]interface{})
	}
	if _, exists := result.Details["checker"]; !exists {
		result.Details["checker"] = checkerName
	}

	return result
}
