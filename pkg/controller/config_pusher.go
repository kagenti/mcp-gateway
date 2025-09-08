package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/kagenti/mcp-gateway/internal/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

// ConfigPusher pushes configuration updates to the broker
type ConfigPusher struct {
	brokerURL string
	authToken string
	client    *http.Client
}

// NewConfigPusher creates a new config pusher
func NewConfigPusher() *ConfigPusher {
	brokerURL := os.Getenv("BROKER_CONFIG_URL")
	if brokerURL == "" {
		brokerURL = "http://mcp-broker.mcp-system.svc.cluster.local:8080/config"
	}

	authToken := os.Getenv("CONFIG_UPDATE_TOKEN")

	return &ConfigPusher{
		brokerURL: brokerURL,
		authToken: authToken,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// PushConfig pushes configuration to the broker
func (p *ConfigPusher) PushConfig(ctx context.Context, servers []*config.MCPServer) error {
	logger := log.FromContext(ctx)

	configData := struct {
		Servers []*config.MCPServer `yaml:"servers"`
	}{
		Servers: servers,
	}

	yamlData, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.brokerURL, bytes.NewBuffer(yamlData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/yaml")
	if p.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.authToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send config update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("broker returned status %d", resp.StatusCode)
	}

	logger.Info("Successfully pushed config to broker", "serverCount", len(servers))
	return nil
}
