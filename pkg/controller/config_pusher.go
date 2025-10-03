package controller

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/kagenti/mcp-gateway/internal/config"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

// ConfigPusher pushes configuration updates to all broker instances
type ConfigPusher struct {
	k8sClient  client.Client
	authToken  string
	httpClient *http.Client
	namespace  string
}

// NewConfigPusher creates a new config pusher
func NewConfigPusher(k8sClient client.Client) *ConfigPusher {
	authToken := os.Getenv("CONFIG_UPDATE_TOKEN")

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "mcp-system"
	}

	return &ConfigPusher{
		k8sClient: k8sClient,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		namespace: namespace,
	}
}

// PushConfig pushes configuration to all broker instances via endpoints
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

	// get endpoints for the config service
	endpoints := &corev1.Endpoints{} //nolint:staticcheck
	err = p.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: p.namespace,
		Name:      "mcp-config",
	}, endpoints)
	if err != nil {
		logger.Error(err, "Failed to get endpoints for mcp-config service")
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	// collect all endpoint addresses
	var addresses []string
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			// use the config port
			url := fmt.Sprintf("http://%s/config", net.JoinHostPort(addr.IP, "8181"))
			addresses = append(addresses, url)
		}
	}

	if len(addresses) == 0 {
		logger.V(1).Info("No broker endpoints found, skipping config push")
		return nil
	}

	// push to all endpoints concurrently with bounded parallelism
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for _, addr := range addresses {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			sem <- struct{}{}        // acquire semaphore
			defer func() { <-sem }() // release semaphore

			if err := p.pushToEndpoint(ctx, url, yamlData); err != nil {
				logger.Error(err, "Failed to push config to endpoint", "url", url)
				mu.Lock()
				errors = append(errors, fmt.Errorf("%s: %w", url, err))
				mu.Unlock()
			} else {
				logger.V(1).Info("Successfully pushed config to endpoint", "url", url)
			}
		}(addr)
	}

	wg.Wait()

	if len(errors) > 0 {
		// log but don't fail if some endpoints couldn't be updated
		// they'll eventually get the config via configmap watching
		logger.Error(fmt.Errorf("%d errors", len(errors)),
			"Failed to push config to some endpoints",
			"failed", len(errors),
			"total", len(addresses))
	}

	logger.V(1).Info("Config push completed",
		"successful", len(addresses)-len(errors),
		"failed", len(errors),
		"total", len(addresses),
		"serverCount", len(servers))

	return nil
}

// pushToEndpoint pushes config to a single broker endpoint
func (p *ConfigPusher) pushToEndpoint(ctx context.Context, url string, yamlData []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(yamlData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/yaml")
	if p.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.authToken)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received status %d", resp.StatusCode)
	}

	return nil
}
