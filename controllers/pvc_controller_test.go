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

func TestPVCController_Reconcile_PVCNotFound(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	controller := NewPVCController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pvc", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCController_Reconcile_PVCNotBound_WrongPhase(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "test-pv",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}
	fakeClient := fake.NewClientBuilder().WithObjects(pvc).Build()
	controller := NewPVCController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pvc", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCController_Reconcile_PVCNotBound_NoVolumeName(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}
	fakeClient := fake.NewClientBuilder().WithObjects(pvc).Build()
	controller := NewPVCController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pvc", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCController_Reconcile_PVNotFound(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "missing-pv",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}
	fakeClient := fake.NewClientBuilder().WithObjects(pvc).Build()
	controller := NewPVCController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pvc", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.Error(t, err)
}

func TestPVCController_Reconcile_PVCReclaimAlreadyExists(t *testing.T) {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "test-pv",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
	}
	reclaim := &v1alpha1.PVCReclaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			Labels: map[string]string{
				"pvc-reclaim.yibozhuang.me/pv-name": "test-pv",
			},
		},
		Spec: v1alpha1.PVCReclaimSpec{
			PersistentVolumeRef: &corev1.ObjectReference{
				Kind:       "PersistentVolume",
				APIVersion: "v1",
				Name:       "test-pv",
			},
			PersistentVolumeClaimSpec: pvc.Spec,
			Restore:                   false,
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(reclaim, pvc, pv).WithObjects(reclaim, pvc, pv).Build()
	controller := NewPVCController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pvc", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}

func TestPVCController_Reconcile_PVCFound(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}
	fakeClient := fake.NewClientBuilder().WithObjects(pvc).Build()
	controller := NewPVCController(fakeClient)

	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-pvc", Namespace: "default"}}
	_, err := controller.Reconcile(context.Background(), req)
	assert.NoError(t, err)
}
