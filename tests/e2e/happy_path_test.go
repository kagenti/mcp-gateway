//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

	It("should aggregate MCP servers and manage HTTPRoute conditions", func() {
		By("Creating HTTPRoutes")
		Expect(k8sClient.Create(ctx, httpRoute1)).To(Succeed())
		Expect(k8sClient.Create(ctx, httpRoute2)).To(Succeed())

		By("Creating MCPServer resources")
		Expect(k8sClient.Create(ctx, mcpServer1)).To(Succeed())
		Expect(k8sClient.Create(ctx, mcpServer2)).To(Succeed())

		By("Verifying MCPServers become ready")
		VerifyMCPServerReady(ctx, k8sClient, mcpServer1.Name, mcpServer1.Namespace)
		VerifyMCPServerReady(ctx, k8sClient, mcpServer2.Name, mcpServer2.Namespace)

		By("Verifying ConfigMap is created with correct content")
		VerifyConfigMapExists(ctx, k8sClient)

		// wait for controller to aggregate both servers into configmap
		configMap := &corev1.ConfigMap{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ConfigMapName,
				Namespace: SystemNamespace,
			}, configMap)
			if err != nil {
				return false
			}
			configContent := configMap.Data["config.yaml"]
			// check both servers are present
			hasS1 := strings.Contains(configContent, "s1_")
			hasS2 := strings.Contains(configContent, "s2_")
			if !hasS1 || !hasS2 {
				fmt.Printf("ConfigMap not ready yet - has s1_: %v, has s2_: %v\n", hasS1, hasS2)
			}
			return hasS1 && hasS2
		}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue(), "Both MCPServers should be in ConfigMap")

		// parse and verify configmap content
		configContent := configMap.Data["config.yaml"]
		fmt.Printf("ConfigMap content:\n%s\n", configContent)
		Expect(configContent).To(ContainSubstring("s1_"))
		Expect(configContent).To(ContainSubstring("s2_"))
		Expect(configContent).To(ContainSubstring("mcp-test-server1.mcp-test.svc.cluster.local"))
		Expect(configContent).To(ContainSubstring("mcp-test-server2.mcp-test.svc.cluster.local"))

		By("Waiting for broker to register servers")
		// wait for broker to load the config and be ready
		// we'll verify by checking if the broker pod has reloaded config
		Eventually(func() bool {
			// check broker logs for config reload message
			cmd := exec.Command("kubectl", "logs", "-n", SystemNamespace,
				"deployment/mcp-broker-router", "--tail=20")
			output, err := cmd.Output()
			if err != nil {
				return false
			}
			// look for evidence that config was loaded with our servers
			return bytes.Contains(output, []byte("s1_")) &&
				bytes.Contains(output, []byte("s2_"))
		}, 90*time.Second, 5*time.Second).Should(BeTrue(),
			"Broker should load configuration with test servers")

		By("Setting up port-forward to broker")
		brokerURL := "http://localhost:18080/mcp"
		portForwardCmd := setupPortForward("mcp-broker", SystemNamespace, "18080:8080")
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
		fmt.Printf("Found %d tools:\n", len(toolsList))
		for _, tool := range toolsList {
			toolMap := tool.(map[string]interface{})
			name := toolMap["name"].(string)
			fmt.Printf("  - %s\n", name)
			if len(name) >= 3 && name[:3] == "s1_" {
				foundS1Tool = true
			}
			if len(name) >= 3 && name[:3] == "s2_" {
				foundS2Tool = true
			}
		}
		Expect(foundS1Tool).To(BeTrue(), "Should find tools with s1_ prefix")
		Expect(foundS2Tool).To(BeTrue(), "Should find tools with s2_ prefix")

		By("Deleting one MCPServer")
		Expect(k8sClient.Delete(ctx, mcpServer1)).To(Succeed())

		By("Verifying ConfigMap is updated")
		Eventually(func() bool {
			configMap := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      ConfigMapName,
				Namespace: SystemNamespace,
			}, configMap)
			if err != nil {
				return false
			}
			// should still have s2_ but not s1_
			configContent := configMap.Data["config.yaml"]
			return !bytes.Contains([]byte(configContent), []byte("s1_")) &&
				bytes.Contains([]byte(configContent), []byte("s2_"))
		}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue())

		By("Test completed successfully")
	})
})

// helper function to setup port-forward
func setupPortForward(service, namespace, ports string) *exec.Cmd {
	cmd := exec.Command("kubectl", "port-forward", "-n", namespace,
		fmt.Sprintf("service/%s", service), ports)
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

	// debug output to see what we got
	fmt.Printf("Response status: %d\n", resp.StatusCode)
	fmt.Printf("Response body: %s\n", string(body))

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
