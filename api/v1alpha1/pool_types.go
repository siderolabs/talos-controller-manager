// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PoolSpec defines the desired state of Pool
type PoolSpec struct {
	Name          string           `json:"name,omitempty"`
	Channel       string           `json:"channel,omitempty"`
	Version       string           `json:"version,omitempty"`
	Registry      string           `json:"registry,omitempty"`
	Repository    string           `json:"repository,omitempty"`
	Concurrency   int              `json:"concurrency,omitempty"`
	FailurePolicy string           `json:"onFailure,omitempty"`
	CheckInterval *metav1.Duration `json:"checkInterval,omitempty"`
}

// PoolStatus defines the observed state of Pool
type PoolStatus struct {
	Size       int         `json:"size,omitempty"`
	NextRun    metav1.Time `json:"nextRun,omitempty"`
	InProgress string      `json:"inProgess,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=pools,scope=Cluster

// Pool is the Schema for the pools API
// See https://book.kubebuilder.io/reference/markers/crd.html
// +kubebuilder:printcolumn:name="Channel",type="string",JSONPath=".spec.channel",description="the pool's upgrade channel"
// +kubebuilder:printcolumn:name="Size",type="integer",JSONPath=".status.size",description="the number of nodes in the pool"
// +kubebuilder:printcolumn:name="Concurrency",type="string",JSONPath=".spec.concurrency",description="the pool's maximum number of concurrent upgrades"
// +kubebuilder:printcolumn:name="Next Run",type="string",format="date-time",JSONPath=".status.nextRun",description="when the next upgrade attempt will be made (UTC time standard)"
// +kubebuilder:printcolumn:name="In Progress",type="string",JSONPath=".status.inProgess",description="the nodes in the pool that are currently in progress of upgrading"
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PoolSpec   `json:"spec,omitempty"`
	Status PoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PoolList contains a list of Pool
type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pool{}, &PoolList{})
}
