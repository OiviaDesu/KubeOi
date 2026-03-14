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

package strategy

import (
	"context"

	"github.com/oiviadesu/oiviak3s-operator/pkg/placement"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// resourceAware implements placement.Strategy for resource-aware placement
// Scores nodes based on available resources relative to requirements
type resourceAware struct{}

// NewResourceAware creates a new resource-aware placement strategy
func NewResourceAware() placement.Strategy {
	return &resourceAware{}
}

// Name returns the unique name of this strategy
func (s *resourceAware) Name() string {
	return "resource"
}

// Score calculates placement score based on resource availability
func (s *resourceAware) Score(ctx context.Context, node *corev1.Node, constraints *placement.Constraint) (float64, error) {
	if constraints == nil {
		// No constraints, score based on general availability
		return s.scoreGeneralResources(node), nil
	}

	// Check if node meets minimum requirements
	if !s.meetsRequirements(node, constraints) {
		return 0, nil
	}

	// Score based on how much resources are available beyond requirements
	cpuScore := s.scoreResource(
		node.Status.Allocatable.Cpu(),
		constraints.ResourceRequirements.Requests.Cpu(),
	)

	memScore := s.scoreResource(
		node.Status.Allocatable.Memory(),
		constraints.ResourceRequirements.Requests.Memory(),
	)

	// Average CPU and memory scores
	totalScore := (cpuScore + memScore) / 2.0

	return totalScore, nil
}

// meetsRequirements checks if node has minimum required resources
func (s *resourceAware) meetsRequirements(node *corev1.Node, constraints *placement.Constraint) bool {
	if constraints.ResourceRequirements.Requests.Cpu() != nil {
		required := constraints.ResourceRequirements.Requests.Cpu().MilliValue()
		available := node.Status.Allocatable.Cpu().MilliValue()
		if available < required {
			return false
		}
	}

	if constraints.ResourceRequirements.Requests.Memory() != nil {
		required := constraints.ResourceRequirements.Requests.Memory().Value()
		available := node.Status.Allocatable.Memory().Value()
		if available < required {
			return false
		}
	}

	return true
}

// scoreResource scores a specific resource type
func (s *resourceAware) scoreResource(available, required *resource.Quantity) float64 {
	if available == nil || available.IsZero() {
		return 0
	}

	availableVal := available.AsApproximateFloat64()

	// If no requirement specified, score based on total availability
	if required == nil || required.IsZero() {
		// Normalize to 0-100 scale (assume 100 cores is max for scoring)
		score := (availableVal / 100.0) * 100.0
		if score > 100 {
			score = 100
		}
		return score
	}

	requiredVal := required.AsApproximateFloat64()
	ratio := availableVal / requiredVal

	// Score based on how much headroom is available
	// 1x requirement = 20, 2x = 50, 4x = 80, 8x+ = 100
	if ratio < 1 {
		return 0
	} else if ratio < 2 {
		return 20 + ((ratio - 1) * 30)
	} else if ratio < 4 {
		return 50 + ((ratio - 2) * 15)
	} else if ratio < 8 {
		return 80 + ((ratio - 4) * 5)
	}

	return 100
}

// scoreGeneralResources provides a general resource availability score
func (s *resourceAware) scoreGeneralResources(node *corev1.Node) float64 {
	cpuScore := s.scoreResource(node.Status.Allocatable.Cpu(), nil)
	memScore := s.scoreResource(node.Status.Allocatable.Memory(), nil)

	return (cpuScore + memScore) / 2.0
}
