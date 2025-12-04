// Package controller provides Kubernetes controllers
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kagenti/mcp-gateway/pkg/config"
)

// ConfigMapWriter writes ConfigMaps
type ConfigMapWriter struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// WriteAggregatedConfig writes aggregated config with retry logic for conflicts
func (w *ConfigMapWriter) WriteAggregatedConfig(
	ctx context.Context,
	namespace, name string,
	brokerConfig *config.BrokerConfig,
) error {
	yamlData, err := yaml.Marshal(brokerConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                        "mcp-gateway",
				"mcp.kagenti.com/aggregated": "true",
			},
		},
		StringData: map[string]string{
			"config.yaml": string(yamlData),
		},
	}

	// Retry with exponential backoff for conflict errors
	return wait.ExponentialBackoff(wait.Backoff{
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		Steps:    5,
	}, func() (bool, error) {
		existing := &corev1.Secret{}
		err := w.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
		if err != nil {
			if errors.IsNotFound(err) {
				err = w.Client.Create(ctx, secret)
				if errors.IsAlreadyExists(err) {
					// Someone else created it, retry
					return false, nil
				}
				return err == nil, err
			}
			return false, err
		}

		// Only update if data or labels have changed
		if !equality.Semantic.DeepEqual(existing.StringData, secret.StringData) ||
			!equality.Semantic.DeepEqual(existing.Labels, secret.Labels) {
			existing.StringData = secret.StringData
			existing.Labels = secret.Labels
			err = w.Client.Update(ctx, existing)
			if errors.IsConflict(err) {
				// Resource conflict, retry
				return false, nil
			}
			return err == nil, err
		}

		return true, nil
	})
}

// NewSecretWriter creates a SecretConfigWriter
func NewSecretWriter(client client.Client, scheme *runtime.Scheme) *ConfigMapWriter {
	return &ConfigMapWriter{
		Client: client,
		Scheme: scheme,
	}
}
