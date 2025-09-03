package controller

import (
	"context"
	"fmt"
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

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	"github.com/kagenti/mcp-gateway/pkg/config"
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
			brokerConfig.Servers = append(brokerConfig.Servers, config.ServerConfig{
				Name:       serverName,
				URL:        serverInfo.Endpoint,
				Hostname:   serverInfo.Hostname,
				ToolPrefix: serverInfo.ToolPrefix,
				Enabled:    true,
				// TODO: Handle credentialRef when implementing auth
			})
		}
	}

	if err := r.writeAggregatedConfig(ctx, brokerConfig); err != nil {
		log.Error(err, "Failed to write aggregated configuration")
		return reconcile.Result{}, err
	}

	log.Info("Successfully regenerated aggregated configuration",
		"serverCount", len(brokerConfig.Servers))
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

	for _, targetRef := range mcpServer.Spec.TargetRefs {
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
	}

	return serverInfos, nil
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
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &mcpv1alpha1.MCPServer{}, "spec.targetRefs.httproute", func(rawObj client.Object) []string {
		mcpServer := rawObj.(*mcpv1alpha1.MCPServer)
		var httpRoutes []string
		for _, targetRef := range mcpServer.Spec.TargetRefs {
			if targetRef.Kind == "HTTPRoute" {
				namespace := targetRef.Namespace
				if namespace == "" {
					namespace = mcpServer.Namespace
				}
				httpRoutes = append(httpRoutes, fmt.Sprintf("%s/%s", namespace, targetRef.Name))
			}
		}
		return httpRoutes
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPServer{}).
		Watches(
			&gatewayv1.HTTPRoute{},
			handler.EnqueueRequestsFromMapFunc(r.findMCPServersForHTTPRoute),
		).
		Complete(r)
}

// findMCPServersForHTTPRoute finds all MCPServers that reference the given HTTPRoute
func (r *MCPServerReconciler) findMCPServersForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	httpRoute := obj.(*gatewayv1.HTTPRoute)
	log := log.FromContext(ctx).WithValues("HTTPRoute", httpRoute.Name, "namespace", httpRoute.Namespace)

	indexKey := fmt.Sprintf("%s/%s", httpRoute.Namespace, httpRoute.Name)
	mcpServerList := &mcpv1alpha1.MCPServerList{}
	if err := r.List(ctx, mcpServerList, client.MatchingFields{"spec.targetRefs.httproute": indexKey}); err != nil {
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
