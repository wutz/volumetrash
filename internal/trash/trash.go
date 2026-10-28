// Package trash is the trash contract between forgeai and this operator. A
// volume (PVC) is in the trash while it carries the file server ignore label,
// and it is purged once its expire-at annotation has passed. forgeai writes
// these fields when it trashes, restores or re-dates a volume; this operator
// only purges.
package trash

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	projectv1 "github.com/wutz/project/api/v1alpha1"
)

const (
	// LabelIgnore marks a volume in the trash. The FileServer operator does not
	// serve a volume with it.
	LabelIgnore = "fileserver.wutz.dev/ignore"
	// LabelLegacy is the trash label before FileServer 0.3.0. Volumes that still
	// carry it are moved to LabelIgnore.
	LabelLegacy = "forgeai.wutz.dev/trash"
	// LabelProject is set on project namespaces; its value is the Project name.
	LabelProject = "project.wutz.dev/project"
	// AnnotationTrashedAt is when the volume was moved to the trash (RFC3339).
	AnnotationTrashedAt = "forgeai.wutz.dev/trashed-at"
	// AnnotationExpireAt is when a trashed volume is purged (RFC3339).
	AnnotationExpireAt = "forgeai.wutz.dev/expire-at"

	literalTrue = "true"
)

// IsTrashed reports whether the labels mark a volume as trashed, including the
// legacy label.
func IsTrashed(labels map[string]string) bool {
	return labels[LabelIgnore] == literalTrue || labels[LabelLegacy] == literalTrue
}

// InTrash reports whether a PVC is in the trash.
func InTrash(pvc *corev1.PersistentVolumeClaim) bool {
	return IsTrashed(pvc.Labels)
}

// ExpireAt returns when a trashed PVC is purged.
func ExpireAt(pvc *corev1.PersistentVolumeClaim) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, pvc.Annotations[AnnotationExpireAt])
	return t, err == nil
}

// MigrateLegacy moves a volume trashed before FileServer 0.3.0 from the legacy
// label to the ignore label. Other volumes are left alone.
func MigrateLegacy(ctx context.Context, c client.Client, pvc *corev1.PersistentVolumeClaim) error {
	if pvc.Labels[LabelLegacy] != literalTrue {
		return nil
	}
	patch := client.MergeFrom(pvc.DeepCopy())
	pvc.Labels[LabelIgnore] = literalTrue
	delete(pvc.Labels, LabelLegacy)
	return c.Patch(ctx, pvc, patch)
}

// Retention returns how long a volume trashed in namespace ns is kept: the
// owning project's trashRetention when it sets one, else platformDefault. A
// namespace or project that does not exist falls back to platformDefault.
func Retention(ctx context.Context, c client.Reader, ns string, platformDefault time.Duration) (time.Duration, error) {
	nsObj := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: ns}, nsObj); err != nil {
		return platformDefault, client.IgnoreNotFound(err)
	}
	// A namespace without a project label has no project to ask: that is the
	// platform default, not a lookup of a project with an empty name.
	name := nsObj.Labels[LabelProject]
	if name == "" {
		return platformDefault, nil
	}
	p := &projectv1.Project{}
	if err := c.Get(ctx, types.NamespacedName{Name: name}, p); err != nil {
		return platformDefault, client.IgnoreNotFound(err)
	}
	if p.Spec.TrashRetention != nil {
		return p.Spec.TrashRetention.Duration, nil
	}
	return platformDefault, nil
}
