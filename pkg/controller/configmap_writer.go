// Package controller provides Kubernetes controllers
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/kagenti/mcp-gateway/pkg/config"
)

// ConfigMapWriter writes ConfigMaps
type ConfigMapWriter struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// WriteAggregatedConfig writes aggregated config
func (w *ConfigMapWriter) WriteAggregatedConfig(
	ctx context.Context,
	namespace, name string,
	brokerConfig *config.BrokerConfig,
) error {
	yamlData, err := yaml.Marshal(brokerConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                        "mcp-gateway",
				"mcp.kagenti.com/aggregated": "true",
			},
		},
		Data: map[string]string{
			"config.yaml": string(yamlData),
		},
	}

	existing := &corev1.ConfigMap{}
	err = w.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return w.Client.Create(ctx, configMap)
		}
		return err
	}

	// Only update if data or labels have changed
	if !equality.Semantic.DeepEqual(existing.Data, configMap.Data) ||
		!equality.Semantic.DeepEqual(existing.Labels, configMap.Labels) {
		existing.Data = configMap.Data
		existing.Labels = configMap.Labels
		return w.Client.Update(ctx, existing)
	}

	return nil
}

// NewConfigMapWriter creates a ConfigMapWriter
func NewConfigMapWriter(client client.Client, scheme *runtime.Scheme) *ConfigMapWriter {
	return &ConfigMapWriter{
		Client: client,
		Scheme: scheme,
	}
}
