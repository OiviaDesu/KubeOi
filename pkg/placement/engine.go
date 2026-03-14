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
	"fmt"
	"sort"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// strategyEntry holds a strategy and its weight
type strategyEntry struct {
	strategy Strategy
	weight   float64
}

// engine implements the Engine interface
// Following Composite pattern for strategy aggregation
type engine struct {
	strategies []strategyEntry
	mu         sync.RWMutex
	logger     logr.Logger
}

// NewEngine creates a new placement engine
func NewEngine(logger logr.Logger) Engine {
	return &engine{
		strategies: make([]strategyEntry, 0),
		logger:     logger,
	}
}

// RegisterStrategy adds a placement strategy to the engine
func (e *engine) RegisterStrategy(strategy Strategy, weight float64) error {
	if strategy == nil {
		return fmt.Errorf("strategy cannot be nil")
	}
	if weight < 0 {
		return fmt.Errorf("weight must be non-negative, got %f", weight)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Check for duplicate strategy names
	for _, existing := range e.strategies {
		if existing.strategy.Name() == strategy.Name() {
			return fmt.Errorf("strategy with name %s already registered", strategy.Name())
		}
	}

	e.strategies = append(e.strategies, strategyEntry{
		strategy: strategy,
		weight:   weight,
	})

	e.logger.Info("registered placement strategy", "strategy", strategy.Name(), "weight", weight)
	return nil
}

// SelectNode chooses the best node for a workload given constraints
func (e *engine) SelectNode(ctx context.Context, nodes []*corev1.Node, constraints *Constraint) (*Decision, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available for placement")
	}

	e.mu.RLock()
	strategies := make([]strategyEntry, len(e.strategies))
	copy(strategies, e.strategies)
	e.mu.RUnlock()

	if len(strategies) == 0 {
		return nil, fmt.Errorf("no placement strategies registered")
	}

	// Calculate weighted scores for each node
	type nodeScore struct {
		node  *corev1.Node
		score float64
	}

	nodeScores := make([]nodeScore, 0, len(nodes))

	for _, node := range nodes {
		totalScore := 0.0
		totalWeight := 0.0

		for _, entry := range strategies {
			score, err := entry.strategy.Score(ctx, node, constraints)
			if err != nil {
				e.logger.Error(err, "strategy scoring failed",
					"strategy", entry.strategy.Name(),
					"node", node.Name)
				continue
			}

			totalScore += score * entry.weight
			totalWeight += entry.weight
		}

		// Normalize by total weight
		if totalWeight > 0 {
			normalizedScore := totalScore / totalWeight
			if normalizedScore > 0 {
				nodeScores = append(nodeScores, nodeScore{
					node:  node,
					score: normalizedScore,
				})
			}
		}
	}

	if len(nodeScores) == 0 {
		return nil, fmt.Errorf("no suitable nodes found for placement")
	}

	// Sort by score descending
	sort.Slice(nodeScores, func(i, j int) bool {
		return nodeScores[i].score > nodeScores[j].score
	})

	// Build decision with top node and alternates
	alternates := make([]string, 0, len(nodeScores)-1)
	for i := 1; i < len(nodeScores) && i < 4; i++ {
		alternates = append(alternates, nodeScores[i].node.Name)
	}

	decision := &Decision{
		TargetNode:     nodeScores[0].node.Name,
		Reason:         fmt.Sprintf("highest weighted score: %.2f", nodeScores[0].score),
		Score:          nodeScores[0].score,
		AlternateNodes: alternates,
	}

	e.logger.Info("placement decision made",
		"targetNode", decision.TargetNode,
		"score", decision.Score,
		"alternates", len(alternates))

	return decision, nil
}

// ValidatePlacement checks if current placement still satisfies constraints
func (e *engine) ValidatePlacement(ctx context.Context, pod *corev1.Pod, constraints *Constraint) (bool, error) {
	if pod.Spec.NodeName == "" {
		return false, nil
	}

	e.mu.RLock()
	strategies := make([]strategyEntry, len(e.strategies))
	copy(strategies, e.strategies)
	e.mu.RUnlock()

	// Validate against each strategy
	// For validation, we just check if the node would score > 0
	node := &corev1.Node{}
	node.Name = pod.Spec.NodeName

	for _, entry := range strategies {
		score, err := entry.strategy.Score(ctx, node, constraints)
		if err != nil {
			e.logger.Error(err, "validation scoring failed",
				"strategy", entry.strategy.Name(),
				"pod", pod.Name)
			return false, err
		}

		if score <= 0 {
			e.logger.Info("placement validation failed",
				"pod", pod.Name,
				"node", node.Name,
				"strategy", entry.strategy.Name())
			return false, nil
		}
	}

	return true, nil
}
