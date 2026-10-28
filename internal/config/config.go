// Package config reads the platform settings the operator needs from the
// platform ConfigMap that forgeai renders. Only trashRetention is read; the
// other keys belong to forgeai and the project operator, which ignore the keys
// they do not use.
package config

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// DefaultConfigMapName is the platform ConfigMap that forgeai renders.
	DefaultConfigMapName = "forgeai-platform"
	// DefaultTrashRetention is kept when the platform config does not set one.
	DefaultTrashRetention = 7 * 24 * time.Hour

	// configKey holds the platform config in the ConfigMap.
	configKey = "config.yaml"
)

// Config is the part of the platform config the operator reads.
type Config struct {
	// TrashRetention is the default retention of deleted volumes.
	TrashRetention metav1.Duration `json:"trashRetention,omitzero"`
}

// Loader reads the platform config on every call. Trashed volumes are rare, so
// the read is not cached, and a change to the ConfigMap applies to the next purge
// schedule.
type Loader struct {
	Reader    client.Reader
	Namespace string
	Name      string
}

// Get returns the platform config, with the default trash retention applied.
func (l *Loader) Get(ctx context.Context) (*Config, error) {
	cm := &corev1.ConfigMap{}
	if err := l.Reader.Get(ctx, types.NamespacedName{Namespace: l.Namespace, Name: l.Name}, cm); err != nil {
		return nil, fmt.Errorf("get platform config %s/%s: %w", l.Namespace, l.Name, err)
	}
	c := &Config{}
	if err := yaml.Unmarshal([]byte(cm.Data[configKey]), c); err != nil {
		return nil, fmt.Errorf("parse platform config: %w", err)
	}
	if c.TrashRetention.Duration == 0 {
		c.TrashRetention.Duration = DefaultTrashRetention
	}
	return c, nil
}
