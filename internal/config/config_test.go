package config

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace = "forgeai-system"
	testName      = DefaultConfigMapName
)

func configMap(data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: testNamespace},
		Data:       map[string]string{configKey: data},
	}
}

func TestLoaderGet(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		objects []*corev1.ConfigMap
		want    time.Duration
		wantErr bool
	}{
		{
			name: "reads trashRetention and ignores other keys",
			objects: []*corev1.ConfigMap{configMap(`
namespacePrefix: forge-
storageTypes:
  shared: {storageClass: shared}
defaultStorageType: shared
trashRetention: 24h
`)},
			want: 24 * time.Hour,
		},
		{
			name:    "unset trashRetention uses the default",
			objects: []*corev1.ConfigMap{configMap("namespacePrefix: forge-\n")},
			want:    DefaultTrashRetention,
		},
		{
			name:    "empty config uses the default",
			objects: []*corev1.ConfigMap{configMap("")},
			want:    DefaultTrashRetention,
		},
		{
			name:    "malformed config is an error",
			objects: []*corev1.ConfigMap{configMap("trashRetention: [")},
			wantErr: true,
		},
		{
			name:    "invalid duration is an error",
			objects: []*corev1.ConfigMap{configMap("trashRetention: forever")},
			wantErr: true,
		},
		{
			name:    "missing ConfigMap is an error",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := fake.NewClientBuilder().WithScheme(s)
			for _, cm := range tt.objects {
				b = b.WithObjects(cm)
			}
			l := &Loader{Reader: b.Build(), Namespace: testNamespace, Name: testName}

			cfg, err := l.Get(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if cfg.TrashRetention.Duration != tt.want {
				t.Errorf("TrashRetention = %v, want %v", cfg.TrashRetention.Duration, tt.want)
			}
		})
	}
}
