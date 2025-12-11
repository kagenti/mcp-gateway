//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var gatewayURL = "http://localhost:8001/mcp"
var _ = Describe("MCP Gateway Registration Happy Path", func() {
	var (
		testResources = []client.Object{}
	)

	BeforeEach(func() {
	})

	AfterEach(func() {
		// cleanup in reverse order
		for _, to := range testResources {
			CleanupResource(ctx, k8sClient, to)
		}
	})

	JustAfterEach(func() {
		// dump logs if test failed
		if CurrentSpecReport().Failed() {
			//	DumpComponentLogs()
			//	DumpTestServerLogs()
		}
	})

	It("should register multiple mcp servers with the gateway and make their tools available", func() {
		By("Creating HTTPRoutes and MCP Servers")
		// create httproutes for test servers that should already be deployed
		registration := NewMCPServerRegistration("basic-registration", k8sClient)
		// Important as we need to make sure to clean up
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer1 := registration.Register(ctx)
		registration = NewMCPServerRegistration("basic-registration", k8sClient)
		// Important as we need to make sure to clean up
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer2 := registration.Register(ctx)

		By("Verifying MCPServers become ready")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer1.Name, registeredServer1.Namespace)).To(BeNil())
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer2.Name, registeredServer2.Namespace)).To(BeNil())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Verifying MCPServers tools are present")
		Eventually(func(g Gomega) {
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer1.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer1.Spec.ToolPrefix))
			g.Expect(verifyMCPServerToolsPresent(registeredServer2.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer2.Spec.ToolPrefix))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

	})

	It("should unregister mcp servers with the gateway", func() {
		registration := NewMCPServerRegistration("basic-unregister", k8sClient)
		// Important as we need to make sure to clean up
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer := registration.Register(ctx)

		By("Ensuring the gateway has registered the server")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).To(BeNil())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("ensuring the tools are present in the gateway")
		Eventually(func(g Gomega) {
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("unregistering an MCPServer by Deleting the resource")
		Expect(k8sClient.Delete(ctx, registeredServer)).To(Succeed())

		By("Verifying broker removes the deleted server")
		// do tools call check tools no longer present
		Eventually(func(g Gomega) {
			err := VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)
			g.Expect(err).NotTo(BeNil())
			g.Expect(err.Error()).Should(ContainSubstring("not found"))
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeFalse())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())
	})

	It("should invoke tools successfully", func() {
		registration := NewMCPServerRegistration("tools-invoke", k8sClient)
		// Important as we need to make sure to clean up
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer := registration.Register(ctx)

		By("Ensuring the gateway has registered the server")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).To(BeNil())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Verifying MCPServers tools are present")
		Eventually(func(g Gomega) {
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		toolName := fmt.Sprintf("%s%s", registeredServer.Spec.ToolPrefix, "hello_world")
		GinkgoWriter.Println("tool", toolName)
		By("Invoking a tool")
		res, err := mcpGatewayClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: toolName, Arguments: map[string]string{
				"name": "e2e",
			}},
		})
		Expect(err).Error().NotTo(HaveOccurred())
		Expect(res).NotTo(BeNil())
		Expect(len(res.Content)).To(BeNumerically("==", 1))
		content, ok := res.Content[0].(mcp.TextContent)
		Expect(ok).To(BeTrue())
		Expect(content.Text).To(Equal("Hello, e2e!"))
	})

	It("should register mcp server with credetential with the gateway and make the tools available", func() {
		cred := BuildCredentialSecret("mcp-credential", "test-api-key-secret-toke")
		registration := NewMCPServerRegistration("credentials", k8sClient).
			WithCredential(cred, "token").WithBackendTarget("mcp-api-key-server", 9090)
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer := registration.Register(ctx)
		By("ensuring broker has failed authentication and the mcp server is not registered and the tools dont exist")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).Error().To(Not(BeNil()))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Verifying MCPServers tools are not present")
		Eventually(func(g Gomega) {
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeFalseBecause("%s should NOT exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("updating the secret to a valid value the server should be registered and the tools should exist")
		patch := client.MergeFrom(cred.DeepCopy())
		cred.StringData = map[string]string{
			"token": "Bearer test-api-key-secret-token",
		}
		Expect(k8sClient.Patch(ctx, cred, patch)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).Error().To(BeNil())
		}, TestTimeoutConfigSync, TestRetryInterval).To(Succeed())

		By("Verifying MCPServers tools are present")
		Eventually(func(g Gomega) {
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

	})

	It("should use and re-use a backend MCP session", func() {

		registration := NewMCPServerRegistration("sessions", k8sClient)
		// Important as we need to make sure to clean up
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer := registration.Register(ctx)

		By("creating a new client")
		mcpClient, err := NewMCPGatewayClient(context.Background(), gatewayURL)
		Expect(err).Error().NotTo(HaveOccurred())
		clientSession := mcpClient.GetSessionId()
		By("Ensuring the gateway has registered the server")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).To(BeNil())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())
		By("Ensuring the gateway has the tools")
		Eventually(func(g Gomega) {
			toolsList, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutMedium, TestRetryInterval).To(Succeed())

		toolName := fmt.Sprintf("%s%s", registeredServer.Spec.ToolPrefix, "headers")
		By("Invoking a tool")
		res, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: toolName},
		})
		Expect(err).Error().NotTo(HaveOccurred())
		Expect(res).NotTo(BeNil())
		mcpsessionid := ""
		for _, cont := range res.Content {
			textContent, ok := cont.(mcp.TextContent)
			Expect(ok).To(BeTrue())
			if strings.HasPrefix(textContent.Text, "Mcp-Session-Id") {
				GinkgoWriter.Println(textContent.Text)
				mcpsessionid = textContent.Text
			}
		}
		Expect(mcpsessionid).To(ContainSubstring("Mcp-Session-Id"))

		By("Invoking the headers tool again")
		res, err = mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: toolName},
		})
		Expect(err).Error().NotTo(HaveOccurred())
		Expect(res).NotTo(BeNil())
		for _, cont := range res.Content {
			textContent, ok := cont.(mcp.TextContent)
			Expect(ok).To(BeTrue())
			if strings.HasPrefix(textContent.Text, "Mcp-Session-Id") {
				Expect(textContent.Text).To(ContainSubstring("Mcp-Session-Id"))
				Expect(mcpsessionid).To(Equal(textContent.Text))
				// the session for the gateway should not be the same as the session for the MCP server
				Expect(mcpsessionid).NotTo(ContainSubstring(clientSession))
			}
		}

		By("deleting the session it should get a new backend session")
		Expect(mcpClient.Close()).Error().NotTo(HaveOccurred())
		// closing the client triggers a delete and cancelling of the context so we need a new client
		mcpClient, err = NewMCPGatewayClient(context.Background(), gatewayURL)
		Expect(err).Error().NotTo(HaveOccurred())
		By("invoking headers tool with new client")
		res, err = mcpClient.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: toolName},
		})
		Expect(err).Error().NotTo(HaveOccurred())
		Expect(res).NotTo(BeNil())
		for _, cont := range res.Content {
			textContent, ok := cont.(mcp.TextContent)
			Expect(ok).To(BeTrue())
			if strings.HasPrefix(textContent.Text, "Mcp-Session-Id") {
				GinkgoWriter.Println(textContent.Text)
				Expect(textContent.Text).To(ContainSubstring("Mcp-Session-Id"))
				Expect(mcpsessionid).To(Not(Equal(textContent.Text)))
				Expect(textContent.Text).To(Not(ContainSubstring(mcpClient.GetSessionId())))
			}
		}
	})

	It("should deploy redis and scale up the broker and see sessions shared", func() {
		Skip("not implemented")
	})

	It("should only return tools specified by MCPVirtualServer when using X-Mcp-Virtualserver header", func() {
		By("Creating an MCPServer with tools")
		registration := NewMCPServerRegistration("virtualserver-test", k8sClient)
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer := registration.Register(ctx)

		By("Ensuring the gateway has registered the server")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).To(BeNil())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Verifying MCPServer tools are present")
		Eventually(func(g Gomega) {
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Creating an MCPVirtualServer with a subset of tools")
		allowedTool := fmt.Sprintf("%s%s", registeredServer.Spec.ToolPrefix, "hello_world")
		virtualServer := BuildTestMCPVirtualServer("test-virtualserver", TestNamespace, []string{allowedTool}).Build()
		testResources = append(testResources, virtualServer)
		Expect(k8sClient.Create(ctx, virtualServer)).To(Succeed())

		By("Creating a client with X-Mcp-Virtualserver header")
		virtualServerHeader := fmt.Sprintf("%s/%s", virtualServer.Namespace, virtualServer.Name)
		virtualServerClient, err := NewMCPGatewayClientWithHeaders(ctx, gatewayURL, map[string]string{
			"X-Mcp-Virtualserver": virtualServerHeader,
		})
		Expect(err).NotTo(HaveOccurred())
		defer virtualServerClient.Close()

		By("Verifying only the tools from MCPVirtualServer are returned")
		Eventually(func(g Gomega) {
			filteredTools, err := virtualServerClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(filteredTools).NotTo(BeNil())
			g.Expect(len(filteredTools.Tools)).To(Equal(1), "expected exactly 1 tool from virtual server")
			g.Expect(filteredTools.Tools[0].Name).To(Equal(allowedTool))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Verifying the original client without header still sees all tools")
		allToolsAgain, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(allToolsAgain.Tools)).To(BeNumerically(">", 1), "expected more than 1 tool without virtual server header")
	})

	It("should send notifications/tools/list_changed to connected clients when MCPServer is registered", func() {

		By("Creating clients with notification handlers and different sessions")
		client1Notification := false
		client1, err := NewMCPGatewayClientWithNotifications(ctx, gatewayURL, func(j mcp.JSONRPCNotification) {
			client1Notification = true
		})
		Expect(err).NotTo(HaveOccurred())
		defer client1.Close()

		client2Notification := false
		client2, err := NewMCPGatewayClientWithNotifications(ctx, gatewayURL, func(j mcp.JSONRPCNotification) {
			client2Notification = true
		})
		Expect(err).NotTo(HaveOccurred())
		defer client2.Close()
		Expect(mcpGatewayClient.sessionID).NotTo(BeEmpty())
		Expect(client2.sessionID).NotTo(BeEmpty())
		Expect(client1.sessionID).NotTo(Equal(client2.sessionID))

		By("registering a new MCPServer")
		registration := NewMCPServerRegistration("notification-test", k8sClient)
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer := registration.Register(ctx)

		By("Waiting for the server to become ready")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).To(BeNil())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		// We do this to wait for the tools to show up as we know then that the gateway has done its work
		Eventually(func(g Gomega) {
			toolsList, err := mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(toolsList).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, toolsList)).To(BeTrueBecause("%s should exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutMedium, TestRetryInterval).To(Succeed())

		By("Verifying both clients received notifications/tools/list_changed within 1 minutes")
		Eventually(func(g Gomega) {
			_, err := client1.ListTools(ctx, mcp.ListToolsRequest{})
			Expect(err).NotTo(HaveOccurred())
			g.Expect(client1Notification).To(BeTrueBecause("client1 should have received a notification within 1 minutes"))
			Expect(client2Notification).To(BeTrueBecause("client2 should have received a notification within 1 minutes"))
		}, TestTimeoutMedium, TestRetryInterval).To(Succeed())
	})

	It("should filter tools based on x-authorized-tools JWT header", func() {
		if !IsTrustedHeadersEnabled() {
			Skip("trusted headers public key not configured - skipping x-authorized-tools test")
		}

		By("Creating an MCPServer with tools")
		registration := NewMCPServerRegistration("authorized-tools-test", k8sClient)
		testResources = append(testResources, registration.GetObjects()...)
		registeredServer := registration.Register(ctx)

		By("Ensuring the gateway has registered the server")
		Eventually(func(g Gomega) {
			g.Expect(VerifyMCPServerReady(ctx, k8sClient, registeredServer.Name, registeredServer.Namespace)).To(BeNil())
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Verifying MCPServer tools are present without filtering")
		var allTools *mcp.ListToolsResult
		Eventually(func(g Gomega) {
			var err error
			allTools, err = mcpGatewayClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(allTools).NotTo(BeNil())
			g.Expect(verifyMCPServerToolsPresent(registeredServer.Spec.ToolPrefix, allTools)).To(BeTrueBecause("%s should exist", registeredServer.Spec.ToolPrefix))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())

		By("Creating a JWT with allowed tools for the server")

		GinkgoWriter.Println("server name ", registeredServer.Name)

		allowedTools := map[string][]string{
			fmt.Sprintf("%s/%s", registeredServer.Namespace, registeredServer.Name): {"hello_world"},
		}
		jwtToken, err := CreateAuthorizedToolsJWT(allowedTools)
		Expect(err).NotTo(HaveOccurred())

		By("Creating a client with x-authorized-tools header")
		authorizedClient, err := NewMCPGatewayClientWithHeaders(ctx, gatewayURL, map[string]string{
			"X-Authorized-Tools": jwtToken,
		})
		Expect(err).NotTo(HaveOccurred())
		defer authorizedClient.Close()

		By("Verifying only the tools from the JWT are returned")
		Eventually(func(g Gomega) {
			filteredTools, err := authorizedClient.ListTools(ctx, mcp.ListToolsRequest{})
			g.Expect(err).Error().NotTo(HaveOccurred())
			g.Expect(filteredTools).NotTo(BeNil())
			g.Expect(len(filteredTools.Tools)).To(Equal(1), "expected exactly 1 tool from authorized tools header")
			expectedToolName := fmt.Sprintf("%s%s", registeredServer.Spec.ToolPrefix, "hello_world")
			g.Expect(filteredTools.Tools[0].Name).To(Equal(expectedToolName))
		}, TestTimeoutLong, TestRetryInterval).To(Succeed())
	})

})

// verifyMCPServerToolsPresent this will ensure at least one tool in the tools list is from the MCPServer that uses the prefix
func verifyMCPServerToolsPresent(serverPrefix string, toolsList *mcp.ListToolsResult) bool {
	if toolsList == nil {
		return false
	}
	for _, t := range toolsList.Tools {
		if strings.HasPrefix(t.Name, serverPrefix) {
			return true
		}
	}
	return false
}
