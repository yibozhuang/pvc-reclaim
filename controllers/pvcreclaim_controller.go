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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/yibozhuang/pvc-reclaim/api/v1alpha1"
)

const (
	reclaimPVLabel     = "pvc-reclaim.yibozhuang.me/pv-name"
	pvAnnotationPrefix = "pv.kubernetes.io"
)

// PVCReclaimController reconciles a PVCReclaim object
type PVCReclaimController struct {
	client client.Client
}

var _ reconcile.Reconciler = &PVCReclaimController{}

func NewPVCReclaimController(client client.Client) *PVCReclaimController {
	return &PVCReclaimController{
		client: client,
	}
}

//+kubebuilder:rbac:groups=yibozhuang.me,resources=pvcreclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=yibozhuang.me,resources=pvcreclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=yibozhuang.me,resources=pvcreclaims/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PVCReclaimController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pvcReclaim v1alpha1.PVCReclaim
	var pv corev1.PersistentVolume
	var pvc corev1.PersistentVolumeClaim

	err := r.client.Get(ctx, req.NamespacedName, &pvcReclaim)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	pvcFetchErr := r.client.Get(ctx, req.NamespacedName, &pvc)
	if pvcFetchErr != nil && !errors.IsNotFound(pvcFetchErr) {
		return ctrl.Result{}, err
	}

	if errors.IsNotFound(err) && errors.IsNotFound(pvcFetchErr) {
		return ctrl.Result{}, nil
	}
	if errors.IsNotFound(err) && pvcFetchErr == nil {
		if pvc.Spec.VolumeName == "" || pvc.Status.Phase != corev1.ClaimBound {
			logger.Info("PVC is not Bound, nothing to be done", "pvc", fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
			return ctrl.Result{}, nil
		}

		if err := r.client.Get(ctx, types.NamespacedName{Name: pvc.Spec.VolumeName}, &pv); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

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
		if err := r.client.Create(ctx, &pvcReclaim); err != nil && !errors.IsAlreadyExists(err) {
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

	deletePolicy := metav1.DeletePropagationForeground
	err = r.client.Get(ctx, types.NamespacedName{Name: pvcReclaim.Spec.PersistentVolumeRef.Name}, &pv)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if errors.IsNotFound(err) {
		logger.Info("PV is not found, deleting PVCReclaim", "pv", pvcReclaim.Spec.PersistentVolumeRef.Name, "PVCReclaim", fmt.Sprintf("%s/%s", pvcReclaim.Namespace, pvcReclaim.Name))
		if innerErr := r.client.Delete(ctx, &pvcReclaim, &client.DeleteOptions{
			GracePeriodSeconds: &[]int64{0}[0],
			PropagationPolicy:  &deletePolicy,
		}); innerErr != nil {
			return ctrl.Result{}, innerErr
		}
		return ctrl.Result{}, nil
	}

	if !pvcReclaim.Spec.Restore {
		return ctrl.Result{}, nil
	}

	// Check to ensure PV is in Released phase
	if pv.Status.Phase != corev1.VolumeReleased {
		logger.Info("PV is not in Released phase", "pv", pvcReclaim.Spec.PersistentVolumeRef.Name, "PVCReclaim", fmt.Sprintf("%s/%s", pvcReclaim.Namespace, pvcReclaim.Name))
		patch := client.MergeFrom(pvcReclaim.DeepCopy())
		pvcReclaim.Spec.Restore = false
		if err := r.client.Patch(ctx, &pvcReclaim, patch); err != nil {
			return ctrl.Result{}, err
		}
		patch = client.MergeFrom(pvcReclaim.DeepCopy())
		pvcReclaim.Status.RecoverStatus = v1alpha1.RecoveryFailed
		pvcReclaim.Status.Reason = fmt.Sprintf("PV %s is not in Released phase", pv.Name)
		if err := r.client.Status().Patch(ctx, &pvcReclaim, patch); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	patch := client.MergeFrom(pvcReclaim.DeepCopy())
	pvcReclaim.Status.RecoverStatus = v1alpha1.RecoveryInProgress
	pvcReclaim.Status.Message = fmt.Sprintf("Recovering PVC %s and having it bound to PV %s", fmt.Sprintf("%s/%s", pvcReclaim.Namespace, pvcReclaim.Name), pv.Name)
	pvcReclaim.Status.Reason = ""
	if err := r.client.Status().Patch(ctx, &pvcReclaim, patch); err != nil {
		return ctrl.Result{}, err
	}

	// recreate the PVC
	pvc = corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcReclaim.Name,
			Namespace:   pvcReclaim.Namespace,
			Labels:      pvcReclaim.Labels,
			Annotations: pvcReclaim.Annotations,
		},
		Spec: pvcReclaim.Spec.PersistentVolumeClaimSpec,
	}
	delete(pvc.Labels, reclaimPVLabel)
	if err := r.client.Create(ctx, &pvc); err != nil && !errors.IsAlreadyExists(err) {
		patch := client.MergeFrom(pvcReclaim.DeepCopy())
		pvcReclaim.Status.RecoverStatus = v1alpha1.RecoveryFailed
		pvcReclaim.Status.Reason = fmt.Sprintf("Failed to re-create PVC %s, error: %v", fmt.Sprintf("%s/%s", pvcReclaim.Namespace, pvcReclaim.Name), err)
		if innerErr := r.client.Status().Patch(ctx, &pvcReclaim, patch); innerErr != nil {
			return ctrl.Result{}, innerErr
		}

		return ctrl.Result{}, err
	}

	// patch up the PVC phase
	patch = client.MergeFrom(pvc.DeepCopy())
	pvc.Status.Phase = corev1.ClaimBound
	if err := r.client.Status().Patch(ctx, &pvc, patch); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.client.Get(ctx, types.NamespacedName{Namespace: pvcReclaim.Namespace, Name: pvcReclaim.Name}, &pvc); err != nil {
		return ctrl.Result{}, err
	}

	patch = client.MergeFrom(pv.DeepCopy())
	for annKey := range pv.Annotations {
		if !strings.Contains(annKey, pvAnnotationPrefix) {
			delete(pv.Annotations, annKey)
		}
	}
	pv.Spec.ClaimRef.UID = pvc.UID
	if err := r.client.Patch(ctx, &pv, patch); err != nil {
		return ctrl.Result{}, err
	}

	patch = client.MergeFrom(pvcReclaim.DeepCopy())
	pvcReclaim.Status.RecoverStatus = v1alpha1.RecoverySuccess
	pvcReclaim.Status.Reason = ""
	pvcReclaim.Status.Message = fmt.Sprintf("Sucessfully restored PVC %s and bound it to PV %s", fmt.Sprintf(pvcReclaim.Namespace, pvcReclaim.Name), pv.Name)
	if err := r.client.Status().Patch(ctx, &pvcReclaim, patch); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Deleting PVCReclaim after successfully recovering PVC", "PVCReclaim", fmt.Sprintf("%s/%s", pvcReclaim.Namespace, pvcReclaim.Name))
	if err := r.client.Delete(ctx, &pvcReclaim, &client.DeleteOptions{
		GracePeriodSeconds: &[]int64{0}[0],
		PropagationPolicy:  &deletePolicy,
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCReclaimController) SetupWithManager(mgr ctrl.Manager) error {
	pvPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	pvEnqueuePVCReclaimReconcileRequestMapFunc := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		pv, ok := object.(*corev1.PersistentVolume)
		if !ok {
			return nil
		}

		var pvcReclaims v1alpha1.PVCReclaimList
		if err := r.client.List(context.Background(), &pvcReclaims, client.MatchingLabels{
			reclaimPVLabel: pv.Name,
		}); err != nil {
			return nil
		}

		var requests []reconcile.Request
		for _, reclaim := range pvcReclaims.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: reclaim.Name, Namespace: reclaim.Namespace},
			})
		}
		return requests
	})

	err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.PVCReclaim{}, reclaimPVLabel, func(object client.Object) []string {
		pvcReclaim, ok := object.(*v1alpha1.PVCReclaim)
		if !ok {
			return nil
		}
		pvLabel, found := pvcReclaim.GetLabels()[reclaimPVLabel]
		if !found {
			return nil
		}
		return []string{pvLabel}
	})
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PVCReclaim{}).
		Watches(&corev1.PersistentVolume{}, pvEnqueuePVCReclaimReconcileRequestMapFunc, builder.WithPredicates(pvPredicate)).
		Complete(r)
}
