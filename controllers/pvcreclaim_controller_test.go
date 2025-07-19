package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yibozhuang/pvc-reclaim/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPVCReclaimController_Reconcile_ReclaimNotFound(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	controller := NewPVCReclaimController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-reclaim", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCReclaimController_Reconcile_PVCNotBound(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)

	// PVC exists but not Bound
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-reclaim",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "test-reclaim-pv",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(pvc).WithObjects(pvc).Build()
	controller := NewPVCReclaimController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-reclaim", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCReclaimController_Reconcile_PVNotFoundForReclaim(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)

	reclaim := &v1alpha1.PVCReclaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-reclaim",
			Namespace: "default",
		},
		Spec: v1alpha1.PVCReclaimSpec{
			PersistentVolumeRef: &corev1.ObjectReference{
				Kind:       "PersistentVolume",
				APIVersion: "v1",
				Name:       "missing-pv",
			},
			Restore: false,
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(reclaim).WithObjects(reclaim).Build()
	controller := NewPVCReclaimController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-reclaim", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCReclaimController_Reconcile_PVNotReleased(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)

	reclaim := &v1alpha1.PVCReclaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-reclaim",
			Namespace: "default",
		},
		Spec: v1alpha1.PVCReclaimSpec{
			PersistentVolumeRef: &corev1.ObjectReference{
				Kind:       "PersistentVolume",
				APIVersion: "v1",
				Name:       "test-pv",
			},
			Restore: true,
		},
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeBound,
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(reclaim, pv).WithObjects(reclaim, pv).Build()
	controller := NewPVCReclaimController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-reclaim", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCReclaimController_Reconcile_PVCReclaimRestore_Success(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)

	reclaim := &v1alpha1.PVCReclaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-reclaim",
			Namespace: "default",
		},
		Spec: v1alpha1.PVCReclaimSpec{
			PersistentVolumeRef: &corev1.ObjectReference{
				Kind:       "PersistentVolume",
				APIVersion: "v1",
				Name:       "test-pv",
			},
			PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
			Restore: true,
		},
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeReleased,
		},
		Spec: corev1.PersistentVolumeSpec{
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-reclaim",
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(reclaim, pv).WithObjects(reclaim, pv).Build()
	controller := NewPVCReclaimController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-reclaim", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

// Existing test
func TestPVCReclaimController_Reconcile_ReclaimFound(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)

	// Setup UIDs for cross-referencing
	pvcUID := types.UID("test-pvc-uid")

	// Create PVC that is Bound and references the PV
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-reclaim",
			Namespace: "default",
			UID:       pvcUID,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "test-reclaim-pv",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}

	// Create PV that is Released and references the PVC
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-reclaim-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			ClaimRef: &corev1.ObjectReference{
				Namespace: "default",
				Name:      "test-reclaim",
				UID:       pvcUID,
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeReleased,
		},
	}

	// Create PVCReclaim that references the PV
	reclaim := &v1alpha1.PVCReclaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-reclaim",
			Namespace: "default",
		},
		Spec: v1alpha1.PVCReclaimSpec{
			PersistentVolumeRef: &corev1.ObjectReference{
				Kind:       "PersistentVolume",
				APIVersion: "v1",
				Name:       "test-reclaim-pv",
			},
			PersistentVolumeClaimSpec: pvc.Spec,
			Restore:                   false,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(reclaim, pvc, pv).Build()
	controller := NewPVCReclaimController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-reclaim", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}
