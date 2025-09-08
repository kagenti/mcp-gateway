//go:build e2e

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
)

const (
	TestTimeoutMedium = time.Second * 30
	TestTimeoutLong   = time.Minute * 2
	TestRetryInterval = time.Second * 2

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
