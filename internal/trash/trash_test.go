package trash

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	projectv1 "github.com/wutz/project/api/v1alpha1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := projectv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIsTrashed(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"nil labels", nil, false},
		{"ignore label", map[string]string{LabelIgnore: "true"}, true},
		{"legacy label", map[string]string{LabelLegacy: "true"}, true},
		{"ignore label false", map[string]string{LabelIgnore: "false"}, false},
		{"legacy label false", map[string]string{LabelLegacy: "false"}, false},
		{"unrelated label", map[string]string{"app": "forge"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTrashed(tt.labels); got != tt.want {
				t.Errorf("IsTrashed(%v) = %v, want %v", tt.labels, got, tt.want)
			}
		})
	}
}

func TestExpireAt(t *testing.T) {
	want := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		annotations map[string]string
		wantOK      bool
	}{
		{"valid RFC3339", map[string]string{AnnotationExpireAt: want.Format(time.RFC3339)}, true},
		{"missing", nil, false},
		{"malformed", map[string]string{AnnotationExpireAt: "tomorrow"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
			got, ok := ExpireAt(pvc)
			if ok != tt.wantOK {
				t.Fatalf("ExpireAt() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !got.Equal(want) {
				t.Errorf("ExpireAt() = %v, want %v", got, want)
			}
		})
	}
}

func TestMigrateLegacy(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name            string
		labels          map[string]string
		annotations     map[string]string
		wantLabels      map[string]string
		wantAnnotations map[string]string
	}{
		{
			name:            "migrate legacy label, trashed-at, and expire-at annotations",
			labels:          map[string]string{LabelLegacy: "true", "app": "forge"},
			annotations:     map[string]string{AnnotationLegacyTrashedAt: now.Format(time.RFC3339), AnnotationLegacyExpireAt: now.Add(time.Hour).Format(time.RFC3339)},
			wantLabels:      map[string]string{LabelIgnore: "true", "app": "forge"},
			wantAnnotations: map[string]string{AnnotationTrashedAt: now.Format(time.RFC3339), AnnotationExpireAt: now.Add(time.Hour).Format(time.RFC3339)},
		},
		{
			name:            "migrate only legacy label",
			labels:          map[string]string{LabelLegacy: "true", "app": "forge"},
			annotations:     nil,
			wantLabels:      map[string]string{LabelIgnore: "true", "app": "forge"},
			wantAnnotations: map[string]string{},
		},
		{
			name:            "migrate only legacy annotations",
			labels:          map[string]string{"app": "forge"},
			annotations:     map[string]string{AnnotationLegacyTrashedAt: now.Format(time.RFC3339)},
			wantLabels:      map[string]string{"app": "forge"},
			wantAnnotations: map[string]string{AnnotationTrashedAt: now.Format(time.RFC3339)},
		},
		{
			name:            "volume without legacy items is left alone",
			labels:          map[string]string{"app": "forge"},
			annotations:     map[string]string{AnnotationExpireAt: now.Format(time.RFC3339)},
			wantLabels:      map[string]string{"app": "forge"},
			wantAnnotations: map[string]string{AnnotationExpireAt: now.Format(time.RFC3339)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
				Name: "data", Namespace: "forge-demo", Labels: tt.labels, Annotations: tt.annotations,
			}}
			c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(pvc.DeepCopy()).Build()
			if err := MigrateLegacy(context.Background(), c, pvc); err != nil {
				t.Fatal(err)
			}
			got := &corev1.PersistentVolumeClaim{}
			if err := c.Get(context.Background(), client.ObjectKeyFromObject(pvc), got); err != nil {
				t.Fatal(err)
			}
			if !sets.KeySet(got.Labels).Equal(sets.KeySet(tt.wantLabels)) {
				t.Errorf("labels = %v, want keys %v", got.Labels, tt.wantLabels)
			}
			for k, v := range tt.wantLabels {
				if got.Labels[k] != v {
					t.Errorf("label %s = %q, want %q", k, got.Labels[k], v)
				}
			}
			if !sets.KeySet(got.Annotations).Equal(sets.KeySet(tt.wantAnnotations)) {
				t.Errorf("annotations = %v, want keys %v", got.Annotations, tt.wantAnnotations)
			}
			for k, v := range tt.wantAnnotations {
				if got.Annotations[k] != v {
					t.Errorf("annotation %s = %q, want %q", k, got.Annotations[k], v)
				}
			}
		})
	}
}

func TestRetention(t *testing.T) {
	const platform = 7 * 24 * time.Hour
	hour := metav1.Duration{Duration: time.Hour}

	tests := []struct {
		name    string
		objects []client.Object
		ns      string
		want    time.Duration
	}{
		{
			name: "project retention overrides the platform default",
			objects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "forge-demo", Labels: map[string]string{LabelProject: "demo"}}},
				&projectv1.Project{ObjectMeta: metav1.ObjectMeta{Name: "demo"}, Spec: projectv1.ProjectSpec{TrashRetention: &hour}},
			},
			ns:   "forge-demo",
			want: time.Hour,
		},
		{
			name: "project without retention uses the platform default",
			objects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "forge-demo", Labels: map[string]string{LabelProject: "demo"}}},
				&projectv1.Project{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
			},
			ns:   "forge-demo",
			want: platform,
		},
		{
			name: "namespace without project label uses the platform default",
			objects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "forge-plain"}},
			},
			ns:   "forge-plain",
			want: platform,
		},
		{
			name: "missing project uses the platform default",
			objects: []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "forge-gone", Labels: map[string]string{LabelProject: "gone"}}},
			},
			ns:   "forge-gone",
			want: platform,
		},
		{
			name:    "missing namespace uses the platform default",
			objects: nil,
			ns:      "nowhere",
			want:    platform,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(tt.objects...).Build()
			got, err := Retention(context.Background(), c, tt.ns, platform)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("Retention() = %v, want %v", got, tt.want)
			}
		})
	}
}
