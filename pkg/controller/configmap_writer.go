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

	// Retry with exponential backoff for conflict errors
	return wait.ExponentialBackoff(wait.Backoff{
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		Steps:    5,
	}, func() (bool, error) {
		existing := &corev1.ConfigMap{}
		err := w.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
		if err != nil {
			if errors.IsNotFound(err) {
				err = w.Client.Create(ctx, configMap)
				if errors.IsAlreadyExists(err) {
					// Someone else created it, retry
					return false, nil
				}
				if err == nil && w.ConfigPusher != nil {
					servers := convertToInternalFormat(brokerConfig.Servers)
					if pushErr := w.ConfigPusher.PushConfig(ctx, servers); pushErr != nil {
						logger := log.FromContext(ctx)
						logger.Error(pushErr, "Failed to push config to broker")
					}
				}
				return err == nil, err
			}
			return false, err
		}

		// Only update if data or labels have changed
		if !equality.Semantic.DeepEqual(existing.Data, configMap.Data) ||
			!equality.Semantic.DeepEqual(existing.Labels, configMap.Labels) {
			existing.Data = configMap.Data
			existing.Labels = configMap.Labels
			err = w.Client.Update(ctx, existing)
			if errors.IsConflict(err) {
				// Resource conflict, retry
				return false, nil
			}
			if err == nil && w.ConfigPusher != nil {
				servers := convertToInternalFormat(brokerConfig.Servers)
				if pushErr := w.ConfigPusher.PushConfig(ctx, servers); pushErr != nil {
					// log error but don't fail - configmap is still updated
					// broker will eventually pick it up via file watch
					logger := log.FromContext(ctx)
					logger.Error(pushErr, "Failed to push config to broker")
				}
			}
			return err == nil, err
		}

		return true, nil
	})
}

// NewConfigMapWriter creates a ConfigMapWriter
func NewConfigMapWriter(client client.Client, scheme *runtime.Scheme) *ConfigMapWriter {
	return &ConfigMapWriter{
		Client:       client,
		Scheme:       scheme,
		ConfigPusher: NewConfigPusher(client),
	}
}

// convertToInternalFormat converts from pkg/config to internal/config format
func convertToInternalFormat(servers []config.ServerConfig) []*internalconfig.MCPServer {
	result := make([]*internalconfig.MCPServer, len(servers))
	for i, s := range servers {
		result[i] = &internalconfig.MCPServer{
			Name:             s.Name,
			URL:              s.URL,
			ToolPrefix:       s.ToolPrefix,
			Enabled:          s.Enabled,
			Hostname:         s.Hostname,
			CredentialEnvVar: s.CredentialEnvVar,
		}
	}
	return result
}
