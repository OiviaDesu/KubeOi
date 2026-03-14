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

package kubelet

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/oiviadesu/oiviak3s-operator/pkg/health"
	corev1 "k8s.io/api/core/v1"
)

// checker implements health.Checker for kubelet status
// Checks node Ready condition and kubelet responsiveness
type checker struct {
	logger logr.Logger
}

// NewChecker creates a new kubelet health checker
func NewChecker(logger logr.Logger) health.Checker {
	return &checker{
		logger: logger,
	}
}

// Name returns the unique name of this checker
func (c *checker) Name() string {
	return "kubelet"
}

// Check performs kubelet health check
func (c *checker) Check(ctx context.Context, node *corev1.Node) (*health.CheckResult, error) {
	result := &health.CheckResult{
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check node Ready condition
	readyCondition := getNodeCondition(node, corev1.NodeReady)
	if readyCondition == nil {
		result.Status = health.HealthStatusUnknown
		result.Message = "Ready condition not found"
		return result, nil
	}

	result.Details["readyStatus"] = string(readyCondition.Status)
	result.Details["readyReason"] = readyCondition.Reason
	result.Details["lastHeartbeat"] = readyCondition.LastHeartbeatTime.String()

	// Check if node is Ready
	if readyCondition.Status != corev1.ConditionTrue {
		result.Status = health.HealthStatusUnhealthy
		result.Message = fmt.Sprintf("Node not ready: %s", readyCondition.Reason)
		return result, nil
	}

	// Check for other concerning conditions
	if hasProblematicConditions(node, result.Details) {
		result.Status = health.HealthStatusDegraded
		result.Message = "Node has problematic conditions"
		return result, nil
	}

	// Check heartbeat staleness
	heartbeatAge := time.Since(readyCondition.LastHeartbeatTime.Time)
	result.Details["heartbeatAge"] = heartbeatAge.String()

	if heartbeatAge > 40*time.Second {
		result.Status = health.HealthStatusDegraded
		result.Message = fmt.Sprintf("Kubelet heartbeat stale: %s", heartbeatAge)
		return result, nil
	}

	result.Status = health.HealthStatusHealthy
	result.Message = "Kubelet healthy and responsive"
	return result, nil
}

// getNodeCondition finds a specific condition in node status
func getNodeCondition(node *corev1.Node, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == conditionType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}

// hasProblematicConditions checks for conditions indicating issues
func hasProblematicConditions(node *corev1.Node, details map[string]interface{}) bool {
	problematic := false

	// Check MemoryPressure
	if cond := getNodeCondition(node, corev1.NodeMemoryPressure); cond != nil && cond.Status == corev1.ConditionTrue {
		details["memoryPressure"] = true
		problematic = true
	}

	// Check DiskPressure
	if cond := getNodeCondition(node, corev1.NodeDiskPressure); cond != nil && cond.Status == corev1.ConditionTrue {
		details["diskPressure"] = true
		problematic = true
	}

	// Check PIDPressure
	if cond := getNodeCondition(node, corev1.NodePIDPressure); cond != nil && cond.Status == corev1.ConditionTrue {
		details["pidPressure"] = true
		problematic = true
	}

	// Check NetworkUnavailable
	if cond := getNodeCondition(node, corev1.NodeNetworkUnavailable); cond != nil && cond.Status == corev1.ConditionTrue {
		details["networkUnavailable"] = true
		problematic = true
	}

	return problematic
}
