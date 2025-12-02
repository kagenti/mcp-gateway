//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
)

const (
	TestTimeoutMedium     = time.Second * 30
	TestTimeoutLong       = time.Minute * 2
	TestTimeoutConfigSync = time.Minute * 4
	TestRetryInterval     = time.Second * 5

	TestNamespace   = "mcp-test"
	SystemNamespace = "mcp-system"
	ConfigMapName   = "mcp-gateway-config"
)

// MCPServerBuilder builds MCPServer resources
type MCPServerBuilder struct {
	name            string
	namespace       string
	targetHTTPRoute string
	prefix          string
	secret          *corev1.Secret
	path            string
	credentialKey   string
}

// NewMCPServerBuilder creates a new MCPServerBuilder
func NewMCPServerBuilder(name, namespace string) *MCPServerBuilder {
	return &MCPServerBuilder{
		name:          name,
		namespace:     namespace,
		credentialKey: "token",
	}
}

// WithTargetHTTPRoute sets the target HTTPRoute
func (b *MCPServerBuilder) WithTargetHTTPRoute(route string) *MCPServerBuilder {
	b.targetHTTPRoute = route
	return b
}

// WithToolPrefix sets the tool prefix
func (b *MCPServerBuilder) WithToolPrefix(prefix string) *MCPServerBuilder {
	b.prefix = prefix
	return b
}

// WithSecret sets the credential secret
func (b *MCPServerBuilder) WithSecret(secret *corev1.Secret) *MCPServerBuilder {
	b.secret = secret
	return b
}

// WithPath sets the custom MCP path
func (b *MCPServerBuilder) WithPath(path string) *MCPServerBuilder {
	b.path = path
	return b
}

// WithCredentialKey sets the secret key for credentials
func (b *MCPServerBuilder) WithCredentialKey(key string) *MCPServerBuilder {
	b.credentialKey = key
	return b
}

// Build creates the MCPServer resource
func (b *MCPServerBuilder) Build() *mcpv1alpha1.MCPServer {
	mcpServ := &mcpv1alpha1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      UniqueName(b.name),
			Namespace: b.namespace,
		},
		Spec: mcpv1alpha1.MCPServerSpec{
			ToolPrefix: b.prefix,
			TargetRef: mcpv1alpha1.TargetReference{
				Group: "gateway.networking.k8s.io",
				Kind:  "HTTPRoute",
				Name:  b.targetHTTPRoute,
			},
		},
	}
	if b.path != "" {
		mcpServ.Spec.Path = b.path
	}
	if b.secret != nil {
		mcpServ.Spec.CredentialRef = &mcpv1alpha1.SecretReference{
			Name: b.secret.Name,
			Key:  b.credentialKey,
		}
	}
	return mcpServ
}

// BuildTestMCPServer creates a test MCPServer resource (legacy function for backwards compatibility)
func BuildTestMCPServer(name, namespace string, targetHTTPRoute string, prefix string) *MCPServerBuilder {
	return NewMCPServerBuilder(name, namespace).
		WithTargetHTTPRoute(targetHTTPRoute).
		WithToolPrefix(prefix)

}

// MCPVirtualServerBuilder builds MCPVirtualServer resources
type MCPVirtualServerBuilder struct {
	name        string
	namespace   string
	description string
	tools       []string
}

// NewMCPVirtualServerBuilder creates a new MCPVirtualServerBuilder
func NewMCPVirtualServerBuilder(name, namespace string) *MCPVirtualServerBuilder {
	return &MCPVirtualServerBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithDescription sets the description
func (b *MCPVirtualServerBuilder) WithDescription(desc string) *MCPVirtualServerBuilder {
	b.description = desc
	return b
}

// WithTools sets the tools list
func (b *MCPVirtualServerBuilder) WithTools(tools []string) *MCPVirtualServerBuilder {
	b.tools = tools
	return b
}

// Build creates the MCPVirtualServer resource
func (b *MCPVirtualServerBuilder) Build() *mcpv1alpha1.MCPVirtualServer {
	return &mcpv1alpha1.MCPVirtualServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      UniqueName(b.name),
			Namespace: b.namespace,
		},
		Spec: mcpv1alpha1.MCPVirtualServerSpec{
			Description: b.description,
			Tools:       b.tools,
		},
	}
}

// BuildTestMCPVirtualServer creates a test MCPVirtualServer resource
func BuildTestMCPVirtualServer(name, namespace string, tools []string) *MCPVirtualServerBuilder {
	return NewMCPVirtualServerBuilder(name, namespace).
		WithTools(tools)
}

// BuildTestHTTPRoute creates a test HTTPRoute
func BuildTestHTTPRoute(name, namespace, hostname, serviceName string, port int32) *gatewayapiv1.HTTPRoute {
	gatewayNamespace := "gateway-system"
	return &gatewayapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      UniqueName(name),
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

// BuildCredentialSecret creates a credential secret for testing
func BuildCredentialSecret(name, token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestNamespace,
			Labels: map[string]string{
				"mcp.kagenti.com/credential": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token": fmt.Sprintf("Bearer %s", token), // valid token
		},
	}
}

// MCPServerName returns the full name of an MCP server from its HTTPRoute
func MCPServerName(route *gatewayapiv1.HTTPRoute) string {
	return fmt.Sprintf("%s/%s", route.Namespace, route.Name)
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

// VerifyMCPServerReady checks if the MCPServer has Ready condition. Once ready it should be able to be invoked
func VerifyMCPServerReady(ctx context.Context, k8sClient client.Client, name, namespace string) error {
	mcpServer := &mcpv1alpha1.MCPServer{}

	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, mcpServer)

	if err != nil {
		return fmt.Errorf("failed to verify mcp server %s ready %w", mcpServer.Name, err)
	}

	for _, condition := range mcpServer.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			return nil
		}
	}
	return fmt.Errorf("mcpserver %s not ready ", mcpServer.Name)

}

// MCPServerRegistrationBuilder builds and registers MCP server resources
type MCPServerRegistrationBuilder struct {
	ctx        context.Context
	k8sClient  client.Client
	credential *corev1.Secret
	httpRoute  *gatewayapiv1.HTTPRoute
	mcpServer  *MCPServerBuilder
}

// NewMCPServerRegistration creates a new registration builder with defaults
func NewMCPServerRegistration(ctx context.Context, k8sClient client.Client) *MCPServerRegistrationBuilder {
	httpRoute := BuildTestHTTPRoute("e2e-server2-route", TestNamespace,
		"e2e-server2.mcp.local", "mcp-test-server2", 9090)
	mcpServer := BuildTestMCPServer("e2e-mcpserver2", TestNamespace,
		httpRoute.Name, httpRoute.Name)

	return &MCPServerRegistrationBuilder{
		ctx:       ctx,
		k8sClient: k8sClient,
		httpRoute: httpRoute,
		mcpServer: mcpServer,
	}
}

// WithBackendTarget sets the backend service and port for the HTTPRoute
func (b *MCPServerRegistrationBuilder) WithBackendTarget(backend string, port int32) *MCPServerRegistrationBuilder {
	if b.httpRoute != nil {
		p := gatewayapiv1.PortNumber(port)
		b.httpRoute.Spec.Rules[0].BackendRefs[0].BackendObjectReference = gatewayapiv1.BackendObjectReference{
			Name: gatewayapiv1.ObjectName(backend),
			Port: &p,
		}
	}
	return b
}

// WithCredential overrides the default credential secret
func (b *MCPServerRegistrationBuilder) WithCredential(secret *corev1.Secret) *MCPServerRegistrationBuilder {
	b.credential = secret
	b.mcpServer.WithSecret(secret)
	return b
}

// WithHTTPRoute overrides the default HTTPRoute
func (b *MCPServerRegistrationBuilder) WithHTTPRoute(route *gatewayapiv1.HTTPRoute) *MCPServerRegistrationBuilder {
	b.httpRoute = route
	b.mcpServer.WithTargetHTTPRoute(route.Name)
	return b
}

// WithMCPServer overrides the default MCPServer builder
func (b *MCPServerRegistrationBuilder) WithMCPServer(builder *MCPServerBuilder) *MCPServerRegistrationBuilder {
	b.mcpServer = builder
	return b
}

// Register creates all resources and returns them
func (b *MCPServerRegistrationBuilder) Register() *v1alpha1.MCPServer {

	if b.credential != nil {
		Expect(b.k8sClient.Create(b.ctx, b.credential)).To(Succeed())
	}
	Expect(b.k8sClient.Create(b.ctx, b.httpRoute)).To(Succeed())

	mcpServer := b.mcpServer.Build()
	Expect(b.k8sClient.Create(b.ctx, mcpServer)).To(Succeed())

	return mcpServer
}

// GetObjects returns all objects defined in the builder
func (b *MCPServerRegistrationBuilder) GetObjects() []client.Object {
	objects := []client.Object{}
	if b.credential != nil {
		objects = append(objects, b.credential)
	}
	if b.httpRoute != nil {
		objects = append(objects, b.httpRoute)
	}
	if b.mcpServer != nil {
		objects = append(objects, b.mcpServer.Build())
	}
	return objects
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

// UniqueName generates a unique name with the given prefix.
func UniqueName(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// NewMCPGatewayClient creates a new MCP client connected to the gateway
func NewMCPGatewayClient(ctx context.Context, gatewayHost string) (*mcpclient.Client, error) {
	mcpGatewayClient, err := mcpclient.NewStreamableHttpClient(gatewayHost, transport.
		WithHTTPHeaders(map[string]string{"e2e": "client"}))
	if err != nil {
		return nil, err
	}
	err = mcpGatewayClient.Start(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := mcpGatewayClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "e2e",
				Version: "0.0.1",
			},
		},
	}); err != nil {
		return nil, err
	}
	return mcpGatewayClient, nil

}
