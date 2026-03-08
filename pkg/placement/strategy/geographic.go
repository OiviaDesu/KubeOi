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
	// LabelRegion is the label key for node region
	LabelRegion = "oiviak3s.io/region"
)

// geographic implements placement.Strategy for geographic-aware placement
// Prefers nodes in specified regions based on RegionPreference
type geographic struct{}

// NewGeographic creates a new geographic placement strategy
func NewGeographic() placement.Strategy {
	return &geographic{}
}

// Name returns the unique name of this strategy
func (s *geographic) Name() string {
	return "geographic"
}

// Score calculates placement score based on geographic preferences
func (s *geographic) Score(ctx context.Context, node *corev1.Node, constraints *placement.Constraint) (float64, error) {
	if constraints == nil || len(constraints.RegionPreference) == 0 {
		// No region preference, all nodes score equally
		return 50.0, nil
	}
	
	// Get node region from label
	nodeRegion, hasRegion := node.Labels[LabelRegion]
	if !hasRegion {
		// Node has no region label, score low but not zero
		return 10.0, nil
	}
	
	// Find region in preference list
	for i, preferredRegion := range constraints.RegionPreference {
		if nodeRegion == preferredRegion {
			// Higher preference (earlier in list) gets higher score
			// First preference: 100, Second: 80, Third: 60, etc.
			score := 100.0 - (float64(i) * 20.0)
			if score < 20.0 {
				score = 20.0
			}
			return score, nil
		}
	}
	
	// Region not in preference list, but node is labeled
	return 15.0, nil
}
