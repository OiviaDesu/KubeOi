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

// Failover strategy constants
const (
	FailoverImmediate = "immediate"
	FailoverGraceful  = "graceful"
	FailoverManual    = "manual"
)

// FailoverTrigger defines what triggers a failover
type FailoverTrigger struct {
	// NodeUnhealthyDuration is how long a node must be unhealthy before failover
	// +kubebuilder:default="2m"
	NodeUnhealthyDuration metav1.Duration `json:"nodeUnhealthyDuration,omitempty"`
	
	// WorkloadUnhealthyDuration is how long a workload must be unhealthy before failover
	// +kubebuilder:default="1m"
	WorkloadUnhealthyDuration metav1.Duration `json:"workloadUnhealthyDuration,omitempty"`
	
	// RegionalOutage triggers failover when all nodes in a region are unhealthy
	// +kubebuilder:default=true
	RegionalOutage bool `json:"regionalOutage,omitempty"`
}

// FailoverStrategy defines how failover is executed
type FailoverStrategy struct {
	// Type specifies the failover strategy type
	// +kubebuilder:validation:Enum=immediate;graceful;manual
	// +kubebuilder:default=graceful
	Type string `json:"type,omitempty"`
	
	// DrainTimeout is the timeout for draining nodes during graceful failover
	// +kubebuilder:default="5m"
	DrainTimeout metav1.Duration `json:"drainTimeout,omitempty"`
	
	// GracePeriod is the grace period for pod termination
	// +kubebuilder:default="30s"
	GracePeriod metav1.Duration `json:"gracePeriod,omitempty"`
	
	// TargetRegionPreference specifies preferred failover target regions
	TargetRegionPreference []string `json:"targetRegionPreference,omitempty"`
}

// NotificationRule defines when to send notifications
type NotificationRule struct {
	// Enabled determines if notifications are enabled
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
	
	// OnFailoverStart sends notification when failover starts
	// +kubebuilder:default=true
	OnFailoverStart bool `json:"onFailoverStart,omitempty"`
	
	// OnFailoverComplete sends notification when failover completes
	// +kubebuilder:default=true
	OnFailoverComplete bool `json:"onFailoverComplete,omitempty"`
	
	// OnFailoverFailed sends notification when failover fails
	// +kubebuilder:default=true
	OnFailoverFailed bool `json:"onFailoverFailed,omitempty"`
	
	// OnNodeHealthChange sends notification when node health changes
	// +kubebuilder:default=false
	OnNodeHealthChange bool `json:"onNodeHealthChange,omitempty"`
	
	// MinSeverity is the minimum severity to trigger notification
	// +kubebuilder:validation:Enum=info;warning;critical
	// +kubebuilder:default=warning
	MinSeverity string `json:"minSeverity,omitempty"`
}

// FailoverPolicySpec defines the desired state of FailoverPolicy
type FailoverPolicySpec struct {
	// Enabled determines if this policy is active
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
	
	// Trigger defines what triggers a failover
	Trigger FailoverTrigger `json:"trigger,omitempty"`
	
	// Strategy defines how failover is executed
	Strategy FailoverStrategy `json:"strategy,omitempty"`
	
	// NotificationRule defines notification behavior
	NotificationRule NotificationRule `json:"notificationRule,omitempty"`
	
	// TargetWorkloads lists workloads this policy applies to
	// Empty list means apply to all workloads
	TargetWorkloads []string `json:"targetWorkloads,omitempty"`
}

// FailoverEvent represents a single failover event
type FailoverEvent struct {
	// Timestamp is when this event occurred
	Timestamp metav1.Time `json:"timestamp"`
	
	// WorkloadName is the name of the workload that failed over
	WorkloadName string `json:"workloadName"`
	
	// SourceNode is the node where workload was running
	SourceNode string `json:"sourceNode"`
	
	// TargetNode is the node where workload was moved to
	TargetNode string `json:"targetNode"`
	
	// Reason explains why failover occurred
	Reason string `json:"reason"`
	
	// Duration is how long the failover took
	Duration metav1.Duration `json:"duration"`
	
	// Success indicates if failover succeeded
	Success bool `json:"success"`
}

// FailoverPolicyStatus defines the observed state of FailoverPolicy
type FailoverPolicyStatus struct {
	// Active indicates if this policy is currently active
	Active bool `json:"active"`
	
	// LastFailoverTime is when the last failover occurred
	LastFailoverTime *metav1.Time `json:"lastFailoverTime,omitempty"`
	
	// TotalFailovers is the total number of failovers performed
	TotalFailovers int32 `json:"totalFailovers,omitempty"`
	
	// RecentEvents lists recent failover events
	RecentEvents []FailoverEvent `json:"recentEvents,omitempty"`
	
	// Conditions represent the latest available observations of the policy's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Strategy",type=string,JSONPath=`.spec.strategy.type`
// +kubebuilder:printcolumn:name="Active",type=boolean,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="Failovers",type=integer,JSONPath=`.status.totalFailovers`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// FailoverPolicy defines failover behavior and triggers
type FailoverPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FailoverPolicySpec   `json:"spec,omitempty"`
	Status FailoverPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FailoverPolicyList contains a list of FailoverPolicy
type FailoverPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FailoverPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FailoverPolicy{}, &FailoverPolicyList{})
}
