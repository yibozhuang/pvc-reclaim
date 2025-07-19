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

package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/yibozhuang/pvc-reclaim/api/v1alpha1"
)

// PVCController reconciles a PersistentVolumeClaim object
type PVCController struct {
	client client.Client
}

var _ reconcile.Reconciler = &PVCController{}

func NewPVCController(client client.Client) *PVCController {
	return &PVCController{
		client: client,
	}
}

//+kubebuilder:rbac:groups=``,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=``,resources=persistentvolumeclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=``,resources=persistentvolumeclaims/finalizers,verbs=update
//+kubebuilder:rbac:groups=``,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=``,resources=persistentvolumes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=``,resources=persistentvolumes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PVCController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pvc corev1.PersistentVolumeClaim
	if err := r.client.Get(ctx, req.NamespacedName, &pvc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if pvc.Spec.VolumeName == "" || pvc.Status.Phase != corev1.ClaimBound {
		logger.Info("PVC is not Bound, nothing to be done", "pvc", fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
		return ctrl.Result{}, nil
	}

	pvcReclaim, err := r.getOrCreateClaimRef(ctx, &pvc)
	if err != nil {
		return ctrl.Result{}, err
	}

	pvcReclaim.Annotations = pvc.Annotations
	for labelKey, labelVal := range pvc.Labels {
		pvcReclaim.Labels[labelKey] = labelVal
	}
	pvcReclaim.Spec.Restore = false
	if err := r.client.Update(ctx, &pvcReclaim); err != nil {
		return ctrl.Result{}, err
	}

	// patch the pvcReclaim status
	patch := client.MergeFrom(pvcReclaim.DeepCopy())
	pvcReclaim.Status.Reason = ""
	pvcReclaim.Status.RecoverStatus = v1alpha1.NotRecovered
	pvcReclaim.Status.Message = fmt.Sprintf("PVC %s Bound, PVCReclaim %s created for recovery", fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name), fmt.Sprintf("%s/%s", pvcReclaim.Namespace, pvcReclaim.Name))
	if err := r.client.Status().Patch(ctx, &pvcReclaim, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *PVCController) getOrCreateClaimRef(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (v1alpha1.PVCReclaim, error) {
	var pvcReclaim v1alpha1.PVCReclaim

	// attempt to get the current Bound PV resource
	var pv corev1.PersistentVolume
	if err := r.client.Get(ctx, types.NamespacedName{Name: pvc.Spec.VolumeName}, &pv); err != nil {
		return pvcReclaim, err
	}

	// attempt to get the current PVC reclaim resource
	err := r.client.Get(ctx, types.NamespacedName{Namespace: pvc.Namespace, Name: pvc.Name}, &pvcReclaim)
	if err != nil && !errors.IsNotFound(err) {
		return pvcReclaim, err
	}

	if errors.IsNotFound(err) {
		pvcReclaim = v1alpha1.PVCReclaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
			},
			Spec: v1alpha1.PVCReclaimSpec{
				PersistentVolumeRef: &corev1.ObjectReference{
					Kind:       pv.Kind,
					APIVersion: pv.APIVersion,
					Name:       pv.Name,
				},
				PersistentVolumeClaimSpec: pvc.Spec,
				Restore:                   false,
			},
		}
		if pvc.Annotations != nil {
			pvcReclaim.Annotations = pvc.Annotations
		}
		if pvc.Labels == nil {
			pvcReclaim.Labels = make(map[string]string)
		} else {
			pvcReclaim.Labels = pvc.Labels
		}
		pvcReclaim.Labels[reclaimPVLabel] = pv.Name
		if err := r.client.Create(ctx, &pvcReclaim); err != nil {
			return pvcReclaim, err
		}
	}
	return pvcReclaim, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}
