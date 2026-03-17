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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	geov1alpha1 "github.com/oiviadesu/oiviak3s-operator/api/v1alpha1"
	"github.com/oiviadesu/oiviak3s-operator/pkg/notification"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// FailoverPolicyReconciler reconciles a FailoverPolicy object
// Following DIP (Dependency Inversion Principle) - depends on notification.Manager interface
type FailoverPolicyReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	NotificationManager notification.Manager
	Logger              logr.Logger
}

// NewFailoverPolicyReconciler creates a new reconciler with dependencies injected
func NewFailoverPolicyReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	notificationManager notification.Manager,
	logger logr.Logger,
) *FailoverPolicyReconciler {
	return &FailoverPolicyReconciler{
		Client:              client,
		Scheme:              scheme,
		NotificationManager: notificationManager,
		Logger:              logger,
	}
}

// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=failoverpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=failoverpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=failoverpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=nodehealthstatuses,verbs=get;list;watch
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=regionalworkloads,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=update;patch

// Reconcile performs the reconciliation logic for FailoverPolicy
func (r *FailoverPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the FailoverPolicy resource
	policy := &geov1alpha1.FailoverPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch FailoverPolicy")
		return ctrl.Result{}, err
	}

	// Check if policy is enabled
	if !policy.Spec.Enabled {
		logger.Info("policy is disabled", "policy", policy.Name)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// Get all node health statuses
	healthList := &geov1alpha1.NodeHealthStatusList{}
	if err := r.List(ctx, healthList); err != nil {
		logger.Error(err, "unable to list NodeHealthStatus")
		return ctrl.Result{}, err
	}

	// Get all regional workloads
	workloadList := &geov1alpha1.RegionalWorkloadList{}
	if err := r.List(ctx, workloadList); err != nil {
		logger.Error(err, "unable to list RegionalWorkload")
		return ctrl.Result{}, err
	}

	// Check for failover triggers
	triggered, reason := r.checkFailoverTriggers(policy, healthList.Items, workloadList.Items)

	if triggered {
		logger.Info("failover triggered", "reason", reason)

		// Send notification if enabled
		if policy.Spec.NotificationRule.OnFailoverStart {
			r.sendNotification(ctx, policy, "Failover Triggered", reason, notification.SeverityCritical)
		}

		// Execute failover based on strategy
		if err := r.executeFailover(ctx, policy, workloadList.Items, reason); err != nil {
			logger.Error(err, "failover execution failed")

			if policy.Spec.NotificationRule.OnFailoverFailed {
				r.sendNotification(ctx, policy, "Failover Failed", err.Error(), notification.SeverityCritical)
			}

			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		// Send success notification
		if policy.Spec.NotificationRule.OnFailoverComplete {
			r.sendNotification(ctx, policy, "Failover Completed", "Workloads successfully failed over", notification.SeverityInfo)
		}

		// Record failover event
		r.recordFailoverEvent(policy, reason, "Success")
		meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
			Type:               "FailoverReady",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: policy.Generation,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Reason:             "FailoverCompleted",
			Message:            "Failover processed successfully",
		})

		// Update status
		if err := r.Status().Update(ctx, policy); err != nil {
			logger.Error(err, "unable to update status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// checkFailoverTriggers checks if any failover trigger conditions are met
func (r *FailoverPolicyReconciler) checkFailoverTriggers(
	policy *geov1alpha1.FailoverPolicy,
	healthStatuses []geov1alpha1.NodeHealthStatus,
	workloads []geov1alpha1.RegionalWorkload,
) (bool, string) {
	triggers := policy.Spec.Trigger

	// Check node unhealthy duration trigger
	if triggers.NodeUnhealthyDuration.Duration > 0 {
		for _, health := range healthStatuses {
			if health.Status.OverallStatus == "Unhealthy" {
				unhealthyDuration := time.Since(health.Status.LastTransitionTime.Time)
				if unhealthyDuration >= triggers.NodeUnhealthyDuration.Duration {
					return true, fmt.Sprintf("Node %s unhealthy for %v", health.Spec.NodeName, unhealthyDuration)
				}
			}
		}
	}

	// Check workload unhealthy duration trigger
	if triggers.WorkloadUnhealthyDuration.Duration > 0 {
		for _, workload := range workloads {
			if workload.Status.Health.Status == "Unhealthy" {
				unhealthyDuration := time.Since(workload.Status.Health.LastCheckTime.Time)
				if unhealthyDuration >= triggers.WorkloadUnhealthyDuration.Duration {
					return true, fmt.Sprintf("Workload %s/%s unhealthy for %v",
						workload.Namespace, workload.Name, unhealthyDuration)
				}
			}
		}
	}

	// Check regional outage trigger
	if triggers.RegionalOutage {
		unhealthyByRegion := make(map[string]int)
		totalByRegion := make(map[string]int)

		for _, health := range healthStatuses {
			region := health.Status.Region
			if region == "" {
				continue
			}
			totalByRegion[region]++
			if health.Status.OverallStatus == "Unhealthy" {
				unhealthyByRegion[region]++
			}
		}

		for region, unhealthy := range unhealthyByRegion {
			total := totalByRegion[region]
			if total > 0 && unhealthy == total {
				return true, fmt.Sprintf("Regional outage in %s: all %d nodes unhealthy",
					region, total)
			}
		}
	}

	return false, ""
}

// executeFailover executes the failover strategy
func (r *FailoverPolicyReconciler) executeFailover(
	ctx context.Context,
	policy *geov1alpha1.FailoverPolicy,
	workloads []geov1alpha1.RegionalWorkload,
	reason string,
) error {
	logger := log.FromContext(ctx)
	strategy := policy.Spec.Strategy

	switch strategy.Type {
	case geov1alpha1.FailoverImmediate:
		return r.executeImmediateFailover(ctx, workloads)
	case geov1alpha1.FailoverGraceful:
		return r.executeGracefulFailover(ctx, workloads, strategy.DrainTimeout.Duration)
	case geov1alpha1.FailoverManual:
		logger.Info("manual failover required - awaiting operator intervention", "reason", reason)
		return nil
	default:
		return fmt.Errorf("unsupported failover strategy: %s", strategy.Type)
	}
}

// executeImmediateFailover performs immediate failover without draining
func (r *FailoverPolicyReconciler) executeImmediateFailover(
	ctx context.Context,
	workloads []geov1alpha1.RegionalWorkload,
) error {
	logger := log.FromContext(ctx)

	for _, workload := range workloads {
		// Trigger workload reconciliation by updating annotation
		annotations := workload.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["geo.oiviak3s.io/failover-triggered"] = time.Now().Format(time.RFC3339)
		workload.SetAnnotations(annotations)

		if err := r.Update(ctx, &workload); err != nil {
			logger.Error(err, "failed to trigger workload failover", "workload", workload.Name)
			return err
		}

		logger.Info("triggered immediate failover", "workload", workload.Name)
	}

	return nil
}

// executeGracefulFailover performs graceful failover with node draining
func (r *FailoverPolicyReconciler) executeGracefulFailover(
	ctx context.Context,
	workloads []geov1alpha1.RegionalWorkload,
	drainTimeout time.Duration,
) error {
	logger := log.FromContext(ctx)

	// Get affected nodes
	affectedNodes := make(map[string]bool)
	for _, workload := range workloads {
		if workload.Status.Placement != nil && workload.Status.Placement.NodeName != "" {
			affectedNodes[workload.Status.Placement.NodeName] = true
		}
	}

	// Cordon affected nodes
	for nodeName := range affectedNodes {
		node := &corev1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
			logger.Error(err, "unable to get node", "node", nodeName)
			continue
		}

		// Mark node as unschedulable
		node.Spec.Unschedulable = true
		if err := r.Update(ctx, node); err != nil {
			logger.Error(err, "failed to cordon node", "node", nodeName)
			return err
		}

		logger.Info("cordoned node for graceful failover", "node", nodeName)
	}

	// Wait for drain timeout
	if err := waitForDrainTimeout(ctx, drainTimeout); err != nil {
		return err
	}

	// Trigger workload reconciliation
	return r.executeImmediateFailover(ctx, workloads)
}

// sendNotification sends a notification
func (r *FailoverPolicyReconciler) sendNotification(
	ctx context.Context,
	policy *geov1alpha1.FailoverPolicy,
	title, message string,
	severity notification.Severity,
) {
	logger := log.FromContext(ctx)

	// Check minimum severity
	minSeverity := notification.Severity(policy.Spec.NotificationRule.MinSeverity)
	if severityLevel(severity) < severityLevel(minSeverity) {
		return
	}

	event := notification.Event{
		Title:     title,
		Message:   message,
		Source:    fmt.Sprintf("FailoverPolicy/%s", policy.Name),
		Severity:  severity,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"policy": policy.Name,
		},
	}

	if err := r.NotificationManager.Notify(ctx, &event); err != nil {
		logger.Error(err, "failed to send notification")
	}
}

func waitForDrainTimeout(ctx context.Context, drainTimeout time.Duration) error {
	if drainTimeout <= 0 {
		return nil
	}

	timer := time.NewTimer(drainTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("drain interrupted: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func severityLevel(severity notification.Severity) int {
	switch severity {
	case notification.SeverityCritical:
		return 3
	case notification.SeverityWarning:
		return 2
	case notification.SeverityInfo:
		return 1
	default:
		return 0
	}
}

// recordFailoverEvent records a failover event in status
func (r *FailoverPolicyReconciler) recordFailoverEvent(
	policy *geov1alpha1.FailoverPolicy,
	reason string,
	success string,
) {
	event := geov1alpha1.FailoverEvent{
		Timestamp: metav1.NewTime(time.Now()),
		Reason:    reason,
		Success:   success == "Success",
	}

	// Keep only last 10 events
	policy.Status.RecentEvents = append([]geov1alpha1.FailoverEvent{event}, policy.Status.RecentEvents...)
	if len(policy.Status.RecentEvents) > 10 {
		policy.Status.RecentEvents = policy.Status.RecentEvents[:10]
	}

	policy.Status.LastFailoverTime = &event.Timestamp
}

// SetupWithManager sets up the controller with the Manager
func (r *FailoverPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&geov1alpha1.FailoverPolicy{}).
		Complete(r)
}
