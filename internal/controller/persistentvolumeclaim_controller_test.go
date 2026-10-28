/*
Copyright 2026.

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

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/wutz/volumetrash/internal/trash"
)

const (
	timeout  = 10 * time.Second
	interval = 250 * time.Millisecond
)

// newPVC returns a volume as the platform creates it.
func newPVC(ns, name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}},
		},
	}
}

// readPVC returns the named volume.
func readPVC(ns, name string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, pvc)).To(Succeed())
	return pvc
}

// isPurged reports whether the named volume was deleted. envtest runs no
// kube-controller-manager, so the pvc-protection finalizer the apiserver adds
// is never removed; a purged claim then stays with a deletion timestamp.
func isPurged(ns, name string) bool {
	pvc := &corev1.PersistentVolumeClaim{}
	err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, pvc)
	if apierrors.IsNotFound(err) {
		return true
	}
	return err == nil && !pvc.DeletionTimestamp.IsZero()
}

// markTrashed trashes a volume the way forgeai does: the ignore label, and the
// expiry annotation when expire is not zero.
func markTrashed(ns, name string, expire time.Time) {
	pvc := readPVC(ns, name)
	patch := client.MergeFrom(pvc.DeepCopy())
	if pvc.Labels == nil {
		pvc.Labels = map[string]string{}
	}
	pvc.Labels[trash.LabelIgnore] = "true"
	if !expire.IsZero() {
		if pvc.Annotations == nil {
			pvc.Annotations = map[string]string{}
		}
		pvc.Annotations[trash.AnnotationExpireAt] = expire.UTC().Format(time.RFC3339)
	}
	Expect(k8sClient.Patch(ctx, pvc, patch)).To(Succeed())
}

var _ = Describe("volume trash", Ordered, func() {
	It("purges an expired volume and keeps one that has not expired", func() {
		Expect(k8sClient.Create(ctx, newPVC("forge-demo", "keep"))).To(Succeed())
		Expect(k8sClient.Create(ctx, newPVC("forge-demo", "scratch"))).To(Succeed())
		markTrashed("forge-demo", "keep", time.Now().Add(time.Hour))
		markTrashed("forge-demo", "scratch", time.Now().Add(time.Second))

		Eventually(func() bool { return isPurged("forge-demo", "scratch") }, timeout, interval).Should(BeTrue())
		Consistently(func() bool { return isPurged("forge-demo", "keep") }, 2*time.Second, interval).Should(BeFalse())
	})

	It("gives a volume trashed without an expiry its project's retention", func() {
		Expect(k8sClient.Create(ctx, newPVC("forge-demo", "labelled"))).To(Succeed())
		markTrashed("forge-demo", "labelled", time.Time{})

		// The project sets a one hour retention, so the purge is an hour away.
		Eventually(func() time.Time {
			t, _ := trash.ExpireAt(readPVC("forge-demo", "labelled"))
			return t
		}, timeout, interval).Should(BeTemporally("~", time.Now().Add(time.Hour), 5*time.Minute))
		Expect(readPVC("forge-demo", "labelled").Annotations).To(HaveKey(trash.AnnotationTrashedAt))
		Consistently(func() bool { return isPurged("forge-demo", "labelled") }, time.Second, interval).Should(BeFalse())
	})

	It("moves the legacy trash label to the ignore label", func() {
		pvc := newPVC("forge-plain", "old")
		pvc.Labels = map[string]string{trash.LabelLegacy: "true"}
		pvc.Annotations = map[string]string{trash.AnnotationExpireAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
		Expect(k8sClient.Create(ctx, pvc)).To(Succeed())

		Eventually(func() map[string]string {
			return readPVC("forge-plain", "old").Labels
		}, timeout, interval).Should(And(
			HaveKeyWithValue(trash.LabelIgnore, "true"),
			Not(HaveKey(trash.LabelLegacy)),
		))
	})

	It("postpones the purge while a pod still mounts the volume", func() {
		Expect(k8sClient.Create(ctx, newPVC("forge-plain", "busy"))).To(Succeed())
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "forge-plain"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
				Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "busy"},
				}}},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status.Phase = corev1.PodRunning
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		markTrashed("forge-plain", "busy", time.Now().Add(time.Second))
		Consistently(func() bool { return isPurged("forge-plain", "busy") }, 3*time.Second, interval).Should(BeFalse())

		// The finished pod wakes the postponed purge; no timer is needed.
		pod.Status.Phase = corev1.PodSucceeded
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		Eventually(func() bool { return isPurged("forge-plain", "busy") }, timeout, interval).Should(BeTrue())
	})
})
