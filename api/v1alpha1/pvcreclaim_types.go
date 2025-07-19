/*
Copyright 2025.

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

type PVCReclaimRecoverStatus string

const (
	NotRecovered       PVCReclaimRecoverStatus = "NotRecovered"
	RecoveryInProgress PVCReclaimRecoverStatus = "RecoveryInProgress"
	RecoveryFailed     PVCReclaimRecoverStatus = "RecoveryFailed"
	RecoverySuccess    PVCReclaimRecoverStatus = "RecoverySuccess"
)

// PVCReclaimSpec defines the desired state of PVCReclaim
type PVCReclaimSpec struct {
	// PersistentVolumeRef is the reference to the PersistentVolume resource bound by the deleted PersistentVolumeClaim
	PersistentVolumeRef *corev1.ObjectReference `json:"persistentVolumeRef"`
	// PersistentVolumeClaimSpec is the spec for PersistentVolumeClaim resource bound to the PersistentVolume
	PersistentVolumeClaimSpec corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec"`
	// Restore indicates whether a restore should be performed to recover the deleted PVC and have it bound to the PV again
	Restore bool `json:"restore"`
}

// PVCReclaimStatus defines the observed state of PVCReclaim
type PVCReclaimStatus struct {
	// RecoverStatus is the status of the current PVC reclaim resource
	RecoverStatus PVCReclaimRecoverStatus `json:"recoverStatus,omitempty"`
	// Reason provides messages indicating reason related to recovery failure
	Reason string `json:"reason,omitempty"`
	// Message is used to provide additional information regarding the state of the reclaim resource
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PVCReclaim is the Schema for the pvcreclaims API
type PVCReclaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PVCReclaimSpec   `json:"spec,omitempty"`
	Status PVCReclaimStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PVCReclaimList contains a list of PVCReclaim
type PVCReclaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PVCReclaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PVCReclaim{}, &PVCReclaimList{})
}
