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
	"github.com/oiviadesu/oiviak3s-operator/pkg/placement"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RegionalWorkloadReconciler reconciles a RegionalWorkload object
// Following DIP (Dependency Inversion Principle) - depends on placement.Engine interface
type RegionalWorkloadReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	PlacementEngine placement.Engine
	Logger          logr.Logger
}

// NewRegionalWorkloadReconciler creates a new reconciler with dependencies injected
func NewRegionalWorkloadReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	placementEngine placement.Engine,
	logger logr.Logger,
) *RegionalWorkloadReconciler {
	return &RegionalWorkloadReconciler{
		Client:          client,
		Scheme:          scheme,
		PlacementEngine: placementEngine,
		Logger:          logger,
	}
}

// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=regionalworkloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=regionalworkloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=regionalworkloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=nodehealthstatuses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile performs the reconciliation logic for RegionalWorkload
func (r *RegionalWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	
	// Fetch the RegionalWorkload resource
	workload := &geov1alpha1.RegionalWorkload{}
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch RegionalWorkload")
		return ctrl.Result{}, err
	}
	
	// Get all healthy nodes
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		logger.Error(err, "unable to list nodes")
		return ctrl.Result{}, err
	}
	
	// Get node health statuses
	healthList := &geov1alpha1.NodeHealthStatusList{}
	if err := r.List(ctx, healthList); err != nil {
		logger.Error(err, "unable to list NodeHealthStatus")
		return ctrl.Result{}, err
	}
	
	// Filter healthy nodes
	healthyNodes := r.filterHealthyNodes(nodeList.Items, healthList.Items)
	
	// Create placement constraint from spec
	constraint := &placement.Constraint{
		RegionPreference: workload.Spec.PlacementConstraints.RegionPreference,
		AvoidNodes:       workload.Spec.PlacementConstraints.AvoidNodes,
		RequireLabels:    workload.Spec.PlacementConstraints.RequireLabels,
		TierPreference:   workload.Spec.PlacementConstraints.TierPreference,
	}
	
	// Select optimal node placement
	decision, err := r.PlacementEngine.SelectNode(ctx, healthyNodes, constraint)
	if err != nil {
		logger.Error(err, "placement decision failed")
		r.updateStatusError(workload, err)
		if err := r.Status().Update(ctx, workload); err != nil {
			logger.Error(err, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	
	// Update placement status
	r.updatePlacementStatus(workload, decision)
	
	// Apply placement to target workload
	if err := r.applyPlacement(ctx, workload, decision); err != nil {
		logger.Error(err, "failed to apply placement")
		return ctrl.Result{}, err
	}
	
	// Update workload health
	if err := r.updateWorkloadHealth(ctx, workload); err != nil {
		logger.Error(err, "failed to update workload health")
	}
	
	// Update status
	if err := r.Status().Update(ctx, workload); err != nil {
		logger.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}
	
	logger.Info("reconciliation complete",
		"workload", workload.Name,
		"targetNode", decision.TargetNode,
		"region", decision.Region)
	
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// filterHealthyNodes filters nodes based on health status
func (r *RegionalWorkloadReconciler) filterHealthyNodes(
	nodes []corev1.Node,
	healthStatuses []geov1alpha1.NodeHealthStatus,
) []*corev1.Node {
	healthMap := make(map[string]string)
	for _, hs := range healthStatuses {
		healthMap[hs.Spec.NodeName] = hs.Status.OverallStatus
	}
	
	healthy := make([]*corev1.Node, 0, len(nodes))
	for i := range nodes {
		status, exists := healthMap[nodes[i].Name]
		if !exists || status == "Healthy" {
			healthy = append(healthy, &nodes[i])
		}
	}
	
	return healthy
}

// updatePlacementStatus updates the placement status
func (r *RegionalWorkloadReconciler) updatePlacementStatus(
	workload *geov1alpha1.RegionalWorkload,
	decision *placement.Decision,
) {
	workload.Status.Placement = &geov1alpha1.PlacementDecision{
		NodeName:  decision.TargetNode,
		Region:    decision.Region,
		Reason:    decision.Reason,
		Score:     decision.Score,
		Timestamp: metav1.NewTime(time.Now()),
	}
	
	// Update conditions
	condition := metav1.Condition{
		Type:               "PlacementReady",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: workload.Generation,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "PlacementSuccessful",
		Message:            fmt.Sprintf("Workload placed on node %s in region %s", decision.TargetNode, decision.Region),
	}
	workload.Status.Conditions = []metav1.Condition{condition}
}

// updateStatusError updates status on error
func (r *RegionalWorkloadReconciler) updateStatusError(
	workload *geov1alpha1.RegionalWorkload,
	err error,
) {
	condition := metav1.Condition{
		Type:               "PlacementReady",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: workload.Generation,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "PlacementFailed",
		Message:            fmt.Sprintf("Failed to place workload: %v", err),
	}
	workload.Status.Conditions = []metav1.Condition{condition}
}

// applyPlacement applies the placement decision to the target workload
func (r *RegionalWorkloadReconciler) applyPlacement(
	ctx context.Context,
	workload *geov1alpha1.RegionalWorkload,
	decision *placement.Decision,
) error {
	logger := log.FromContext(ctx)
	
	// Get target workload reference
	ref := workload.Spec.WorkloadRef
	if ref.Kind == "" || ref.Name == "" {
		return fmt.Errorf("invalid workload reference")
	}
	
	switch ref.Kind {
	case "Deployment":
		return r.applyToDeployment(ctx, workload.Namespace, ref.Name, decision.TargetNode)
	case "StatefulSet":
		return r.applyToStatefulSet(ctx, workload.Namespace, ref.Name, decision.TargetNode)
	default:
		logger.Info("unsupported workload kind", "kind", ref.Kind)
		return fmt.Errorf("unsupported workload kind: %s", ref.Kind)
	}
}

// applyToDeployment applies nodeSelector to a Deployment
func (r *RegionalWorkloadReconciler) applyToDeployment(
	ctx context.Context,
	namespace, name, nodeName string,
) error {
	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	
	if err := r.Get(ctx, key, deployment); err != nil {
		return err
	}
	
	// Update nodeSelector
	if deployment.Spec.Template.Spec.NodeSelector == nil {
		deployment.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"] = nodeName
	
	return r.Update(ctx, deployment)
}

// applyToStatefulSet applies nodeSelector to a StatefulSet
func (r *RegionalWorkloadReconciler) applyToStatefulSet(
	ctx context.Context,
	namespace, name, nodeName string,
) error {
	statefulSet := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	
	if err := r.Get(ctx, key, statefulSet); err != nil {
		return err
	}
	
	// Update nodeSelector
	if statefulSet.Spec.Template.Spec.NodeSelector == nil {
		statefulSet.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	statefulSet.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"] = nodeName
	
	return r.Update(ctx, statefulSet)
}

// updateWorkloadHealth updates the workload health status
func (r *RegionalWorkloadReconciler) updateWorkloadHealth(
	ctx context.Context,
	workload *geov1alpha1.RegionalWorkload,
) error {
	ref := workload.Spec.WorkloadRef
	
	var ready, desired int32
	var err error
	
	switch ref.Kind {
	case "Deployment":
		ready, desired, err = r.getDeploymentStatus(ctx, workload.Namespace, ref.Name)
	case "StatefulSet":
		ready, desired, err = r.getStatefulSetStatus(ctx, workload.Namespace, ref.Name)
	default:
		return nil
	}
	
	if err != nil {
		return err
	}
	
	workload.Status.Health.ReadyReplicas = ready
	workload.Status.Health.DesiredReplicas = desired
	
	if ready == desired && ready > 0 {
		workload.Status.Health.Status = "Healthy"
	} else if ready > 0 {
		workload.Status.Health.Status = "Degraded"
	} else {
		workload.Status.Health.Status = "Unhealthy"
	}
	
	workload.Status.Health.LastCheckTime = metav1.NewTime(time.Now())
	
	return nil
}

// getDeploymentStatus gets deployment replica status
func (r *RegionalWorkloadReconciler) getDeploymentStatus(
	ctx context.Context,
	namespace, name string,
) (ready, desired int32, err error) {
	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	
	if err := r.Get(ctx, key, deployment); err != nil {
		return 0, 0, err
	}
	
	return deployment.Status.ReadyReplicas, *deployment.Spec.Replicas, nil
}

// getStatefulSetStatus gets statefulset replica status
func (r *RegionalWorkloadReconciler) getStatefulSetStatus(
	ctx context.Context,
	namespace, name string,
) (ready, desired int32, err error) {
	statefulSet := &appsv1.StatefulSet{}
	key := types.NamespacedName{Namespace: namespace, Name: name}
	
	if err := r.Get(ctx, key, statefulSet); err != nil {
		return 0, 0, err
	}
	
	return statefulSet.Status.ReadyReplicas, *statefulSet.Spec.Replicas, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *RegionalWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&geov1alpha1.RegionalWorkload{}).
		Complete(r)
}
