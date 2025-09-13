package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
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

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	"github.com/kagenti/mcp-gateway/pkg/config"
)

const (
	// ConfigNamespace is the namespace for config
	ConfigNamespace = "mcp-system"
	// ConfigName is the name of the config
	ConfigName = "mcp-gateway-config"
)

// generateCredentialEnvVar generates an environment variable name for credentials
// following the pattern KAGENTAI_{MCP_NAME}_CRED (note: KAGENTAI with AI at the end)
func generateCredentialEnvVar(mcpServerName string) string {
	// convert to uppercase and replace hyphens with underscores
	// e.g., "weather-service" -> "WEATHER_SERVICE"
	name := strings.ToUpper(mcpServerName)
	name = strings.ReplaceAll(name, "-", "_")
	return fmt.Sprintf("KAGENTAI_%s_CRED", name)
}

// ServerInfo holds server information
type ServerInfo struct {
	Endpoint           string
	Hostname           string
	ToolPrefix         string
	HTTPRouteName      string
	HTTPRouteNamespace string
	CredentialEnvVar   string // env var name if auth configured
}

// MCPServerReconciler reconciles MCPServer resources
type MCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mcp.kagenti.com,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.kagenti.com,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

// Reconcile reconciles an MCPServer resource
func (r *MCPServerReconciler) Reconcile(
	ctx context.Context,
	req reconcile.Request,
) (reconcile.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling MCPServer", "name", req.Name, "namespace", req.Namespace)

	mcpServer := &mcpv1alpha1.MCPServer{}
	err := r.Get(ctx, req.NamespacedName, mcpServer)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("MCPServer resource not found, regenerating aggregated config")
			return r.regenerateAggregatedConfig(ctx)
		}
		log.Error(err, "Failed to get MCPServer")
		return reconcile.Result{}, err
	}

	serverInfos, err := r.discoverServersFromHTTPRoutes(ctx, mcpServer)
	if err != nil {
		log.Error(err, "Failed to discover servers from HTTPRoutes")
		return reconcile.Result{}, r.updateStatus(ctx, mcpServer, false, err.Error())
	}

	if err := r.updateStatus(ctx, mcpServer, true, fmt.Sprintf("MCPServer successfully reconciled with %d servers", len(serverInfos))); err != nil {
		log.Error(err, "Failed to update status")
		return reconcile.Result{}, err
	}

	if err := r.updateHTTPRouteStatus(ctx, mcpServer, true); err != nil {
		log.Error(err, "Failed to update HTTPRoute status")
	}

	return r.regenerateAggregatedConfig(ctx)
}

func (r *MCPServerReconciler) regenerateAggregatedConfig(
	ctx context.Context,
) (reconcile.Result, error) {
	log := log.FromContext(ctx)

	mcpServerList := &mcpv1alpha1.MCPServerList{}
	if err := r.List(ctx, mcpServerList); err != nil {
		log.Error(err, "Failed to list MCPServers")
		return reconcile.Result{}, err
	}

	referencedHTTPRoutes := make(map[string]struct{})
	for _, mcpServer := range mcpServerList.Items {
		targetRef := mcpServer.Spec.TargetRef
		if targetRef.Kind == "HTTPRoute" {
			namespace := mcpServer.Namespace
			if targetRef.Namespace != "" {
				namespace = targetRef.Namespace
			}
			key := fmt.Sprintf("%s/%s", namespace, targetRef.Name)
			referencedHTTPRoutes[key] = struct{}{}
		}
	}

	if len(mcpServerList.Items) == 0 {
		log.Info("No MCPServers found, writing empty ConfigMap")
		emptyConfig := &config.BrokerConfig{
			Servers: []config.ServerConfig{},
		}
		if err := r.writeAggregatedConfig(ctx, emptyConfig); err != nil {
			log.Error(err, "Failed to write empty configuration")
			return reconcile.Result{}, err
		}
		log.Info("Successfully wrote empty ConfigMap")
		return reconcile.Result{}, nil
	}

	brokerConfig := &config.BrokerConfig{
		Servers: []config.ServerConfig{},
	}

	for _, mcpServer := range mcpServerList.Items {
		if !isReady(&mcpServer) {
			log.Info("Skipping MCPServer that is not ready",
				"name", mcpServer.Name,
				"namespace", mcpServer.Namespace)
			continue
		}

		serverInfos, err := r.discoverServersFromHTTPRoutes(ctx, &mcpServer)
		if err != nil {
			log.Error(err, "Failed to discover server endpoints",
				"name", mcpServer.Name,
				"namespace", mcpServer.Namespace)
			continue
		}

		for _, serverInfo := range serverInfos {
			serverName := fmt.Sprintf(
				"%s/%s",
				serverInfo.HTTPRouteNamespace,
				serverInfo.HTTPRouteName,
			)
			serverConfig := config.ServerConfig{
				Name:       serverName,
				URL:        serverInfo.Endpoint,
				Hostname:   serverInfo.Hostname,
				ToolPrefix: serverInfo.ToolPrefix,
				Enabled:    true,
			}

			// add credential env var if configured
			if serverInfo.CredentialEnvVar != "" {
				serverConfig.CredentialEnvVar = serverInfo.CredentialEnvVar
			}

			brokerConfig.Servers = append(brokerConfig.Servers, serverConfig)
		}
	}

	if err := r.writeAggregatedConfig(ctx, brokerConfig); err != nil {
		log.Error(err, "Failed to write aggregated configuration")
		return reconcile.Result{}, err
	}

	// aggregate credentials from all MCPServers into a single secret
	if err := r.aggregateCredentials(ctx, mcpServerList.Items); err != nil {
		log.Error(err, "Failed to aggregate credentials")
		// don't fail reconciliation on credential errors
		// the broker can still work without credentials
	}

	log.Info("Successfully regenerated aggregated configuration",
		"serverCount", len(brokerConfig.Servers))

	if err := r.cleanupOrphanedHTTPRoutes(ctx, referencedHTTPRoutes); err != nil {
		log.Error(err, "Failed to cleanup orphaned HTTPRoute conditions")
	}

	return reconcile.Result{}, nil
}

func (r *MCPServerReconciler) writeAggregatedConfig(
	ctx context.Context,
	brokerConfig *config.BrokerConfig,
) error {
	writer := NewConfigMapWriter(r.Client, r.Scheme)
	return writer.WriteAggregatedConfig(ctx, ConfigNamespace, ConfigName, brokerConfig)
}

func isReady(mcpServer *mcpv1alpha1.MCPServer) bool {
	for _, condition := range mcpServer.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *MCPServerReconciler) discoverServersFromHTTPRoutes(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
) ([]ServerInfo, error) {
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
	if len(httpRoute.Spec.Hostnames) == 0 {
		return nil, fmt.Errorf(
			"HTTPRoute %s/%s must have at least one hostname for MCP backend routing",
			namespace,
			targetRef.Name,
		)
	}
	// use first hostname if multiple are present
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

	serverInfo := ServerInfo{
		Endpoint:           endpoint,
		Hostname:           hostname,
		ToolPrefix:         toolPrefix,
		HTTPRouteName:      targetRef.Name,
		HTTPRouteNamespace: namespace,
	}

	// generate credential env var name if credentialRef is set
	if mcpServer.Spec.CredentialRef != nil {
		// convert mcp server name to env var format
		// e.g., "weather-service" -> "KAGENTI_WEATHER_SERVICE_CRED"
		envVarName := generateCredentialEnvVar(mcpServer.Name)
		serverInfo.CredentialEnvVar = envVarName
	}

	serverInfos = append(serverInfos, serverInfo)

	return serverInfos, nil
}

func (r *MCPServerReconciler) cleanupOrphanedHTTPRoutes(
	ctx context.Context,
	referencedHTTPRoutes map[string]struct{},
) error {
	log := log.FromContext(ctx)

	httpRouteList := &gatewayv1.HTTPRouteList{}
	if err := r.List(ctx, httpRouteList, client.MatchingFields{"status.hasProgrammedCondition": "true"}); err != nil {
		return fmt.Errorf("failed to list programmed HTTPRoutes: %w", err)
	}

	for _, httpRoute := range httpRouteList.Items {
		key := fmt.Sprintf("%s/%s", httpRoute.Namespace, httpRoute.Name)

		if _, referenced := referencedHTTPRoutes[key]; referenced {
			continue
		}

		hasProgrammedCondition := false
		updateNeeded := false
		for i, parentStatus := range httpRoute.Status.Parents {
			newConditions := []metav1.Condition{}
			for _, condition := range parentStatus.Conditions {
				if condition.Type == "Programmed" && condition.Status == metav1.ConditionTrue {
					hasProgrammedCondition = true
					updateNeeded = true
				} else {
					newConditions = append(newConditions, condition)
				}
			}
			if updateNeeded {
				httpRoute.Status.Parents[i].Conditions = newConditions
			}
		}

		if hasProgrammedCondition {
			log.Info("Cleaning up Programmed condition on orphaned HTTPRoute",
				"HTTPRoute", httpRoute.Name,
				"namespace", httpRoute.Namespace)

			if err := r.Status().Update(ctx, &httpRoute); err != nil {
				log.Error(err, "Failed to cleanup HTTPRoute status",
					"HTTPRoute", httpRoute.Name,
					"namespace", httpRoute.Namespace)
			}
		}
	}

	return nil
}

func (r *MCPServerReconciler) updateHTTPRouteStatus(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
	affected bool,
) error {
	log := log.FromContext(ctx)
	targetRef := mcpServer.Spec.TargetRef

	if targetRef.Kind != "HTTPRoute" {
		return nil
	}

	namespace := mcpServer.Namespace
	if targetRef.Namespace != "" {
		namespace = targetRef.Namespace
	}

	httpRoute := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      targetRef.Name,
		Namespace: namespace,
	}, httpRoute)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get HTTPRoute: %w", err)
	}

	condition := metav1.Condition{
		Type:               "Programmed",
		ObservedGeneration: httpRoute.Generation,
		LastTransitionTime: metav1.Now(),
	}

	if affected {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "InUseByMCPServer"
		condition.Message = fmt.Sprintf("HTTPRoute is referenced by MCPServer %s/%s", mcpServer.Namespace, mcpServer.Name)
	} else {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "NotInUse"
		condition.Message = "HTTPRoute is not referenced by any MCPServer"
	}

	found := false
	for i, cond := range httpRoute.Status.Parents {
		conditionFound := false
		for j, c := range cond.Conditions {
			if c.Type == condition.Type {
				httpRoute.Status.Parents[i].Conditions[j] = condition
				conditionFound = true
				break
			}
		}
		if !conditionFound {
			httpRoute.Status.Parents[i].Conditions = append(
				httpRoute.Status.Parents[i].Conditions,
				condition,
			)
		}
		found = true
	}

	if !found {
		log.Info("HTTPRoute has no parent statuses, skipping condition update",
			"HTTPRoute", httpRoute.Name,
			"namespace", httpRoute.Namespace)
		return nil
	}

	if err := r.Status().Update(ctx, httpRoute); err != nil {
		return fmt.Errorf("failed to update HTTPRoute status: %w", err)
	}

	log.Info("Updated HTTPRoute status",
		"HTTPRoute", httpRoute.Name,
		"namespace", httpRoute.Namespace,
		"affected", affected)

	return nil
}

func (r *MCPServerReconciler) updateStatus(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
	ready bool,
	message string,
) error {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "NotReady",
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	if ready {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Ready"
	}

	found := false
	for i, cond := range mcpServer.Status.Conditions {
		if cond.Type == condition.Type {
			mcpServer.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		mcpServer.Status.Conditions = append(mcpServer.Status.Conditions, condition)
	}

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

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &gatewayv1.HTTPRoute{}, "status.hasProgrammedCondition", func(rawObj client.Object) []string {
		httpRoute := rawObj.(*gatewayv1.HTTPRoute)
		for _, parentStatus := range httpRoute.Status.Parents {
			for _, condition := range parentStatus.Conditions {
				if condition.Type == "Programmed" && condition.Status == metav1.ConditionTrue {
					return []string{"true"}
				}
			}
		}
		return []string{"false"}
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

// aggregateCredentials collects credentials from all MCPServers and creates/updates
// a single aggregated secret for the broker to use with envFrom
func (r *MCPServerReconciler) aggregateCredentials(ctx context.Context, mcpServers []mcpv1alpha1.MCPServer) error {
	log := log.FromContext(ctx)

	// collect all credentials
	aggregatedData := make(map[string][]byte)

	for _, mcpServer := range mcpServers {
		if mcpServer.Spec.CredentialRef == nil {
			continue // skip if no credentials configured
		}

		// fetch the referenced secret
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      mcpServer.Spec.CredentialRef.Name,
			Namespace: mcpServer.Namespace,
		}, secret)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Error(err, "Referenced secret not found",
					"mcpserver", mcpServer.Name,
					"secret", mcpServer.Spec.CredentialRef.Name)
				continue // skip this one but continue with others
			}
			return fmt.Errorf("failed to get secret %s: %w", mcpServer.Spec.CredentialRef.Name, err)
		}

		// determine which key to use
		key := mcpServer.Spec.CredentialRef.Key
		if key == "" {
			key = "token" // default
		}

		// get the credential value
		credValue, exists := secret.Data[key]
		if !exists {
			log.Error(nil, "Secret key not found",
				"secret", mcpServer.Spec.CredentialRef.Name,
				"key", key)
			continue
		}

		// add to aggregated data with standardized env var name
		envVarName := generateCredentialEnvVar(mcpServer.Name)
		aggregatedData[envVarName] = credValue
	}

	// create or update the aggregated secret
	aggregatedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcp-aggregated-credentials",
			Namespace: ConfigNamespace,
			Labels: map[string]string{
				"app":                        "mcp-gateway",
				"mcp.kagenti.com/aggregated": "true",
			},
		},
		Data: aggregatedData,
	}

	// check if secret exists
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      aggregatedSecret.Name,
		Namespace: aggregatedSecret.Namespace,
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// create new secret
			if err := r.Create(ctx, aggregatedSecret); err != nil {
				return fmt.Errorf("failed to create aggregated secret: %w", err)
			}
			log.Info("Created aggregated credentials secret",
				"credentialCount", len(aggregatedData))
		} else {
			return fmt.Errorf("failed to get aggregated secret: %w", err)
		}
	} else {
		// update existing secret
		existing.Data = aggregatedData
		if err := r.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update aggregated secret: %w", err)
		}
		log.Info("Updated aggregated credentials secret",
			"credentialCount", len(aggregatedData))
	}

	return nil
}
