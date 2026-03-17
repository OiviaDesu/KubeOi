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
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	geov1alpha1 "github.com/oiviadesu/oiviak3s-operator/api/v1alpha1"
	"github.com/oiviadesu/oiviak3s-operator/pkg/placement"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

const disableNodePinningAnnotation = "geo.oiviak3s.io/disable-node-pinning"

type validationError struct {
	field   string
	message string
}

func (e *validationError) Error() string {
	if e.field == "" {
		return e.message
	}
	return fmt.Sprintf("invalid %s: %s", e.field, e.message)
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
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

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

	if err := r.validateWorkload(workload); err != nil {
		logger.Error(err, "regional workload validation failed")
		r.updateCondition(workload, "ConfigurationReady", metav1.ConditionFalse, "ValidationFailed", err.Error())
		r.updateCondition(workload, "EndpointReady", metav1.ConditionFalse, "ValidationFailed", err.Error())
		if statusErr := r.Status().Update(ctx, workload); statusErr != nil {
			logger.Error(statusErr, "unable to update invalid workload status")
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}
	r.updateCondition(workload, "ConfigurationReady", metav1.ConditionTrue, "ValidationSucceeded", "RegionalWorkload configuration is valid")

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
		r.updateStatusError(workload, "PlacementReady", "PlacementFailed", err)
		if err := r.Status().Update(ctx, workload); err != nil {
			logger.Error(err, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
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
		r.updateStatusError(workload, "EndpointReady", "EndpointReconcileFailed", err)
		if statusErr := r.Status().Update(ctx, workload); statusErr != nil {
			logger.Error(statusErr, "unable to update endpoint failure status")
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
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
	r.updateCondition(
		workload,
		"PlacementReady",
		metav1.ConditionTrue,
		"PlacementSuccessful",
		fmt.Sprintf("Workload placed on node %s in region %s", decision.TargetNode, decision.Region),
	)
}

// updateStatusError updates status on error
func (r *RegionalWorkloadReconciler) updateStatusError(
	workload *geov1alpha1.RegionalWorkload,
	conditionType string,
	reason string,
	err error,
) {
	r.updateCondition(workload, conditionType, metav1.ConditionFalse, reason, err.Error())
}

func (r *RegionalWorkloadReconciler) updateCondition(
	workload *geov1alpha1.RegionalWorkload,
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: workload.Generation,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
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
	if len(cfg.Endpoints) == 0 && cfg.IP != "" {
		cfg.Endpoints = []geov1alpha1.SharedEndpointTarget{{
			Name: "primary",
			IP:   cfg.IP,
		}}
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
		if err := r.deleteSharedEndpointServices(ctx, workload); err != nil {
			return err
		}
		r.updateCondition(workload, "EndpointReady", metav1.ConditionFalse, "SharedEndpointDisabled", "Shared endpoints are disabled")
		return nil
	}

	if cfg.Mode != "kube-vip" {
		return &validationError{field: "sharedEndpoint.mode", message: fmt.Sprintf("unsupported shared endpoint mode %q", cfg.Mode)}
	}

	desiredServices, err := r.buildSharedEndpointServices(ctx, workload, cfg)
	if err != nil {
		return err
	}

	if err := r.syncSharedEndpointServices(ctx, workload, desiredServices); err != nil {
		return err
	}

	r.updateCondition(workload, "EndpointReady", metav1.ConditionTrue, "SharedEndpointsReady", fmt.Sprintf("Reconciled %d shared endpoint service(s)", len(desiredServices)))
	return nil
}

func (r *RegionalWorkloadReconciler) syncSharedEndpointServices(
	ctx context.Context,
	workload *geov1alpha1.RegionalWorkload,
	desiredServices []*corev1.Service,
) error {
	desiredByName := make(map[string]*corev1.Service, len(desiredServices))
	for _, desired := range desiredServices {
		desiredByName[desired.Name] = desired
	}

	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList,
		client.InNamespace(workload.Namespace),
		client.MatchingLabels(map[string]string{"geo.oiviak3s.io/regional-workload": workload.Name}),
	); err != nil {
		return err
	}

	for i := range serviceList.Items {
		existing := &serviceList.Items[i]
		desired, exists := desiredByName[existing.Name]
		if !exists {
			if err := client.IgnoreNotFound(r.Delete(ctx, existing)); err != nil {
				return err
			}
			continue
		}

		if err := r.updateSharedEndpointService(ctx, existing, desired); err != nil {
			return err
		}
		delete(desiredByName, existing.Name)
	}

	for _, desired := range desiredByName {
		if err := r.Create(ctx, desired); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

func (r *RegionalWorkloadReconciler) buildSharedEndpointServices(
	ctx context.Context,
	workload *geov1alpha1.RegionalWorkload,
	cfg geov1alpha1.SharedEndpointConfig,
) ([]*corev1.Service, error) {
	selector, ports, err := r.getWorkloadSelectorAndPorts(ctx, workload)
	if err != nil {
		return nil, err
	}

	targets, err := normalizeSharedEndpointTargets(cfg)
	if err != nil {
		return nil, err
	}

	services := make([]*corev1.Service, 0, len(targets))
	for _, target := range targets {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sharedEndpointServiceName(workload.Name, target, len(targets) == 1),
				Namespace: workload.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by":      "oiviak3s-operator",
					"geo.oiviak3s.io/regional-workload": workload.Name,
					"geo.oiviak3s.io/shared-endpoint":   target.Name,
				},
				Annotations: map[string]string{
					"kube-vip.io/loadbalancerIPs": target.IP,
				},
			},
			Spec: corev1.ServiceSpec{
				Type:           corev1.ServiceTypeLoadBalancer,
				Selector:       selector,
				Ports:          ports,
				LoadBalancerIP: target.IP,
			},
		}

		if err := controllerutil.SetControllerReference(workload, svc, r.Scheme); err != nil {
			return nil, err
		}

		services = append(services, svc)
	}

	return services, nil
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

func (r *RegionalWorkloadReconciler) deleteSharedEndpointServices(ctx context.Context, workload *geov1alpha1.RegionalWorkload) error {
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList,
		client.InNamespace(workload.Namespace),
		client.MatchingLabels(map[string]string{"geo.oiviak3s.io/regional-workload": workload.Name}),
	); err != nil {
		return err
	}

	for i := range serviceList.Items {
		if err := client.IgnoreNotFound(r.Delete(ctx, &serviceList.Items[i])); err != nil {
			return err
		}
	}

	return nil
}

func sharedEndpointServiceName(workloadName string, target geov1alpha1.SharedEndpointTarget, singleTarget bool) string {
	if singleTarget {
		return fmt.Sprintf("%s-shared-endpoint", workloadName)
	}
	return fmt.Sprintf("%s-shared-endpoint-%s", workloadName, sanitizeEndpointName(target.Name))
}

func sanitizeEndpointName(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return "endpoint"
	}

	var b strings.Builder
	b.Grow(len(trimmed))
	lastDash := false
	for _, r := range trimmed {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}

	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "endpoint"
	}
	return result
}

func normalizeSharedEndpointTargets(cfg geov1alpha1.SharedEndpointConfig) ([]geov1alpha1.SharedEndpointTarget, error) {
	if len(cfg.Endpoints) == 0 {
		if cfg.IP == "" {
			return nil, &validationError{field: "sharedEndpoint", message: "enabled shared endpoints require ip or endpoints"}
		}
		return []geov1alpha1.SharedEndpointTarget{{Name: "primary", IP: cfg.IP}}, nil
	}

	targets := make([]geov1alpha1.SharedEndpointTarget, 0, len(cfg.Endpoints))
	seenNames := make(map[string]struct{}, len(cfg.Endpoints))
	seenIPs := make(map[string]struct{}, len(cfg.Endpoints))
	for i, endpoint := range cfg.Endpoints {
		name := sanitizeEndpointName(endpoint.Name)
		if endpoint.IP == "" {
			return nil, &validationError{field: fmt.Sprintf("sharedEndpoint.endpoints[%d].ip", i), message: "must not be empty"}
		}
		if _, exists := seenNames[name]; exists {
			return nil, &validationError{field: fmt.Sprintf("sharedEndpoint.endpoints[%d].name", i), message: fmt.Sprintf("duplicate endpoint name %q", name)}
		}
		if _, exists := seenIPs[endpoint.IP]; exists {
			return nil, &validationError{field: fmt.Sprintf("sharedEndpoint.endpoints[%d].ip", i), message: fmt.Sprintf("duplicate endpoint IP %q", endpoint.IP)}
		}
		seenNames[name] = struct{}{}
		seenIPs[endpoint.IP] = struct{}{}
		targets = append(targets, geov1alpha1.SharedEndpointTarget{Name: name, IP: endpoint.IP})
	}

	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	return targets, nil
}

func (r *RegionalWorkloadReconciler) validateWorkload(workload *geov1alpha1.RegionalWorkload) error {
	if workload.Spec.WorkloadRef.Kind == "" {
		return &validationError{field: "workloadRef.kind", message: "must not be empty"}
	}
	if workload.Spec.WorkloadRef.Name == "" {
		return &validationError{field: "workloadRef.name", message: "must not be empty"}
	}

	cfg := r.resolveSharedEndpointConfig(workload)
	if !cfg.Enabled {
		return nil
	}

	_, err := normalizeSharedEndpointTargets(cfg)
	return err
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

	if shouldDisableNodePinning(deployment.Annotations) {
		if deployment.Spec.Template.Spec.NodeSelector == nil {
			return nil
		}
		if _, exists := deployment.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"]; !exists {
			return nil
		}

		delete(deployment.Spec.Template.Spec.NodeSelector, "kubernetes.io/hostname")
		if len(deployment.Spec.Template.Spec.NodeSelector) == 0 {
			deployment.Spec.Template.Spec.NodeSelector = nil
		}
		return r.Update(ctx, deployment)
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

	if shouldDisableNodePinning(statefulSet.Annotations) {
		if statefulSet.Spec.Template.Spec.NodeSelector == nil {
			return nil
		}
		if _, exists := statefulSet.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"]; !exists {
			return nil
		}

		delete(statefulSet.Spec.Template.Spec.NodeSelector, "kubernetes.io/hostname")
		if len(statefulSet.Spec.Template.Spec.NodeSelector) == 0 {
			statefulSet.Spec.Template.Spec.NodeSelector = nil
		}
		return r.Update(ctx, statefulSet)
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

func shouldDisableNodePinning(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}
	v, ok := annotations[disableNodePinningAnnotation]
	if !ok {
		return false
	}
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "true" || v == "1" || v == "yes"
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
