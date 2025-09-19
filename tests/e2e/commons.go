//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
)

const (
	TestTimeoutMedium     = time.Second * 30
	TestTimeoutLong       = time.Minute * 2
	TestTimeoutConfigSync = time.Minute * 3 // configmap volume mount sync can take up to 2 minutes
	TestRetryInterval     = time.Second * 2

	TestNamespace   = "mcp-test"
	SystemNamespace = "mcp-system"
	ConfigMapName   = "mcp-gateway-config"
)

// TestMCPServer creates a test MCPServer resource
func BuildTestMCPServer(name, namespace string, targetHTTPRoute string, prefix string) *mcpv1alpha1.MCPServer {
	return &mcpv1alpha1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: mcpv1alpha1.MCPServerSpec{
			ToolPrefix: prefix,
			TargetRef: mcpv1alpha1.TargetReference{
				Group: "gateway.networking.k8s.io",
				Kind:  "HTTPRoute",
				Name:  targetHTTPRoute,
			},
		},
	}
}

// BuildTestHTTPRoute creates a test HTTPRoute
func BuildTestHTTPRoute(name, namespace, hostname, serviceName string, port int32) *gatewayapiv1.HTTPRoute {
	gatewayNamespace := "gateway-system"
	return &gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayapiv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
				ParentRefs: []gatewayapiv1.ParentReference{
					{
						Name:      "mcp-gateway",
						Namespace: (*gatewayapiv1.Namespace)(&gatewayNamespace),
					},
				},
			},
			Hostnames: []gatewayapiv1.Hostname{
				gatewayapiv1.Hostname(hostname),
				// add second hostname to match real deployments
				gatewayapiv1.Hostname(strings.Replace(hostname, ".mcp.example.com", ".127-0-0-1.sslip.io", 1)),
			},
			Rules: []gatewayapiv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayapiv1.HTTPBackendRef{
						{
							BackendRef: gatewayapiv1.BackendRef{
								BackendObjectReference: gatewayapiv1.BackendObjectReference{
									Name: gatewayapiv1.ObjectName(serviceName),
									Port: (*gatewayapiv1.PortNumber)(&port),
								},
							},
						},
					},
				},
			},
		},
	}
}

// VerifyConfigMapExists checks if the ConfigMap exists and has content
func VerifyConfigMapExists(ctx context.Context, k8sClient client.Client) {
	configMap := &corev1.ConfigMap{}
	Eventually(func() error {
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      ConfigMapName,
			Namespace: SystemNamespace,
		}, configMap)
	}, TestTimeoutMedium, TestRetryInterval).Should(Succeed())

	Expect(configMap.Data).To(HaveKey("config.yaml"))
	Expect(configMap.Data["config.yaml"]).ToNot(BeEmpty())
}

// VerifyMCPServerReady checks if the MCPServer has Ready condition
func VerifyMCPServerReady(ctx context.Context, k8sClient client.Client, name, namespace string) {
	mcpServer := &mcpv1alpha1.MCPServer{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
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
	}, TestTimeoutMedium, TestRetryInterval).Should(BeTrue())
}

// CleanupResource deletes a resource and waits for it to be gone
func CleanupResource(ctx context.Context, k8sClient client.Client, obj client.Object) {
	err := k8sClient.Delete(ctx, obj)
	if err != nil {
		// ignore not found errors
		if client.IgnoreNotFound(err) != nil {
			Expect(err).ToNot(HaveOccurred())
		}
	}

	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		return client.IgnoreNotFound(err) != nil
	}, TestTimeoutMedium, TestRetryInterval).Should(BeFalse())
}

// DumpComponentLogs dumps logs for mcp-gateway components on test failure
func DumpComponentLogs() {
	GinkgoWriter.Println("=== Dumping Component Logs ===")

	// dump controller logs
	GinkgoWriter.Println("\n--- Controller Logs ---")
	cmd := exec.Command("kubectl", "logs", "-n", SystemNamespace,
		"deployment/mcp-controller", "--tail=50")
	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("Failed to get controller logs: %v\n", err)
	} else {
		GinkgoWriter.Printf("%s\n", output)
	}

	// dump broker/router logs
	GinkgoWriter.Println("\n--- Broker/Router Logs ---")
	cmd = exec.Command("kubectl", "logs", "-n", SystemNamespace,
		"deployment/mcp-broker-router", "--tail=100")
	output, err = cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("Failed to get broker/router logs: %v\n", err)
	} else {
		GinkgoWriter.Printf("%s\n", output)
	}

	// dump configmap content
	GinkgoWriter.Println("\n--- ConfigMap Content ---")
	cmd = exec.Command("kubectl", "get", "configmap", "-n", SystemNamespace,
		ConfigMapName, "-o", "yaml")
	output, err = cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("Failed to get configmap: %v\n", err)
	} else {
		GinkgoWriter.Printf("%s\n", output)
	}

	// dump pod status
	GinkgoWriter.Println("\n--- Pod Status ---")
	cmd = exec.Command("kubectl", "get", "pods", "-n", SystemNamespace, "-o", "wide")
	output, err = cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("Failed to get pod status: %v\n", err)
	} else {
		GinkgoWriter.Printf("%s\n", output)
	}

	// dump events
	GinkgoWriter.Println("\n--- Recent Events ---")
	cmd = exec.Command("kubectl", "get", "events", "-n", SystemNamespace,
		"--sort-by=lastTimestamp", "--tail=20")
	output, err = cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("Failed to get events: %v\n", err)
	} else {
		GinkgoWriter.Printf("%s\n", output)
	}

	GinkgoWriter.Println("=== End Component Logs ===")
}

// DumpTestServerLogs dumps logs for test MCP servers
func DumpTestServerLogs() {
	GinkgoWriter.Println("\n=== Test Server Logs ===")

	servers := []string{"mcp-test-server1", "mcp-test-server2", "mcp-test-server3"}
	for _, server := range servers {
		GinkgoWriter.Printf("\n--- %s Logs ---\n", server)
		cmd := exec.Command("kubectl", "logs", "-n", TestNamespace,
			fmt.Sprintf("deployment/%s", server), "--tail=30")
		output, err := cmd.CombinedOutput()
		if err != nil {
			GinkgoWriter.Printf("Failed to get %s logs: %v\n", server, err)
		} else {
			GinkgoWriter.Printf("%s\n", output)
		}
	}

	GinkgoWriter.Println("=== End Test Server Logs ===")
}
