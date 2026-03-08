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

package placement

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Decision represents a placement decision for a workload
type Decision struct {
	TargetNode     string
	Reason         string
	Score          float64
	Region         string
	AlternateNodes []string
}

// Constraint represents a placement constraint from RegionalWorkload spec
type Constraint struct {
	// RegionPreference specifies preferred regions in priority order
	RegionPreference []string
	
	// AvoidNodes lists node names to avoid
	AvoidNodes []string
	
	// RequireLabels specifies required node labels
	RequireLabels map[string]string
	
	// ResourceRequirements specifies minimum resource requirements
	ResourceRequirements corev1.ResourceRequirements
	
	// TierPreference specifies preferred tier levels
	TierPreference []string
}

// Strategy defines the interface for workload placement strategies
// Following Strategy pattern (OCP - Open Closed Principle)
type Strategy interface {
	// Name returns the unique name of this strategy
	Name() string
	
	// Score calculates placement score for a node given constraints
	// Higher score means better fit. Returns 0 if node is unsuitable.
	Score(ctx context.Context, node *corev1.Node, constraints *Constraint) (float64, error)
}

// Engine coordinates multiple placement strategies to make final decisions
// Following SRP (Single Responsibility Principle) - orchestrates strategies
type Engine interface {
	// RegisterStrategy adds a placement strategy to the engine
	RegisterStrategy(strategy Strategy, weight float64) error
	
	// SelectNode chooses the best node for a workload given constraints
	SelectNode(ctx context.Context, nodes []*corev1.Node, constraints *Constraint) (*Decision, error)
	
	// ValidatePlacement checks if current placement still satisfies constraints
	ValidatePlacement(ctx context.Context, pod *corev1.Pod, constraints *Constraint) (bool, error)
}

// WorkloadSpec represents the workload specification for placement
type WorkloadSpec struct {
	Name         string
	Namespace    string
	Labels       map[string]string
	Replicas     int32
	PodTemplate  corev1.PodTemplateSpec
	Constraints  *Constraint
	CreationTime metav1.Time
}
