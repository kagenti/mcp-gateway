//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec" // needed for setupPortForward
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
)

var _ = Describe("MCP Gateway Happy Path", func() {
	var (
		httpRoute1 *gatewayapiv1.HTTPRoute
		httpRoute2 *gatewayapiv1.HTTPRoute
		mcpServer1 *mcpv1alpha1.MCPServer
		mcpServer2 *mcpv1alpha1.MCPServer
	)

	BeforeEach(func() {
		// create httproutes for test servers that should already be deployed
		httpRoute1 = BuildTestHTTPRoute("e2e-server1-route", TestNamespace,
			"server1.mcp.example.com", "mcp-test-server1", 9090)
		httpRoute2 = BuildTestHTTPRoute("e2e-server2-route", TestNamespace,
			"server2.mcp.example.com", "mcp-test-server2", 9090)

		// create mcpserver resources
		mcpServer1 = BuildTestMCPServer("e2e-mcpserver1", TestNamespace,
			"e2e-server1-route", "s1_")
		mcpServer2 = BuildTestMCPServer("e2e-mcpserver2", TestNamespace,
			"e2e-server2-route", "s2_")
	})

	AfterEach(func() {
		// cleanup in reverse order
		if mcpServer1 != nil {
			CleanupResource(ctx, k8sClient, mcpServer1)
		}
		if mcpServer2 != nil {
			CleanupResource(ctx, k8sClient, mcpServer2)
		}
		if httpRoute1 != nil {
			CleanupResource(ctx, k8sClient, httpRoute1)
		}
		if httpRoute2 != nil {
			CleanupResource(ctx, k8sClient, httpRoute2)
		}
	})

	JustAfterEach(func() {
		// dump logs if test failed
		if CurrentSpecReport().Failed() {
			DumpComponentLogs()
			DumpTestServerLogs()
		}
	})

	It("should aggregate MCP servers and manage HTTPRoute conditions", func() {
		By("Creating HTTPRoutes")
		// Clean up any existing resources first
		_ = k8sClient.Delete(ctx, httpRoute1)
		_ = k8sClient.Delete(ctx, httpRoute2)
		_ = k8sClient.Delete(ctx, mcpServer1)
		_ = k8sClient.Delete(ctx, mcpServer2)
		// Wait a moment for deletion to process
		time.Sleep(2 * time.Second)

		Expect(k8sClient.Create(ctx, httpRoute1)).To(Succeed())
		Expect(k8sClient.Create(ctx, httpRoute2)).To(Succeed())

		By("Creating MCPServer resources")
		Expect(k8sClient.Create(ctx, mcpServer1)).To(Succeed())
		Expect(k8sClient.Create(ctx, mcpServer2)).To(Succeed())

		By("Verifying MCPServers become ready")
		VerifyMCPServerReady(ctx, k8sClient, mcpServer1.Name, mcpServer1.Namespace)
		VerifyMCPServerReady(ctx, k8sClient, mcpServer2.Name, mcpServer2.Namespace)

		By("Setting up port-forward to broker for status check")
		statusPortForwardCmd := setupPortForward("mcp-broker-router", SystemNamespace, "18081:8080")
		defer statusPortForwardCmd.Process.Kill()

		// wait for port-forward to be ready
		Eventually(func() error {
			client := &http.Client{Timeout: 1 * time.Second}
			resp, err := client.Get("http://localhost:18081/status")
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed(), "Port-forward should be ready")

		By("Waiting for broker to register servers")
		// wait for broker to load the config and be ready
		// note: configmap volume mounts can take 60-120s to sync in kubernetes
		// plus additional time for fsnotify to detect and reload
		// use broker /status endpoint to reliably check server registration
		type StatusResponse struct {
			Servers []struct {
				URL              string `json:"url"`
				Name             string `json:"name"`
				ToolPrefix       string `json:"toolPrefix"`
				ConnectionStatus struct {
					IsReachable bool `json:"isReachable"`
				} `json:"connectionStatus"`
			} `json:"servers"`
			HealthyServers int `json:"healthyServers"`
			TotalServers   int `json:"totalServers"`
		}

		Eventually(func() bool {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get("http://localhost:18081/status")
			if err != nil {
				GinkgoWriter.Printf("Failed to connect to broker status endpoint: %v\n", err)
				return false
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				GinkgoWriter.Printf("Broker status endpoint returned %d\n", resp.StatusCode)
				return false
			}

			var status StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				GinkgoWriter.Printf("Failed to decode status response: %v\n", err)
				return false
			}

			// check if both servers are registered and healthy
			foundServer1 := false
			foundServer2 := false
			for _, server := range status.Servers {
				if strings.Contains(server.URL, "mcp-test-server1") {
					foundServer1 = server.ToolPrefix == "s1_" && server.ConnectionStatus.IsReachable
					if !foundServer1 {
						GinkgoWriter.Printf("Server1 found but not ready - prefix: %s, reachable: %v\n",
							server.ToolPrefix, server.ConnectionStatus.IsReachable)
					}
				}
				if strings.Contains(server.URL, "mcp-test-server2") {
					foundServer2 = server.ToolPrefix == "s2_" && server.ConnectionStatus.IsReachable
					if !foundServer2 {
						GinkgoWriter.Printf("Server2 found but not ready - prefix: %s, reachable: %v\n",
							server.ToolPrefix, server.ConnectionStatus.IsReachable)
					}
				}
			}

			if !foundServer1 || !foundServer2 {
				GinkgoWriter.Printf("Broker status - Total: %d, Healthy: %d, Server1: %v, Server2: %v\n",
					status.TotalServers, status.HealthyServers, foundServer1, foundServer2)
			}

			return foundServer1 && foundServer2 && status.HealthyServers >= 2
		}, TestTimeoutConfigSync, 5*time.Second).Should(BeTrue(),
			"Broker should register both test servers with correct prefixes and be healthy")

		By("Setting up port-forward to broker MCP endpoint")
		brokerURL := "http://localhost:18080/mcp"
		portForwardCmd := setupPortForward("mcp-broker-router", SystemNamespace, "18080:8080")
		defer portForwardCmd.Process.Kill() // ensure we clean up

		// wait for port-forward to be ready by testing connection
		Eventually(func() error {
			client := &http.Client{Timeout: 1 * time.Second}
			resp, err := client.Get(brokerURL)
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
			"Port-forward should be ready")

		By("Testing broker MCP endpoint")
		// test initialize
		initReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "0.1.0",
				"capabilities":    map[string]interface{}{},
				"clientInfo": map[string]interface{}{
					"name":    "e2e-test",
					"version": "1.0.0",
				},
			},
			"id": 1,
		}

		resp, sessionID := makeMCPRequestWithSession(brokerURL, initReq)
		Expect(resp).To(HaveKey("result"))
		result := resp["result"].(map[string]interface{})
		Expect(result).To(HaveKey("serverInfo"))
		serverInfo := result["serverInfo"].(map[string]interface{})
		Expect(serverInfo["name"]).To(Equal("Kagenti MCP Broker"))

		// verify broker capabilities indicate tool support
		Expect(result).To(HaveKey("capabilities"))
		capabilities := result["capabilities"].(map[string]interface{})
		Expect(capabilities).To(HaveKey("tools"))
		tools := capabilities["tools"].(map[string]interface{})
		Expect(tools["listChanged"]).To(Equal(true))

		By("Listing tools from broker")
		toolsReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"params":  map[string]interface{}{},
			"id":      2,
		}

		toolsResp := makeMCPRequestWithSessionID(brokerURL, toolsReq, sessionID)
		Expect(toolsResp).To(HaveKey("result"))
		toolsResult := toolsResp["result"].(map[string]interface{})
		Expect(toolsResult).To(HaveKey("tools"))
		toolsList := toolsResult["tools"].([]interface{})

		// should have tools from both test servers
		Expect(len(toolsList)).To(BeNumerically(">", 0))

		// check for tools with expected prefixes
		var foundS1Tool, foundS2Tool bool
		for _, tool := range toolsList {
			toolMap := tool.(map[string]interface{})
			name := toolMap["name"].(string)
			if strings.HasPrefix(name, "s1_") {
				foundS1Tool = true
			}
			if strings.HasPrefix(name, "s2_") {
				foundS2Tool = true
			}
		}
		Expect(foundS1Tool).To(BeTrue(), "Should find tools with s1_ prefix")
		Expect(foundS2Tool).To(BeTrue(), "Should find tools with s2_ prefix")

		By("Deleting one MCPServer")
		Expect(k8sClient.Delete(ctx, mcpServer1)).To(Succeed())

		By("Verifying broker removes the deleted server")
		// use status endpoint to verify server removal
		Eventually(func() bool {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get("http://localhost:18081/status")
			if err != nil {
				return false
			}
			defer resp.Body.Close()

			var status StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				return false
			}

			// should only have server2 now
			hasServer1 := false
			hasServer2 := false
			for _, server := range status.Servers {
				if strings.Contains(server.URL, "mcp-test-server1") {
					hasServer1 = true
				}
				if strings.Contains(server.URL, "mcp-test-server2") {
					hasServer2 = true
				}
			}

			// server1 should be gone, server2 should remain
			return !hasServer1 && hasServer2
		}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue(), "Broker should remove deleted server")

		By("Testing server failure recovery")
		// simulate server failure by scaling down
		testServerDeployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      "mcp-test-server2",
			Namespace: TestNamespace,
		}, testServerDeployment)).To(Succeed())

		By("Scaling down test-server2 to simulate failure")
		zero := int32(0)
		testServerDeployment.Spec.Replicas = &zero
		Expect(k8sClient.Update(ctx, testServerDeployment)).To(Succeed())

		By("Waiting for broker to detect server2 is unreachable")
		Eventually(func() bool {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get("http://localhost:18081/status")
			if err != nil {
				return false
			}
			defer resp.Body.Close()

			var status StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				return false
			}

			// check if server2 is marked as unreachable or removed
			for _, server := range status.Servers {
				if strings.Contains(server.URL, "mcp-test-server2") {
					return !server.ConnectionStatus.IsReachable
				}
			}
			// if not found, that's also acceptable
			return true
		}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue(), "Server2 should become unreachable")

		By("Verifying server remains unreachable for a few seconds")
		// Just verify the server stays unreachable for a bit to ensure retry is happening
		// We already confirmed it's unreachable above, this just ensures it doesn't immediately recover
		Consistently(func() bool {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get("http://localhost:18081/status")
			if err != nil {
				return true // connection error means still down
			}
			defer resp.Body.Close()

			var status StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				return true // decode error means still down
			}

			// check if server2 is still unreachable
			for _, server := range status.Servers {
				if strings.Contains(server.URL, "mcp-test-server2") {
					return !server.ConnectionStatus.IsReachable
				}
			}
			// not found means it's been removed, which is fine
			return true
		}, "3s", "1s").Should(BeTrue(), "Server should remain unreachable while down")

		By("Scaling test-server2 back up")
		// refresh deployment to avoid resource version conflicts
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      "mcp-test-server2",
			Namespace: TestNamespace,
		}, testServerDeployment)).To(Succeed())
		one := int32(1)
		testServerDeployment.Spec.Replicas = &one
		Expect(k8sClient.Update(ctx, testServerDeployment)).To(Succeed())

		By("Waiting for pod to be running")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			err := k8sClient.List(ctx, pods,
				client.InNamespace(TestNamespace),
				client.MatchingLabels{"app": "mcp-test-server2"})
			if err != nil || len(pods.Items) == 0 {
				return false
			}
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					return true
				}
			}
			return false
		}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue())

		By("Verifying broker reconnects to server2")
		Eventually(func() bool {
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get("http://localhost:18081/status")
			if err != nil {
				return false
			}
			defer resp.Body.Close()

			var status StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				return false
			}

			// check if server2 is back and healthy
			for _, server := range status.Servers {
				if strings.Contains(server.URL, "mcp-test-server2") {
					GinkgoWriter.Printf("Server2 recovery status - Reachable: %v\n",
						server.ConnectionStatus.IsReachable)
					return server.ConnectionStatus.IsReachable
				}
			}
			return false
		}, TestTimeoutLong, TestRetryInterval).Should(BeTrue(), "Server2 should recover and reconnect")

		By("Test completed successfully")
	})

	It("should handle credential changes and re-register servers", func() {
		By("Creating HTTPRoute for api-key-server")
		httpRouteApiKey := BuildTestHTTPRoute("e2e-apikey-route", TestNamespace,
			"apikey.mcp.example.com", "mcp-api-key-server", 9090)
		_ = k8sClient.Delete(ctx, httpRouteApiKey)
		time.Sleep(2 * time.Second)
		Expect(k8sClient.Create(ctx, httpRouteApiKey)).To(Succeed())
		defer CleanupResource(ctx, k8sClient, httpRouteApiKey)

		By("Creating credential secret with valid token")
		credentialSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-apikey-credentials",
				Namespace: TestNamespace,
				Labels: map[string]string{
					"mcp.kagenti.com/credential": "true",
				},
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"token": "Bearer test-api-key-secret-token", // valid token
			},
		}
		_ = k8sClient.Delete(ctx, credentialSecret)
		time.Sleep(2 * time.Second)
		Expect(k8sClient.Create(ctx, credentialSecret)).To(Succeed())
		defer CleanupResource(ctx, k8sClient, credentialSecret)

		// wait for secret to be fully created and readable
		By("Verifying credential secret is created with data")
		Eventually(func() bool {
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "e2e-apikey-credentials",
				Namespace: TestNamespace,
			}, secret); err != nil {
				return false
			}
			// check secret has the token data
			if secret.Data == nil || len(secret.Data["token"]) == 0 {
				return false
			}
			return string(secret.Data["token"]) == "Bearer test-api-key-secret-token"
		}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue())

		By("Creating MCPServer with credential reference")
		mcpServerApiKey := &mcpv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-apikey-server",
				Namespace: TestNamespace,
			},
			Spec: mcpv1alpha1.MCPServerSpec{
				ToolPrefix: "e2ecred_", // unique prefix to avoid conflicts
				TargetRef: mcpv1alpha1.TargetReference{
					Group: "gateway.networking.k8s.io",
					Kind:  "HTTPRoute",
					Name:  "e2e-apikey-route",
				},
				CredentialRef: &mcpv1alpha1.SecretReference{
					Name: "e2e-apikey-credentials",
					Key:  "token",
				},
			},
		}
		_ = k8sClient.Delete(ctx, mcpServerApiKey)
		time.Sleep(2 * time.Second)
		Expect(k8sClient.Create(ctx, mcpServerApiKey)).To(Succeed())
		defer CleanupResource(ctx, k8sClient, mcpServerApiKey)

		// wait for aggregated secret to be created with the credential
		By("Waiting for aggregated credentials secret to contain the credential")
		Eventually(func() bool {
			aggregatedSecret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "mcp-aggregated-credentials",
				Namespace: SystemNamespace,
			}, aggregatedSecret)
			if err != nil {
				return false
			}
			// check if the expected credential env var exists and has value
			envVarName := "KAGENTAI_E2E_APIKEY_SERVER_CRED"
			if val, exists := aggregatedSecret.Data[envVarName]; exists {
				return string(val) == "Bearer test-api-key-secret-token"
			}
			return false
		}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue(),
			"Aggregated secret should contain the credential")

		By("Verifying MCPServer becomes ready with valid credentials")
		// For servers with credentials, we need to wait longer due to volume mount sync
		mcpServer := &mcpv1alpha1.MCPServer{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      mcpServerApiKey.Name,
				Namespace: mcpServerApiKey.Namespace,
			}, mcpServer)
			if err != nil {
				return false
			}

			for _, condition := range mcpServer.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
					return true
				}
			}
			return false
		}, TestTimeoutConfigSync, TestRetryInterval).Should(BeTrue(),
			"MCPServer should become ready with valid credentials")

		By("Setting up port-forward to broker for status check")
		statusPortForwardCmd := setupPortForward("mcp-broker-router", SystemNamespace, "18083:8080")
		defer statusPortForwardCmd.Process.Kill()

		// wait for port-forward to be ready
		Eventually(func() error {
			client := &http.Client{Timeout: 1 * time.Second}
			resp, err := client.Get("http://localhost:18083/status")
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		By("Verifying server is registered with valid credentials")
		// Initial registration may need to wait for volume mount sync
		// Periodically trigger config reloads to prompt registration once credentials are available
		Eventually(func() bool {
			// trigger config reload to prompt re-registration
			mcpServerPatch := client.MergeFrom(mcpServerApiKey.DeepCopy())
			if mcpServerApiKey.Annotations == nil {
				mcpServerApiKey.Annotations = make(map[string]string)
			}
			mcpServerApiKey.Annotations["reconcile-initial"] = fmt.Sprintf("%d", time.Now().Unix())
			_ = k8sClient.Patch(ctx, mcpServerApiKey, mcpServerPatch)

			// wait a moment for config push
			time.Sleep(2 * time.Second)

			// check if server is registered and reachable
			// Look for the actual service name: mcp-api-key-server
			reachable, err := verifyServerInBrokerStatus("http://localhost:18083/status", "mcp-api-key-server", true)
			return err == nil && reachable
		}, TestTimeoutConfigSync, 10*time.Second).Should(BeTrue(),
			"Server should be registered and reachable with valid credentials after volume mount sync")

		By("Updating credential to invalid value")
		// patch secret with invalid token
		patch := client.MergeFrom(credentialSecret.DeepCopy())
		credentialSecret.StringData = map[string]string{
			"token": "Bearer invalid-token",
		}
		Expect(k8sClient.Patch(ctx, credentialSecret, patch)).To(Succeed())

		By("Waiting for volume mount to sync credential change")
		// Volume mounts can take 60-120s to sync in Kubernetes
		// We'll wait and periodically trigger config reloads until the broker detects the change
		Eventually(func() bool {
			// trigger config reload by annotating mcpserver
			mcpServerPatch := client.MergeFrom(mcpServerApiKey.DeepCopy())
			if mcpServerApiKey.Annotations == nil {
				mcpServerApiKey.Annotations = make(map[string]string)
			}
			mcpServerApiKey.Annotations["reconcile"] = fmt.Sprintf("%d", time.Now().Unix())
			if err := k8sClient.Patch(ctx, mcpServerApiKey, mcpServerPatch); err != nil {
				return false
			}

			// wait a moment for the config push to process
			time.Sleep(2 * time.Second)

			// check if server became unreachable (indicating credential change was detected)
			reachable, err := verifyServerInBrokerStatus("http://localhost:18083/status", "mcp-api-key-server", false)
			if err != nil {
				// server might be completely removed from status
				return true
			}
			return !reachable
		}, TestTimeoutConfigSync, 10*time.Second).Should(BeTrue(),
			"Broker should detect credential change after volume mount syncs")

		By("Verifying server becomes unreachable with invalid credentials")
		Eventually(func() bool {
			reachable, err := verifyServerInBrokerStatus("http://localhost:18083/status", "mcp-api-key-server", false)
			if err != nil {
				// server might be completely removed from status
				return true
			}
			return !reachable // we expect it to be unreachable
		}, TestTimeoutLong, TestRetryInterval).Should(BeTrue(),
			"Server should be unreachable or removed with invalid credentials")

		By("Updating credential back to valid value")
		patch = client.MergeFrom(credentialSecret.DeepCopy())
		credentialSecret.StringData = map[string]string{
			"token": "Bearer test-api-key-secret-token",
		}
		Expect(k8sClient.Patch(ctx, credentialSecret, patch)).To(Succeed())

		By("Waiting for volume mount to sync valid credential and broker to re-register")
		// Periodically trigger config reloads until the broker detects the valid credential
		Eventually(func() bool {
			// trigger config reload
			mcpServerPatch := client.MergeFrom(mcpServerApiKey.DeepCopy())
			if mcpServerApiKey.Annotations == nil {
				mcpServerApiKey.Annotations = make(map[string]string)
			}
			mcpServerApiKey.Annotations["reconcile"] = fmt.Sprintf("%d", time.Now().Unix())
			if err := k8sClient.Patch(ctx, mcpServerApiKey, mcpServerPatch); err != nil {
				return false
			}

			// wait a moment for the config push to process
			time.Sleep(2 * time.Second)

			// check if server became reachable again
			reachable, err := verifyServerInBrokerStatus("http://localhost:18083/status", "mcp-api-key-server", true)
			return err == nil && reachable
		}, TestTimeoutConfigSync, 10*time.Second).Should(BeTrue(),
			"Server should be reachable again with valid credentials after volume mount syncs")

		By("Test completed successfully")
	})
})

// helper to check if server exists in broker status and its reachability
func verifyServerInBrokerStatus(statusURL, serverNamePart string, expectReachable bool) (bool, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(statusURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status endpoint returned %d", resp.StatusCode)
	}

	type StatusResponse struct {
		Servers []struct {
			URL              string `json:"url"`
			Name             string `json:"name"`
			ConnectionStatus struct {
				IsReachable bool `json:"isReachable"`
			} `json:"connectionStatus"`
		} `json:"servers"`
	}

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, err
	}

	for _, server := range status.Servers {
		if strings.Contains(server.URL, serverNamePart) {
			return server.ConnectionStatus.IsReachable == expectReachable, nil
		}
	}

	// server not found in status
	return false, fmt.Errorf("server %s not found in status", serverNamePart)
}

// helper function to setup port-forward
func setupPortForward(resource, namespace, ports string) *exec.Cmd {
	cmd := exec.Command("kubectl", "port-forward", "-n", namespace,
		fmt.Sprintf("deployment/%s", resource), ports)
	err := cmd.Start()
	Expect(err).ToNot(HaveOccurred())
	return cmd
}

// helper function for making MCP requests
func makeMCPRequest(url string, payload map[string]interface{}) map[string]interface{} {
	jsonData, err := json.Marshal(payload)
	Expect(err).ToNot(HaveOccurred())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	Expect(err).ToNot(HaveOccurred())

	return result
}

// make mcp request and get session id
func makeMCPRequestWithSession(url string, payload map[string]interface{}) (map[string]interface{}, string) {
	jsonData, err := json.Marshal(payload)
	Expect(err).ToNot(HaveOccurred())

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	Expect(err).ToNot(HaveOccurred())

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// get session id from response header
	sessionID := resp.Header.Get("Mcp-Session-Id")

	body, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	Expect(err).ToNot(HaveOccurred())

	return result, sessionID
}

// make mcp request with existing session
func makeMCPRequestWithSessionID(url string, payload map[string]interface{}, sessionID string) map[string]interface{} {
	jsonData, err := json.Marshal(payload)
	Expect(err).ToNot(HaveOccurred())

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	Expect(err).ToNot(HaveOccurred())

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	Expect(err).ToNot(HaveOccurred())

	return result
}
