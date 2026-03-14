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

package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/oiviadesu/oiviak3s-operator/pkg/health"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// checker implements health.Checker for node resource availability
// Checks CPU, memory, and storage capacity
type checker struct {
	logger               logr.Logger
	cpuThresholdPercent  float64
	memThresholdPercent  float64
	diskThresholdPercent float64
}

// Config holds configuration for resource checker
type Config struct {
	CPUThresholdPercent  float64
	MemThresholdPercent  float64
	DiskThresholdPercent float64
}

// NewChecker creates a new resource health checker
func NewChecker(logger logr.Logger, cfg Config) health.Checker {
	// Set defaults if not provided
	if cfg.CPUThresholdPercent == 0 {
		cfg.CPUThresholdPercent = 85.0
	}
	if cfg.MemThresholdPercent == 0 {
		cfg.MemThresholdPercent = 85.0
	}
	if cfg.DiskThresholdPercent == 0 {
		cfg.DiskThresholdPercent = 90.0
	}

	return &checker{
		logger:               logger,
		cpuThresholdPercent:  cfg.CPUThresholdPercent,
		memThresholdPercent:  cfg.MemThresholdPercent,
		diskThresholdPercent: cfg.DiskThresholdPercent,
	}
}

// Name returns the unique name of this checker
func (c *checker) Name() string {
	return "resource"
}

// Check performs resource availability check
func (c *checker) Check(ctx context.Context, node *corev1.Node) (*health.CheckResult, error) {
	result := &health.CheckResult{
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check CPU capacity and allocatable
	cpuStatus := c.checkResource(
		node.Status.Capacity.Cpu(),
		node.Status.Allocatable.Cpu(),
		c.cpuThresholdPercent,
		"CPU",
		result.Details,
	)

	// Check memory capacity and allocatable
	memStatus := c.checkResource(
		node.Status.Capacity.Memory(),
		node.Status.Allocatable.Memory(),
		c.memThresholdPercent,
		"Memory",
		result.Details,
	)

	// Check ephemeral storage
	storageStatus := c.checkResource(
		node.Status.Capacity.StorageEphemeral(),
		node.Status.Allocatable.StorageEphemeral(),
		c.diskThresholdPercent,
		"Storage",
		result.Details,
	)

	// Determine overall status
	if cpuStatus == health.HealthStatusUnhealthy ||
		memStatus == health.HealthStatusUnhealthy ||
		storageStatus == health.HealthStatusUnhealthy {
		result.Status = health.HealthStatusUnhealthy
		result.Message = "Node resources critically low"
		return result, nil
	}

	if cpuStatus == health.HealthStatusDegraded ||
		memStatus == health.HealthStatusDegraded ||
		storageStatus == health.HealthStatusDegraded {
		result.Status = health.HealthStatusDegraded
		result.Message = "Node resources under pressure"
		return result, nil
	}

	result.Status = health.HealthStatusHealthy
	result.Message = "Node resources healthy"
	return result, nil
}

// checkResource evaluates a specific resource type
func (c *checker) checkResource(
	capacity *resource.Quantity,
	allocatable *resource.Quantity,
	threshold float64,
	resourceName string,
	details map[string]interface{},
) health.HealthStatus {
	if capacity == nil || allocatable == nil {
		return health.HealthStatusUnknown
	}

	capacityVal := capacity.AsApproximateFloat64()
	allocatableVal := allocatable.AsApproximateFloat64()

	if capacityVal == 0 {
		return health.HealthStatusUnknown
	}

	// Calculate percentage of capacity that is allocatable
	allocatablePercent := (allocatableVal / capacityVal) * 100

	details[fmt.Sprintf("%sCapacity", resourceName)] = capacity.String()
	details[fmt.Sprintf("%sAllocatable", resourceName)] = allocatable.String()
	details[fmt.Sprintf("%sAllocatablePercent", resourceName)] = fmt.Sprintf("%.2f%%", allocatablePercent)

	// If allocatable is too low compared to capacity, resources are consumed
	remainingPercent := allocatablePercent

	if remainingPercent < (100 - threshold) {
		return health.HealthStatusUnhealthy
	}

	if remainingPercent < (100 - (threshold * 0.7)) {
		return health.HealthStatusDegraded
	}

	return health.HealthStatusHealthy
}
