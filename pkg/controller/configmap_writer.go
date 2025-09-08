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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	internalconfig "github.com/kagenti/mcp-gateway/internal/config"
	"github.com/kagenti/mcp-gateway/pkg/config"
)

// ConfigMapWriter writes ConfigMaps
type ConfigMapWriter struct {
	Client       client.Client
	Scheme       *runtime.Scheme
	ConfigPusher *ConfigPusher
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
			if err := w.Client.Create(ctx, configMap); err != nil {
				return err
			}
			if w.ConfigPusher != nil {
				servers := convertToInternalFormat(brokerConfig.Servers)
				if err := w.ConfigPusher.PushConfig(ctx, servers); err != nil {
					logger := log.FromContext(ctx)
					logger.Error(err, "Failed to push config to broker")
				}
			}
			return nil
		}
		return err
	}

	// Only update if data or labels have changed
	if !equality.Semantic.DeepEqual(existing.Data, configMap.Data) ||
		!equality.Semantic.DeepEqual(existing.Labels, configMap.Labels) {
		existing.Data = configMap.Data
		existing.Labels = configMap.Labels
		if err := w.Client.Update(ctx, existing); err != nil {
			return err
		}

		if w.ConfigPusher != nil {
			servers := convertToInternalFormat(brokerConfig.Servers)
			if err := w.ConfigPusher.PushConfig(ctx, servers); err != nil {
				// log error but don't fail - configmap is still updated
				// broker will eventually pick it up via file watch
				logger := log.FromContext(ctx)
				logger.Error(err, "Failed to push config to broker")
			}
		}
	}

	return nil
}

// NewConfigMapWriter creates a ConfigMapWriter
func NewConfigMapWriter(client client.Client, scheme *runtime.Scheme) *ConfigMapWriter {
	return &ConfigMapWriter{
		Client:       client,
		Scheme:       scheme,
		ConfigPusher: NewConfigPusher(),
	}
}

// convertToInternalFormat converts from pkg/config to internal/config format
func convertToInternalFormat(servers []config.ServerConfig) []*internalconfig.MCPServer {
	result := make([]*internalconfig.MCPServer, len(servers))
	for i, s := range servers {
		result[i] = &internalconfig.MCPServer{
			Name:       s.Name,
			URL:        s.URL,
			ToolPrefix: s.ToolPrefix,
			Enabled:    s.Enabled,
			Hostname:   s.Hostname,
		}
	}
	return result
}
