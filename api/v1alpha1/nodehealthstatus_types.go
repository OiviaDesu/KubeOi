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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeHealthStatusSpec defines the desired state of NodeHealthStatus
type NodeHealthStatusSpec struct {
	// NodeName is the name of the node being monitored
	NodeName string `json:"nodeName"`
	
	// CheckInterval specifies how often to check node health
	// +kubebuilder:default="30s"
	CheckInterval metav1.Duration `json:"checkInterval,omitempty"`
	
	// FailureThreshold is the number of consecutive failures before marking unhealthy
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

// HealthCheckStatus represents the status of a specific health check
type HealthCheckStatus struct {
	// CheckerName is the name of the health checker
	CheckerName string `json:"checkerName"`
	
	// Status is the health status from this checker
	Status string `json:"status"`
	
	// Message provides additional context about the status
	Message string `json:"message,omitempty"`
	
	// LastCheckTime is when this check was last performed
	LastCheckTime metav1.Time `json:"lastCheckTime"`
	
	// Details contains additional check-specific information
	Details map[string]string `json:"details,omitempty"`
}

// NodeHealthStatusStatus defines the observed state of NodeHealthStatus
type NodeHealthStatusStatus struct {
	// OverallStatus is the aggregated health status
	// +kubebuilder:validation:Enum=Healthy;Degraded;Unhealthy;Unknown
	OverallStatus string `json:"overallStatus"`
	
	// Checks contains the status of individual health checks
	Checks []HealthCheckStatus `json:"checks,omitempty"`
	
	// LastTransitionTime is when the overall status last changed
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	
	// ConsecutiveFailures is the number of consecutive failures
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
	
	// Conditions represent the latest available observations of the node's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	
	// Region is the geographic region of this node
	Region string `json:"region,omitempty"`
	
	// Tier is the priority tier of this node
	Tier string `json:"tier,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.overallStatus`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.status.region`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodeHealthStatus tracks the health status of a cluster node
type NodeHealthStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeHealthStatusSpec   `json:"spec,omitempty"`
	Status NodeHealthStatusStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeHealthStatusList contains a list of NodeHealthStatus
type NodeHealthStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeHealthStatus `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeHealthStatus{}, &NodeHealthStatusList{})
}
