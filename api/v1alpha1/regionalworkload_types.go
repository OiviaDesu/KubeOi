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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlacementConstraints specifies where a workload should be placed
type PlacementConstraints struct {
	// RegionPreference lists preferred regions in priority order
	RegionPreference []string `json:"regionPreference,omitempty"`

	// AvoidNodes lists node names to avoid for placement
	AvoidNodes []string `json:"avoidNodes,omitempty"`

	// RequireLabels specifies labels that target nodes must have
	RequireLabels map[string]string `json:"requireLabels,omitempty"`

	// TierPreference lists preferred tiers in priority order
	// +kubebuilder:validation:Enum=primary;secondary;tertiary
	TierPreference []string `json:"tierPreference,omitempty"`

	// AntiAffinity prevents co-location with specified workloads
	AntiAffinity []string `json:"antiAffinity,omitempty"`
}

// FailoverConfig specifies failover behavior for this workload
type FailoverConfig struct {
	// Enabled determines if automated failover is enabled
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// MaxFailoverTime is the maximum time allowed for failover
	// +kubebuilder:default="5m"
	MaxFailoverTime metav1.Duration `json:"maxFailoverTime,omitempty"`

	// MinHealthyReplicas is the minimum number of healthy replicas required
	// +kubebuilder:validation:Minimum=0
	MinHealthyReplicas int32 `json:"minHealthyReplicas,omitempty"`

	// HealthCheckGracePeriod is the grace period before marking workload unhealthy
	// +kubebuilder:default="1m"
	HealthCheckGracePeriod metav1.Duration `json:"healthCheckGracePeriod,omitempty"`
}

// SharedEndpointTarget specifies one public entrypoint for a shared endpoint workload.
type SharedEndpointTarget struct {
	// Name is a stable identifier used to name the managed Service.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// IP is the public IP address for this shared endpoint target.
	// +kubebuilder:validation:Pattern=`^((25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])$`
	IP string `json:"ip"`
}

// SharedEndpointConfig specifies stable endpoint IPs shared across failover events.
// +kubebuilder:validation:XValidation:rule="!self.enabled || has(self.ip) || size(self.endpoints) > 0",message="ip or endpoints is required when shared endpoint is enabled"
type SharedEndpointConfig struct {
	// Enabled determines if a shared endpoint should be managed for this workload
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Mode selects the shared endpoint management mode
	// +kubebuilder:validation:Enum=kube-vip
	// +kubebuilder:default=kube-vip
	Mode string `json:"mode,omitempty"`

	// IP is the shared endpoint IP address
	// +kubebuilder:validation:Pattern=`^((25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])$`
	// +kubebuilder:default="192.168.86.8"
	IP string `json:"ip,omitempty"`

	// Endpoints mirrors the same backend behavior across multiple public IPs.
	// When specified, every endpoint publishes the same service port set.
	Endpoints []SharedEndpointTarget `json:"endpoints,omitempty"`

	// AutoFailback determines if placement should automatically return to preferred nodes after recovery
	// +kubebuilder:default=true
	AutoFailback bool `json:"autoFailback,omitempty"`
}

// RegionalWorkloadSpec defines the desired state of RegionalWorkload
type RegionalWorkloadSpec struct {
	// WorkloadRef references the underlying workload (Deployment, StatefulSet, etc)
	WorkloadRef corev1.ObjectReference `json:"workloadRef"`

	// PlacementConstraints specifies placement requirements
	PlacementConstraints PlacementConstraints `json:"placementConstraints,omitempty"`

	// FailoverConfig specifies failover behavior
	FailoverConfig FailoverConfig `json:"failoverConfig,omitempty"`

	// SharedEndpoint configures a stable endpoint across failover events
	SharedEndpoint SharedEndpointConfig `json:"sharedEndpoint,omitempty"`

	// NotificationEnabled determines if notifications are sent for this workload
	// +kubebuilder:default=true
	NotificationEnabled bool `json:"notificationEnabled,omitempty"`
}

// PlacementDecision represents where a workload is currently placed
type PlacementDecision struct {
	// NodeName is the node where workload is placed
	NodeName string `json:"nodeName"`

	// Region is the region of the placement
	Region string `json:"region"`

	// Reason explains why this placement was chosen
	Reason string `json:"reason"`

	// Score is the placement score
	Score float64 `json:"score"`

	// Timestamp is when this placement decision was made
	Timestamp metav1.Time `json:"timestamp"`
}

// WorkloadHealth represents the health status of the workload
type WorkloadHealth struct {
	// Status is the overall health status
	// +kubebuilder:validation:Enum=Healthy;Degraded;Unhealthy;Unknown
	Status string `json:"status"`

	// ReadyReplicas is the number of ready replicas
	ReadyReplicas int32 `json:"readyReplicas"`

	// DesiredReplicas is the desired number of replicas
	DesiredReplicas int32 `json:"desiredReplicas"`

	// LastCheckTime is when health was last checked
	LastCheckTime metav1.Time `json:"lastCheckTime"`
}

// RegionalWorkloadStatus defines the observed state of RegionalWorkload
type RegionalWorkloadStatus struct {
	// Placement represents the current placement decision
	Placement *PlacementDecision `json:"placement,omitempty"`

	// Health represents the current health status
	Health WorkloadHealth `json:"health"`

	// Conditions represent the latest available observations of the workload's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastFailoverTime is when the last failover occurred
	LastFailoverTime *metav1.Time `json:"lastFailoverTime,omitempty"`

	// FailoverCount is the total number of failovers
	FailoverCount int32 `json:"failoverCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Workload",type=string,JSONPath=`.spec.workloadRef.name`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.placement.nodeName`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.status.placement.region`
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.health.status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RegionalWorkload manages workload placement across geo-distributed nodes
type RegionalWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegionalWorkloadSpec   `json:"spec,omitempty"`
	Status RegionalWorkloadStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RegionalWorkloadList contains a list of RegionalWorkload
type RegionalWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RegionalWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RegionalWorkload{}, &RegionalWorkloadList{})
}
