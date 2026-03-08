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

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	geov1alpha1 "github.com/oiviadesu/oiviak3s-operator/api/v1alpha1"
	"github.com/oiviadesu/oiviak3s-operator/pkg/health"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NodeHealthStatusReconciler reconciles a NodeHealthStatus object
// Following DIP (Dependency Inversion Principle) - depends on health.Provider interface
type NodeHealthStatusReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	HealthProvider health.Provider
	Logger         logr.Logger
}

// NewNodeHealthStatusReconciler creates a new reconciler with dependencies injected
func NewNodeHealthStatusReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	healthProvider health.Provider,
	logger logr.Logger,
) *NodeHealthStatusReconciler {
	return &NodeHealthStatusReconciler{
		Client:         client,
		Scheme:         scheme,
		HealthProvider: healthProvider,
		Logger:         logger,
	}
}

// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=nodehealthstatuses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=nodehealthstatuses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=nodehealthstatuses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile performs the reconciliation logic for NodeHealthStatus
func (r *NodeHealthStatusReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	
	// Fetch the NodeHealthStatus resource
	nodeHealth := &geov1alpha1.NodeHealthStatus{}
	if err := r.Get(ctx, req.NamespacedName, nodeHealth); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NodeHealthStatus")
		return ctrl.Result{}, err
	}
	
	// Fetch the corresponding Node
	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: nodeHealth.Spec.NodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("node not found", "node", nodeHealth.Spec.NodeName)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		logger.Error(err, "unable to fetch Node")
		return ctrl.Result{}, err
	}
	
	// Perform health checks
	checkResults, err := r.HealthProvider.CheckNode(ctx, node)
	if err != nil {
		logger.Error(err, "health check failed", "node", node.Name)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	
	// Update status with check results
	oldStatus := nodeHealth.Status.OverallStatus
	r.updateStatus(nodeHealth, node, checkResults)
	
	// Update the resource status
	if err := r.Status().Update(ctx, nodeHealth); err != nil {
		logger.Error(err, "unable to update NodeHealthStatus status")
		return ctrl.Result{}, err
	}
	
	// Log status transitions
	if oldStatus != nodeHealth.Status.OverallStatus {
		logger.Info("node health status changed",
			"node", node.Name,
			"oldStatus", oldStatus,
			"newStatus", nodeHealth.Status.OverallStatus)
	}
	
	// Requeue based on configured interval
	interval := nodeHealth.Spec.CheckInterval.Duration
	if interval == 0 {
		interval = 30 * time.Second
	}
	
	return ctrl.Result{RequeueAfter: interval}, nil
}

// updateStatus updates the NodeHealthStatus status based on check results
func (r *NodeHealthStatusReconciler) updateStatus(
	nodeHealth *geov1alpha1.NodeHealthStatus,
	node *corev1.Node,
	checkResults []*health.CheckResult,
) {
	// Convert check results to status format
	checks := make([]geov1alpha1.HealthCheckStatus, 0, len(checkResults))
	for _, result := range checkResults {
		checkStatus := geov1alpha1.HealthCheckStatus{
			CheckerName:   result.Details["checker"].(string),
			Status:        string(result.Status),
			Message:       result.Message,
			LastCheckTime: metav1.NewTime(result.Timestamp),
			Details:       make(map[string]string),
		}
		
		// Convert details map
		for k, v := range result.Details {
			if str, ok := v.(string); ok {
				checkStatus.Details[k] = str
			}
		}
		
		checks = append(checks, checkStatus)
	}
	
	// Get overall status
	overallStatus := r.HealthProvider.GetOverallStatus(checkResults)
	
	// Check if status changed
	statusChanged := nodeHealth.Status.OverallStatus != string(overallStatus)
	
	// Update consecutive failures
	if overallStatus == health.HealthStatusUnhealthy {
		nodeHealth.Status.ConsecutiveFailures++
	} else {
		nodeHealth.Status.ConsecutiveFailures = 0
	}
	
	// Update status fields
	nodeHealth.Status.OverallStatus = string(overallStatus)
	nodeHealth.Status.Checks = checks
	
	if statusChanged {
		nodeHealth.Status.LastTransitionTime = metav1.NewTime(time.Now())
	}
	
	// Extract region and tier from node labels
	if region, ok := node.Labels["oiviak3s.io/region"]; ok {
		nodeHealth.Status.Region = region
	}
	if tier, ok := node.Labels["oiviak3s.io/tier"]; ok {
		nodeHealth.Status.Tier = tier
	}
	
	// Update conditions
	r.updateConditions(nodeHealth, overallStatus)
}

// updateConditions updates the status conditions
func (r *NodeHealthStatusReconciler) updateConditions(
	nodeHealth *geov1alpha1.NodeHealthStatus,
	status health.HealthStatus,
) {
	condition := metav1.Condition{
		Type:               "Healthy",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: nodeHealth.Generation,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "NodeHealthy",
		Message:            "Node is healthy",
	}
	
	switch status {
	case health.HealthStatusUnhealthy:
		condition.Status = metav1.ConditionFalse
		condition.Reason = "NodeUnhealthy"
		condition.Message = "Node is unhealthy"
	case health.HealthStatusDegraded:
		condition.Status = metav1.ConditionFalse
		condition.Reason = "NodeDegraded"
		condition.Message = "Node is degraded"
	case health.HealthStatusUnknown:
		condition.Status = metav1.ConditionUnknown
		condition.Reason = "NodeUnknown"
		condition.Message = "Node health is unknown"
	}
	
	// Update or append condition
	nodeHealth.Status.Conditions = []metav1.Condition{condition}
}

// SetupWithManager sets up the controller with the Manager
func (r *NodeHealthStatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&geov1alpha1.NodeHealthStatus{}).
		Complete(r)
}
