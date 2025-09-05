package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpclient "github.com/kagenti/mcp-gateway/internal/mcp"
	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	"github.com/kagenti/mcp-gateway/pkg/config"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// ConfigNamespace is the namespace for config
	ConfigNamespace = "mcp-system"
	// ConfigName is the name of the config
	ConfigName = "mcp-gateway-config"
)

// ServerInfo holds server information
type ServerInfo struct {
	Endpoint           string
	Hostname           string
	ToolPrefix         string
	HTTPRouteName      string
	HTTPRouteNamespace string
}

type mcpServerValidation struct {
	mcpServer             mcpv1alpha1.MCPServer
	validationData        []ServerValidationData
	registrationDecisions map[string]ServerRegistrationDecision
}

// ServerValidationData holds all information gathered from a single MCP connection
type ServerValidationData struct {
	ServerInfo      ServerInfo
	ConnectionError error
	InitResult      *mcp.InitializeResult
	Connected       bool
	ValidTransport  bool
	Tools           []mcp.Tool
}

// ServerRegistrationDecision represents whether a server should be registered and why
type ServerRegistrationDecision struct {
	ShouldRegister bool
	Reason         string
	StatusMessage  string
}

// MCPServerReconciler reconciles MCPServer resources
type MCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mcp.kagenti.com,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.kagenti.com,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

// Reconcile reconciles an MCPServer resource
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	slog.Info("Reconciling MCPServer", "name", req.Name, "namespace", req.Namespace)

	mcpServer := &mcpv1alpha1.MCPServer{}
	err := r.Get(ctx, req.NamespacedName, mcpServer)
	if err != nil {
		if errors.IsNotFound(err) {
			slog.Info("MCPServer resource not found, regenerating aggregated config")
			return r.regenerateAggregatedConfig(ctx)
		}
		slog.Error("Failed to get MCPServer", "error", err)
		return reconcile.Result{}, err
	}

	serverInfos, err := r.discoverServersFromHTTPRoutes(ctx, mcpServer)
	if err != nil {
		slog.Error("Failed to discover servers from HTTPRoutes", "error", err)
		return reconcile.Result{}, err
	}

	if mcpServer.Generation == mcpServer.Status.ObservedGeneration {
		return r.regenerateAggregatedConfig(ctx)
	}

	slog.Info("MCPServe changed, validating MCP servers", "serverCount", len(serverInfos))

	// Perform validation and determine registration decisions
	validationData, err := r.gatherServerInfo(ctx, serverInfos)
	if err != nil {
		slog.Error("Server validation failed", "error", err)
		if updateErr := r.updateErrorStatus(ctx, mcpServer, fmt.Sprintf("Validation failed: %s", err.Error())); updateErr != nil {
			slog.Error("Failed to update error status", "error", updateErr)
		}
		return reconcile.Result{}, err
	}

	registrationDecisions := r.determineServerRegistration(validationData)

	// Categorize servers for Ready condition
	var healthyCount int
	var degradedServers []string
	var excludedServers []string

	for serverName, decision := range registrationDecisions {
		if decision.ShouldRegister {
			if decision.StatusMessage == "" {
				healthyCount++
			} else {
				degradedServers = append(degradedServers, fmt.Sprintf("%s (%s)", serverName, decision.StatusMessage))
			}
		} else {
			excludedServers = append(excludedServers, fmt.Sprintf("%s (%s)", serverName, decision.StatusMessage))
		}
	}

	// Update status with Ready condition
	if updateErr := r.updateReadyStatus(ctx, mcpServer, degradedServers, excludedServers); updateErr != nil {
		slog.Error("Failed to update status", "error", updateErr)
		return reconcile.Result{}, updateErr
	}

	// Regenerate config after validation
	if _, err := r.regenerateAggregatedConfig(ctx); err != nil {
		slog.Error("Failed to regenerate aggregated config", "error", err)
		return reconcile.Result{}, err
	}

	// Reconcile on server changes
	return reconcile.Result{}, nil
}

func (r *MCPServerReconciler) regenerateAggregatedConfig(ctx context.Context) (reconcile.Result, error) {
	mcpServerList := &mcpv1alpha1.MCPServerList{}
	if err := r.List(ctx, mcpServerList); err != nil {
		slog.Error("Failed to list MCPServers", "error", err)
		return reconcile.Result{}, err
	}

	if len(mcpServerList.Items) == 0 {
		slog.Info("No MCPServers found, writing empty ConfigMap")
		emptyConfig := &config.BrokerConfig{
			Servers: []config.ServerConfig{},
		}
		if err := r.writeAggregatedConfig(ctx, emptyConfig); err != nil {
			slog.Error("Failed to write empty configuration", "error", err)
			return reconcile.Result{}, err
		}
		slog.Info("Successfully wrote empty ConfigMap")
		return reconcile.Result{}, nil
	}

	brokerConfig := &config.BrokerConfig{
		Servers: []config.ServerConfig{},
	}

	var allValidations []mcpServerValidation

	for _, mcpServer := range mcpServerList.Items {

		serverInfos, err := r.discoverServersFromHTTPRoutes(ctx, &mcpServer)
		if err != nil {
			slog.Error("Failed to discover server endpoints",
				"error", err,
				"name", mcpServer.Name,
				"namespace", mcpServer.Namespace)
			continue
		}

		// Validate servers
		validationData, err := r.gatherServerInfo(ctx, serverInfos)
		if err != nil {
			slog.Error("Failed to validate servers for broker config",
				"error", err,
				"name", mcpServer.Name,
				"namespace", mcpServer.Namespace)
			// If validation fails, dont register the server
			slog.Info("Skipping server registration due to validation failure",
				"mcpServer", mcpServer.Name,
				"namespace", mcpServer.Namespace)
			continue
		}

		registrationDecisions := r.determineServerRegistration(validationData)

		allValidations = append(allValidations, mcpServerValidation{
			mcpServer:             mcpServer,
			validationData:        validationData,
			registrationDecisions: registrationDecisions,
		})
	}

	var allValidationData []ServerValidationData
	for _, validation := range allValidations {
		for _, data := range validation.validationData {
			serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)
			if decision, exists := validation.registrationDecisions[serverName]; exists && decision.ShouldRegister {
				allValidationData = append(allValidationData, data)
			}
		}
	}

	// Global tool conflict detection
	globalConflictMap := r.validateToolConflicts(allValidationData)

	for _, validation := range allValidations {
		for _, data := range validation.validationData {
			serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)

			if decision, exists := validation.registrationDecisions[serverName]; exists && decision.ShouldRegister {
				if globalConflictMap[serverName] {
					slog.Info("Server excluded from broker config", "server", serverName, "reason", "tool_naming_conflicts", "endpoint", data.ServerInfo.Endpoint)
					continue
				}

				brokerConfig.Servers = append(brokerConfig.Servers, config.ServerConfig{
					Name:       serverName,
					URL:        data.ServerInfo.Endpoint,
					Hostname:   data.ServerInfo.Hostname,
					ToolPrefix: data.ServerInfo.ToolPrefix,
					Enabled:    true,
					// TODO: Handle credentialRef when implementing auth
				})

				slog.Info("Server registered with broker", "server", serverName, "reason", decision.Reason, "endpoint", data.ServerInfo.Endpoint)
			} else {
				slog.Info("Server excluded from broker config", "server", serverName, "reason", decision.Reason, "endpoint", data.ServerInfo.Endpoint)
			}
		}
	}

	if err := r.writeAggregatedConfig(ctx, brokerConfig); err != nil {
		slog.Error("Failed to write aggregated configuration", "error", err)
		return reconcile.Result{}, err
	}

	slog.Info("Successfully regenerated aggregated configuration",
		"serverCount", len(brokerConfig.Servers))
	return reconcile.Result{}, nil
}

// determineServerRegistration decides if servers should be registered based on validation results
func (r *MCPServerReconciler) determineServerRegistration(validationData []ServerValidationData) map[string]ServerRegistrationDecision {
	decisions := make(map[string]ServerRegistrationDecision)

	// Run validation checks using existing functions (all return maps for efficient lookup)
	connectivity := r.validateConnectivity(validationData)
	capability := r.validateCapabilities(validationData)
	protocol := r.validateProtocolVersion(validationData)
	transport := r.validateTransport(validationData)

	for _, data := range validationData {
		serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)

		// Apply validation rules
		if connectivity[serverName] {
			// 1: Connectivity fails → Still register
			decisions[serverName] = ServerRegistrationDecision{
				ShouldRegister: true,
				Reason:         "connectivity_failed_but_registered",
				StatusMessage:  "connection failed",
			}
		} else if capability[serverName] {
			// 2: Missing tools capability → Don't register
			decisions[serverName] = ServerRegistrationDecision{
				ShouldRegister: false,
				Reason:         "missing_required_tools_capability",
				StatusMessage:  "excluded (missing tools)",
			}
		} else if protocol[serverName] {
			// 3: Wrong protocol version → Don't register
			decisions[serverName] = ServerRegistrationDecision{
				ShouldRegister: false,
				Reason:         "unsupported_protocol_version",
				StatusMessage:  "excluded (unsupported protocol)",
			}
		} else if transport[serverName] {
			// 4: Wrong transport → Don't register
			decisions[serverName] = ServerRegistrationDecision{
				ShouldRegister: false,
				Reason:         "unsupported_transport",
				StatusMessage:  "excluded (unsupported transport)",
			}
		} else {
			// 5: All validations passed → Register (tool conflicts checked globally)
			decisions[serverName] = ServerRegistrationDecision{
				ShouldRegister: true,
				Reason:         "valid_server",
				StatusMessage:  "",
			}
		}
	}

	return decisions
}

func (r *MCPServerReconciler) writeAggregatedConfig(ctx context.Context, brokerConfig *config.BrokerConfig) error {
	writer := NewConfigMapWriter(r.Client, r.Scheme)
	return writer.WriteAggregatedConfig(ctx, ConfigNamespace, ConfigName, brokerConfig)
}

func (r *MCPServerReconciler) discoverServersFromHTTPRoutes(ctx context.Context, mcpServer *mcpv1alpha1.MCPServer) ([]ServerInfo, error) {
	var serverInfos []ServerInfo

	targetRef := mcpServer.Spec.TargetRef

	// Validate group and kind
	if targetRef.Group != "gateway.networking.k8s.io" {
		return nil, fmt.Errorf(
			"invalid targetRef group %q: only gateway.networking.k8s.io is supported",
			targetRef.Group,
		)
	}
	if targetRef.Kind != "HTTPRoute" {
		return nil, fmt.Errorf(
			"invalid targetRef kind %q: only HTTPRoute is supported",
			targetRef.Kind,
		)
	}

	namespace := mcpServer.Namespace
	if targetRef.Namespace != "" && targetRef.Namespace != namespace {
		return nil, fmt.Errorf(
			"cross-namespace reference to %s/%s not allowed without ReferenceGrant support",
			targetRef.Namespace,
			targetRef.Name,
		)
	}

	httpRoute := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      targetRef.Name,
		Namespace: namespace,
	}, httpRoute)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("HTTPRoute %s/%s not found", namespace, targetRef.Name)
		}
		return nil, fmt.Errorf(
			"failed to get HTTPRoute %s/%s: %w",
			namespace,
			targetRef.Name,
			err,
		)
	}

	if len(httpRoute.Spec.Rules) == 0 || len(httpRoute.Spec.Rules[0].BackendRefs) == 0 {
		return nil, fmt.Errorf(
			"HTTPRoute %s/%s has no backend references",
			namespace,
			targetRef.Name,
		)
	}

	backendRef := httpRoute.Spec.Rules[0].BackendRefs[0]
	if backendRef.Name == "" {
		return nil, fmt.Errorf("backend reference has no name")
	}

	kind := "Service"
	if backendRef.Kind != nil {
		kind = string(*backendRef.Kind)
	}

	if kind != "Service" {
		return nil, fmt.Errorf("backend reference is not a Service: %s", kind)
	}

	// Determine service namespace, default to HTTPRoute namespace
	serviceNamespace := httpRoute.Namespace
	if backendRef.Namespace != nil {
		serviceNamespace = string(*backendRef.Namespace)
	}

	// Construct full service DNS name
	serviceDNSName := fmt.Sprintf("%s.%s.svc.cluster.local", backendRef.Name, serviceNamespace)

	var nameAndEndpoint string
	if backendRef.Port != nil {
		nameAndEndpoint = fmt.Sprintf("%s:%d", serviceDNSName, *backendRef.Port)
	} else {
		nameAndEndpoint = serviceDNSName
	}

	toolPrefix := mcpServer.Spec.ToolPrefix

	// Extract hostname from HTTPRoute
	if len(httpRoute.Spec.Hostnames) != 1 {
		return nil, fmt.Errorf(
			"HTTPRoute %s/%s must have exactly one hostname for MCP backend routing, found %d",
			namespace,
			targetRef.Name,
			len(httpRoute.Spec.Hostnames),
		)
	}
	hostname := string(httpRoute.Spec.Hostnames[0])

	protocol := "http"
	if httpRoute.Spec.ParentRefs != nil {
		for _, parentRef := range httpRoute.Spec.ParentRefs {
			if parentRef.SectionName != nil &&
				strings.Contains(string(*parentRef.SectionName), "https") {
				protocol = "https"
				break
			}
		}
	}

	// to think about: service vs ingress via GW API
	endpoint := fmt.Sprintf("%s://%s/mcp", protocol, nameAndEndpoint)

	serverInfos = append(serverInfos, ServerInfo{
		Endpoint:           endpoint,
		Hostname:           hostname,
		ToolPrefix:         toolPrefix,
		HTTPRouteName:      targetRef.Name,
		HTTPRouteNamespace: namespace,
	})

	return serverInfos, nil
}

// gatherServerInfo makes a single connection per server and collects all validation data
func (r *MCPServerReconciler) gatherServerInfo(ctx context.Context, serverInfos []ServerInfo) ([]ServerValidationData, error) {
	validationData := make([]ServerValidationData, 0, len(serverInfos))

	for _, serverInfo := range serverInfos {
		client, initResult, err := mcpclient.CreateClient(ctx, serverInfo.Endpoint)
		if err != nil {
			data := ServerValidationData{
				ServerInfo:      serverInfo,
				ConnectionError: err,
				InitResult:      nil,
				Connected:       false,
				ValidTransport:  false,
				Tools:           nil,
			}
			validationData = append(validationData, data)
			continue
		}

		// Validate transport based on protocol version
		validTransport := false
		if initResult != nil {
			switch initResult.ProtocolVersion {
			case "2025-06-18":
				validTransport = true
			case "2024-11-05":
				validTransport = false
			default:
				validTransport = false
			}
		}

		// List tools from the server for conflict detection
		var tools []mcp.Tool
		if toolsResult, err := mcpclient.ListTools(ctx, client); err != nil {
			slog.Info("Failed to list tools from server", "server", serverInfo.Endpoint, "error", err)
		} else if toolsResult != nil {
			tools = toolsResult.Tools
			slog.Info("Listed tools from server", "server", serverInfo.Endpoint, "toolCount", len(tools))
		}

		// Create validation data with all information
		data := ServerValidationData{
			ServerInfo:      serverInfo,
			ConnectionError: nil,
			InitResult:      initResult,
			Connected:       true,
			ValidTransport:  validTransport,
			Tools:           tools,
		}

		if err := client.Close(); err != nil {
			slog.Warn("Failed to close MCP client connection", "error", err, "endpoint", data.ServerInfo.Endpoint)
		}

		validationData = append(validationData, data)
	}

	return validationData, nil
}

// validateConnectivity checks connection results from gathered data
func (r *MCPServerReconciler) validateConnectivity(validationData []ServerValidationData) map[string]bool {
	failedServers := make(map[string]bool)

	for _, data := range validationData {
		if !data.Connected {
			serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)
			failedServers[serverName] = true

			slog.Info("Server connectivity validation failed", "server", serverName, "endpoint", data.ServerInfo.Endpoint, "error", data.ConnectionError)
		}
	}

	return failedServers
}

// validateCapabilities checks if servers have tools capability
func (r *MCPServerReconciler) validateCapabilities(validationData []ServerValidationData) map[string]bool {
	failedCapabilities := make(map[string]bool)

	for _, data := range validationData {
		if !data.Connected {
			continue
		}

		serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)

		if data.InitResult.Capabilities.Tools == nil {
			failedCapabilities[serverName] = true

			slog.Info("Server capability validation failed - missing tools capability", "server", serverName, "endpoint", data.ServerInfo.Endpoint, "reason", "missing tools capability (required by broker)")
		}
	}

	return failedCapabilities
}

// validateProtocolVersion checks protocol version compatibility (no network calls)
func (r *MCPServerReconciler) validateProtocolVersion(validationData []ServerValidationData) map[string]bool {
	failedValidation := make(map[string]bool)
	const requiredProtocolVersion = "2025-06-18"

	for _, data := range validationData {
		if !data.Connected {
			continue
		}

		serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)

		if data.InitResult.ProtocolVersion != requiredProtocolVersion {
			failedValidation[serverName] = true
			slog.Info("Server protocol validation failed - wrong version", "server", serverName, "endpoint", data.ServerInfo.Endpoint, "actualProtocol", data.InitResult.ProtocolVersion, "requiredProtocol", requiredProtocolVersion)
		}
	}

	return failedValidation
}

// validateTransport checks transport compatibility
func (r *MCPServerReconciler) validateTransport(validationData []ServerValidationData) map[string]bool {
	failedValidation := make(map[string]bool)

	for _, data := range validationData {
		if !data.Connected {
			continue
		}

		serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)

		if !data.ValidTransport {
			failedValidation[serverName] = true
			slog.Info("Server transport validation failed - unsupported transport", "server", serverName, "endpoint", data.ServerInfo.Endpoint, "protocolVersion", data.InitResult.ProtocolVersion)
		}
	}

	return failedValidation
}

// validateToolConflicts checks for tool naming conflicts between servers
func (r *MCPServerReconciler) validateToolConflicts(validationData []ServerValidationData) map[string]bool {
	toolToServers := make(map[string][]string)

	for _, data := range validationData {

		serverName := fmt.Sprintf("%s/%s", data.ServerInfo.HTTPRouteNamespace, data.ServerInfo.HTTPRouteName)
		toolPrefix := data.ServerInfo.ToolPrefix

		for _, tool := range data.Tools {
			finalToolName := tool.Name
			if toolPrefix != "" {
				finalToolName = toolPrefix + tool.Name
			}

			toolToServers[finalToolName] = append(toolToServers[finalToolName], serverName)
		}
	}

	conflictedServers := make(map[string]bool)
	for toolName, servers := range toolToServers {
		if len(servers) > 1 {
			slog.Info("Tool name conflict detected", "toolName", toolName, "conflictingServers", strings.Join(servers, ", "), "serverCount", len(servers))

			for _, server := range servers {
				conflictedServers[server] = true
			}
		}
	}

	return conflictedServers
}

// updateReadyStatus updates MCPServer status with Ready condition
func (r *MCPServerReconciler) updateReadyStatus(ctx context.Context, mcpServer *mcpv1alpha1.MCPServer, degradedServers, excludedServers []string) error {
	readyCondition := metav1.Condition{
		Type:               "Ready",
		LastTransitionTime: metav1.Now(),
	}

	if len(excludedServers) > 0 {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "ServerExcluded"
		readyCondition.Message = fmt.Sprintf("Server excluded: %s", strings.Join(excludedServers, ", "))
	} else if len(degradedServers) > 0 {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "ServerDegraded"
		readyCondition.Message = "Server registered (connectivity issues detected)"
	} else {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "ServerRegistered"
		readyCondition.Message = "Server registered"
	}

	r.setCondition(mcpServer, readyCondition)

	mcpServer.Status.ObservedGeneration = mcpServer.Generation

	return r.Status().Update(ctx, mcpServer)
}

// setCondition updates or adds a condition to the MCPServer status
func (r *MCPServerReconciler) setCondition(mcpServer *mcpv1alpha1.MCPServer, newCondition metav1.Condition) {
	found := false
	for i, existingCondition := range mcpServer.Status.Conditions {
		if existingCondition.Type == newCondition.Type {
			mcpServer.Status.Conditions[i] = newCondition
			found = true
			break
		}
	}
	if !found {
		mcpServer.Status.Conditions = append(mcpServer.Status.Conditions, newCondition)
	}
}

// updateErrorStatus sets error conditions when validation/discovery fails
func (r *MCPServerReconciler) updateErrorStatus(ctx context.Context, mcpServer *mcpv1alpha1.MCPServer, errorMessage string) error {
	// Ready condition = False (system error)
	readyCondition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ValidationError",
		Message:            errorMessage,
		LastTransitionTime: metav1.Now(),
	}

	// Update condition and save
	r.setCondition(mcpServer, readyCondition)

	// Update ObservedGeneration even for error states
	mcpServer.Status.ObservedGeneration = mcpServer.Generation

	return r.Status().Update(ctx, mcpServer)
}

// SetupWithManager sets up the reconciler
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &mcpv1alpha1.MCPServer{}, "spec.targetRef.httproute", func(rawObj client.Object) []string {
		mcpServer := rawObj.(*mcpv1alpha1.MCPServer)

		targetRef := mcpServer.Spec.TargetRef
		if targetRef.Kind == "HTTPRoute" {
			namespace := targetRef.Namespace
			if namespace == "" {
				namespace = mcpServer.Namespace
			}
			return []string{fmt.Sprintf("%s/%s", namespace, targetRef.Name)}
		}
		return []string{}
	}); err != nil {
		return err
	}

	controller := ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPServer{}).
		Watches(
			&gatewayv1.HTTPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findMCPServersForHTTPRoute),
		)

	// Perform startup reconciliation to ensure config exists even with zero MCPServers
	if err := mgr.Add(&startupReconciler{reconciler: r}); err != nil {
		return err
	}

	return controller.Complete(r)
}

// findMCPServersForHTTPRoute finds all MCPServers that reference the given HTTPRoute
func (r *MCPServerReconciler) findMCPServersForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	httpRoute := obj.(*gatewayv1.HTTPRoute)
	log := log.FromContext(ctx).WithValues("HTTPRoute", httpRoute.Name, "namespace", httpRoute.Namespace)

	indexKey := fmt.Sprintf("%s/%s", httpRoute.Namespace, httpRoute.Name)
	mcpServerList := &mcpv1alpha1.MCPServerList{}
	if err := r.List(ctx, mcpServerList, client.MatchingFields{"spec.targetRef.httproute": indexKey}); err != nil {
		log.Error(err, "Failed to list MCPServers using index")
		return nil
	}

	var requests []reconcile.Request
	for _, mcpServer := range mcpServerList.Items {
		log.Info("Found MCPServer referencing HTTPRoute via index",
			"MCPServer", mcpServer.Name,
			"MCPServerNamespace", mcpServer.Namespace)
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			},
		})
	}

	return requests
}

// startupReconciler ensures initial configuration is written even with zero MCPServers
type startupReconciler struct {
	reconciler *MCPServerReconciler
}

// Start implements manager.Runnable
func (s *startupReconciler) Start(ctx context.Context) error {
	log := log.FromContext(ctx).WithName("startup-reconciler")
	log.Info("Running startup reconciliation to ensure config exists")

	if _, err := s.reconciler.regenerateAggregatedConfig(ctx); err != nil {
		log.Error(err, "Failed to run startup reconciliation")
		return err
	}

	log.Info("Startup reconciliation completed successfully")
	return nil
}
