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
)

const (
	// LabelTier is the label key for node tier
	LabelTier = "oiviak3s.io/tier"

	// LabelPowerStability is the label key for power stability rating
	LabelPowerStability = "oiviak3s.io/power-stability"
)

// tier implements placement.Strategy for tier-aware placement
// Considers node tiers (primary/secondary/tertiary) and power stability
type tier struct{}

// NewTier creates a new tier-aware placement strategy
func NewTier() placement.Strategy {
	return &tier{}
}

// Name returns the unique name of this strategy
func (s *tier) Name() string {
	return "tier"
}

// Score calculates placement score based on tier preferences and stability
func (s *tier) Score(ctx context.Context, node *corev1.Node, constraints *placement.Constraint) (float64, error) {
	baseScore := 50.0

	// Check if node is in avoid list
	if constraints != nil && len(constraints.AvoidNodes) > 0 {
		for _, avoidNode := range constraints.AvoidNodes {
			if node.Name == avoidNode {
				return 0, nil
			}
		}
	}

	// Check required labels
	if constraints != nil && len(constraints.RequireLabels) > 0 {
		for key, value := range constraints.RequireLabels {
			nodeValue, exists := node.Labels[key]
			if !exists || nodeValue != value {
				return 0, nil
			}
		}
	}

	// Score based on tier preference
	if constraints != nil && len(constraints.TierPreference) > 0 {
		nodeTier, hasTier := node.Labels[LabelTier]
		if hasTier {
			for i, preferredTier := range constraints.TierPreference {
				if nodeTier == preferredTier {
					// Earlier in preference list = higher score
					tierScore := 100.0 - (float64(i) * 25.0)
					if tierScore < 25.0 {
						tierScore = 25.0
					}
					baseScore = tierScore
					break
				}
			}
		}
	} else {
		// No tier preference, score based on tier quality
		nodeTier, hasTier := node.Labels[LabelTier]
		if hasTier {
			switch nodeTier {
			case "primary":
				baseScore = 90.0
			case "secondary":
				baseScore = 70.0
			case "tertiary":
				baseScore = 50.0
			}
		}
	}

	// Adjust score based on power stability
	powerStability, hasStability := node.Labels[LabelPowerStability]
	if hasStability {
		switch powerStability {
		case "high":
			baseScore *= 1.1 // 10% boost
		case "medium":
			baseScore *= 1.0 // No change
		case "low":
			baseScore *= 0.8 // 20% penalty
		}
	}

	// Cap score at 100
	if baseScore > 100 {
		baseScore = 100
	}

	return baseScore, nil
}
