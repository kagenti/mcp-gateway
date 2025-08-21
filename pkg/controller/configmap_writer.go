package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	"github.com/kagenti/mcp-gateway/pkg/config"
)

type ConfigMapWriter struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (w *ConfigMapWriter) WriteConfig(ctx context.Context, owner *mcpv1alpha1.MCPGateway, brokerConfig *config.BrokerConfig) error {
	namespace := owner.Namespace
	name := owner.Name
	configMapName := fmt.Sprintf("mcp-broker-config-%s", name)

	yamlData, err := yaml.Marshal(brokerConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                     "mcp-broker",
				"mcp.kagenti.com/gateway": name,
			},
		},
		Data: map[string]string{
			"config.yaml": string(yamlData),
		},
	}

	if err := controllerutil.SetControllerReference(owner, configMap, w.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err = w.Client.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			return w.Client.Create(ctx, configMap)
		}
		return err
	}

	existing.Data = configMap.Data
	existing.Labels = configMap.Labels
	// Owner references are preserved from existing
	return w.Client.Update(ctx, existing)
}

func (w *ConfigMapWriter) WriteAggregatedConfig(ctx context.Context, namespace, name string, brokerConfig *config.BrokerConfig) error {
	yamlData, err := yaml.Marshal(brokerConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                        "mcp-broker",
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

	existing.Data = configMap.Data
	existing.Labels = configMap.Labels
	return w.Client.Update(ctx, existing)
}

func NewConfigMapWriter(client client.Client, scheme *runtime.Scheme) *ConfigMapWriter {
	return &ConfigMapWriter{
		Client: client,
		Scheme: scheme,
	}
}
