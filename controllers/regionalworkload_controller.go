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
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	geov1alpha1 "github.com/oiviadesu/oiviak3s-operator/api/v1alpha1"
	"github.com/oiviadesu/oiviak3s-operator/pkg/placement"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RegionalWorkloadReconciler reconciles a RegionalWorkload object
// Following DIP (Dependency Inversion Principle) - depends on placement.Engine interface
type RegionalWorkloadReconciler struct {
	client.Client
	Scheme                            *runtime.Scheme
	PlacementEngine                   placement.Engine
	Logger                            logr.Logger
	DefaultSharedEndpointEnabled      bool
	DefaultSharedEndpointMode         string
	DefaultSharedEndpointIP           string
	DefaultSharedEndpointAutoFailback bool
}

// NewRegionalWorkloadReconciler creates a new reconciler with dependencies injected
func NewRegionalWorkloadReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	placementEngine placement.Engine,
	defaultSharedEndpointEnabled bool,
	defaultSharedEndpointMode string,
	defaultSharedEndpointIP string,
	defaultSharedEndpointAutoFailback bool,
	logger logr.Logger,
) *RegionalWorkloadReconciler {
	return &RegionalWorkloadReconciler{
		Client:                            client,
		Scheme:                            scheme,
		PlacementEngine:                   placementEngine,
		Logger:                            logger,
		DefaultSharedEndpointEnabled:      defaultSharedEndpointEnabled,
		DefaultSharedEndpointMode:         defaultSharedEndpointMode,
		DefaultSharedEndpointIP:           defaultSharedEndpointIP,
		DefaultSharedEndpointAutoFailback: defaultSharedEndpointAutoFailback,
	}
}

// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=regionalworkloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=regionalworkloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=regionalworkloads/finalizers,verbs=update
// +kubebuilder:rbac:groups=geo.oiviak3s.io,resources=nodehealthstatuses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch

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

	// Reconcile shared endpoint service
	if err := r.reconcileSharedEndpoint(ctx, workload); err != nil {
		logger.Error(err, "failed to reconcile shared endpoint")
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
	meta.SetStatusCondition(&workload.Status.Conditions, condition)
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
	meta.SetStatusCondition(&workload.Status.Conditions, condition)
}

// resolveSharedEndpointConfig resolves runtime defaults for shared endpoint behavior.
func (r *RegionalWorkloadReconciler) resolveSharedEndpointConfig(
	workload *geov1alpha1.RegionalWorkload,
) geov1alpha1.SharedEndpointConfig {
	cfg := workload.Spec.SharedEndpoint

	if cfg.Mode == "" {
		cfg.Mode = r.DefaultSharedEndpointMode
	}
	if cfg.IP == "" {
		cfg.IP = r.DefaultSharedEndpointIP
	}
	if !cfg.Enabled && workload.Spec.SharedEndpoint.Mode == "" && workload.Spec.SharedEndpoint.IP == "" {
		cfg.Enabled = r.DefaultSharedEndpointEnabled
		cfg.AutoFailback = r.DefaultSharedEndpointAutoFailback
	}

	return cfg
}

// reconcileSharedEndpoint ensures the workload has a stable shared endpoint service.
func (r *RegionalWorkloadReconciler) reconcileSharedEndpoint(
	ctx context.Context,
	workload *geov1alpha1.RegionalWorkload,
) error {
	cfg := r.resolveSharedEndpointConfig(workload)
	if !cfg.Enabled {
		return r.deleteSharedEndpoint(ctx, workload)
	}

	if cfg.Mode != "kube-vip" {
		return fmt.Errorf("unsupported shared endpoint mode: %s", cfg.Mode)
	}

	desired, err := r.buildSharedEndpointService(ctx, workload, cfg)
	if err != nil {
		return err
	}

	key := client.ObjectKeyFromObject(desired)
	existing := &corev1.Service{}
	if err := r.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return r.Create(ctx, desired)
	}

	return r.updateSharedEndpointService(ctx, existing, desired)
}


func (r *RegionalWorkloadReconciler) buildSharedEndpointService(
	ctx context.Context,
	workload *geov1alpha1.RegionalWorkload,
	cfg geov1alpha1.SharedEndpointConfig,
) (*corev1.Service, error) {
	selector, ports, err := r.getWorkloadSelectorAndPorts(ctx, workload)
	if err != nil {
		return nil, err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sharedEndpointServiceName(workload.Name),
			Namespace: workload.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":      "oiviak3s-operator",
				"geo.oiviak3s.io/regional-workload": workload.Name,
			},
			Annotations: map[string]string{
				"kube-vip.io/loadbalancerIPs": cfg.IP,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:           corev1.ServiceTypeLoadBalancer,
			Selector:       selector,
			Ports:          ports,
			LoadBalancerIP: cfg.IP,
		},
	}

	if err := controllerutil.SetControllerReference(workload, svc, r.Scheme); err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *RegionalWorkloadReconciler) updateSharedEndpointService(
	ctx context.Context,
	existing *corev1.Service,
	desired *corev1.Service,
) error {
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}

	needsUpdate := false
	if !reflect.DeepEqual(existing.Labels, desired.Labels) {
		existing.Labels = desired.Labels
		needsUpdate = true
	}
	if existing.Annotations["kube-vip.io/loadbalancerIPs"] != desired.Annotations["kube-vip.io/loadbalancerIPs"] {
		existing.Annotations["kube-vip.io/loadbalancerIPs"] = desired.Annotations["kube-vip.io/loadbalancerIPs"]
		needsUpdate = true
	}
	if existing.Spec.Type != desired.Spec.Type {
		existing.Spec.Type = desired.Spec.Type
		needsUpdate = true
	}
	if existing.Spec.LoadBalancerIP != desired.Spec.LoadBalancerIP {
		existing.Spec.LoadBalancerIP = desired.Spec.LoadBalancerIP
		needsUpdate = true
	}
	if !reflect.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Selector = desired.Spec.Selector
		needsUpdate = true
	}
	if !reflect.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) {
		existing.Spec.Ports = desired.Spec.Ports
		needsUpdate = true
	}

	if !needsUpdate {
		return nil
	}

	return r.Update(ctx, existing)
}

func (r *RegionalWorkloadReconciler) deleteSharedEndpoint(ctx context.Context, workload *geov1alpha1.RegionalWorkload) error {
	service := &corev1.Service{}
	key := types.NamespacedName{Namespace: workload.Namespace, Name: sharedEndpointServiceName(workload.Name)}
	if err := r.Get(ctx, key, service); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return client.IgnoreNotFound(r.Delete(ctx, service))
}

func sharedEndpointServiceName(workloadName string) string {
	return fmt.Sprintf("%s-shared-endpoint", workloadName)
}

// getWorkloadSelectorAndPorts extracts selector and ports from workload target.
func (r *RegionalWorkloadReconciler) getWorkloadSelectorAndPorts(
	ctx context.Context,
	workload *geov1alpha1.RegionalWorkload,
) (map[string]string, []corev1.ServicePort, error) {
	ref := workload.Spec.WorkloadRef

	switch ref.Kind {
	case "Deployment":
		deployment := &appsv1.Deployment{}
		key := types.NamespacedName{Namespace: workload.Namespace, Name: ref.Name}
		if err := r.Get(ctx, key, deployment); err != nil {
			return nil, nil, err
		}

		selector := deployment.Spec.Selector.MatchLabels
		if len(selector) == 0 {
			selector = deployment.Spec.Template.Labels
		}
		if len(selector) == 0 {
			selector = map[string]string{"app": ref.Name}
		}

		return selector, extractServicePorts(deployment.Spec.Template.Spec.Containers), nil
	case "StatefulSet":
		statefulSet := &appsv1.StatefulSet{}
		key := types.NamespacedName{Namespace: workload.Namespace, Name: ref.Name}
		if err := r.Get(ctx, key, statefulSet); err != nil {
			return nil, nil, err
		}

		selector := statefulSet.Spec.Selector.MatchLabels
		if len(selector) == 0 {
			selector = statefulSet.Spec.Template.Labels
		}
		if len(selector) == 0 {
			selector = map[string]string{"app": ref.Name}
		}

		return selector, extractServicePorts(statefulSet.Spec.Template.Spec.Containers), nil
	default:
		return nil, nil, fmt.Errorf("unsupported workload kind for shared endpoint: %s", ref.Kind)
	}
}

// extractServicePorts creates service ports from workload container ports.
func extractServicePorts(containers []corev1.Container) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0)
	seen := make(map[string]bool)

	for _, container := range containers {
		for _, p := range container.Ports {
			if p.ContainerPort <= 0 {
				continue
			}

			protocol := p.Protocol
			if protocol == "" {
				protocol = corev1.ProtocolTCP
			}

			key := fmt.Sprintf("%s/%d", protocol, p.ContainerPort)
			if seen[key] {
				continue
			}
			seen[key] = true

			name := p.Name
			if name == "" {
				name = fmt.Sprintf("%s-%d", strings.ToLower(string(protocol)), p.ContainerPort)
			}

			ports = append(ports, corev1.ServicePort{
				Name:       name,
				Port:       p.ContainerPort,
				TargetPort: intstr.FromInt32(p.ContainerPort),
				Protocol:   protocol,
			})
		}
	}

	if len(ports) == 0 {
		ports = append(ports, corev1.ServicePort{
			Name:       "http",
			Port:       80,
			TargetPort: intstr.FromInt(80),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	return ports
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

	if deployment.Spec.Template.Spec.NodeSelector == nil {
		deployment.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	if deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"] == nodeName {
		return nil
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

	if statefulSet.Spec.Template.Spec.NodeSelector == nil {
		statefulSet.Spec.Template.Spec.NodeSelector = make(map[string]string)
	}
	if statefulSet.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"] == nodeName {
		return nil
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

	return deployment.Status.ReadyReplicas, desiredReplicas(deployment.Spec.Replicas), nil
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

	return statefulSet.Status.ReadyReplicas, desiredReplicas(statefulSet.Spec.Replicas), nil
}

func desiredReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}

	return *replicas
}

// SetupWithManager sets up the controller with the Manager
func (r *RegionalWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&geov1alpha1.RegionalWorkload{}).
		Complete(r)
}
