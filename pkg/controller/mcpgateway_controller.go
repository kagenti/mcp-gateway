package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	"github.com/kagenti/mcp-gateway/pkg/config"
)

const (
	ConfigNamespace = "mcp-system"
	ConfigName      = "mcp-gateway-config"
)

type ServerInfo struct {
	Endpoint           string
	Hostname           string
	ToolPrefix         string
	HTTPRouteName      string
	HTTPRouteNamespace string
}

type MCPGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mcp.kagenti.com,resources=mcpgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.kagenti.com,resources=mcpgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

func (r *MCPGatewayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling MCPGateway", "name", req.Name, "namespace", req.Namespace)

	mcpGateway := &mcpv1alpha1.MCPGateway{}
	err := r.Get(ctx, req.NamespacedName, mcpGateway)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("MCPGateway resource not found, regenerating aggregated config")
			return r.regenerateAggregatedConfig(ctx)
		}
		log.Error(err, "Failed to get MCPGateway")
		return reconcile.Result{}, err
	}

	serverInfos, err := r.discoverServersFromHTTPRoutes(ctx, mcpGateway)
	if err != nil {
		log.Error(err, "Failed to discover servers from HTTPRoutes")
		return reconcile.Result{}, r.updateStatus(ctx, mcpGateway, false, err.Error())
	}

	if err := r.updateStatus(ctx, mcpGateway, true, fmt.Sprintf("MCPGateway successfully reconciled with %d servers", len(serverInfos))); err != nil {
		log.Error(err, "Failed to update status")
		return reconcile.Result{}, err
	}

	return r.regenerateAggregatedConfig(ctx)
}

func (r *MCPGatewayReconciler) regenerateAggregatedConfig(ctx context.Context) (reconcile.Result, error) {
	log := log.FromContext(ctx)

	mcpGatewayList := &mcpv1alpha1.MCPGatewayList{}
	if err := r.List(ctx, mcpGatewayList); err != nil {
		log.Error(err, "Failed to list MCPGateways")
		return reconcile.Result{}, err
	}

	if len(mcpGatewayList.Items) == 0 {
		log.Info("No MCPGateways found, deleting ConfigMap if it exists")
		configMap := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      ConfigName,
			Namespace: ConfigNamespace,
		}, configMap)
		if err == nil {
			if err := r.Delete(ctx, configMap); err != nil {
				log.Error(err, "Failed to delete ConfigMap")
				return reconcile.Result{}, err
			}
		} else if !errors.IsNotFound(err) {
			// Only return error if it's not a NotFound error
			log.Error(err, "Failed to get ConfigMap")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	brokerConfig := &config.BrokerConfig{
		Port:     8080,
		BindAddr: "0.0.0.0",
		LogLevel: "info",
		Servers:  []config.ServerConfig{},
	}

	for _, mcpGateway := range mcpGatewayList.Items {
		if !isReady(&mcpGateway) {
			log.Info("Skipping MCPGateway that is not ready",
				"name", mcpGateway.Name,
				"namespace", mcpGateway.Namespace)
			continue
		}

		serverInfos, err := r.discoverServersFromHTTPRoutes(ctx, &mcpGateway)
		if err != nil {
			log.Error(err, "Failed to discover server endpoints",
				"name", mcpGateway.Name,
				"namespace", mcpGateway.Namespace)
			continue
		}

		for _, serverInfo := range serverInfos {
			serverName := fmt.Sprintf("%s/%s", serverInfo.HTTPRouteNamespace, serverInfo.HTTPRouteName)
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
	return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *MCPGatewayReconciler) writeAggregatedConfig(ctx context.Context, brokerConfig *config.BrokerConfig) error {
	writer := NewConfigMapWriter(r.Client, r.Scheme)
	return writer.WriteAggregatedConfig(ctx, ConfigNamespace, ConfigName, brokerConfig)
}

func isReady(mcpGateway *mcpv1alpha1.MCPGateway) bool {
	for _, condition := range mcpGateway.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *MCPGatewayReconciler) discoverServersFromHTTPRoutes(ctx context.Context, mcpGateway *mcpv1alpha1.MCPGateway) ([]ServerInfo, error) {
	var serverInfos []ServerInfo

	for _, targetRef := range mcpGateway.Spec.TargetRefs {
		// Validate group and kind
		if targetRef.Group != "gateway.networking.k8s.io" {
			return nil, fmt.Errorf("invalid targetRef group %q: only gateway.networking.k8s.io is supported", targetRef.Group)
		}
		if targetRef.Kind != "HTTPRoute" {
			return nil, fmt.Errorf("invalid targetRef kind %q: only HTTPRoute is supported", targetRef.Kind)
		}

		namespace := mcpGateway.Namespace
		if targetRef.Namespace != "" && targetRef.Namespace != namespace {
			return nil, fmt.Errorf("cross-namespace reference to %s/%s not allowed without ReferenceGrant support", targetRef.Namespace, targetRef.Name)
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
			return nil, fmt.Errorf("failed to get HTTPRoute %s/%s: %w", namespace, targetRef.Name, err)
		}

		if len(httpRoute.Spec.Rules) == 0 || len(httpRoute.Spec.Rules[0].BackendRefs) == 0 {
			return nil, fmt.Errorf("HTTPRoute %s/%s has no backend references", namespace, targetRef.Name)
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

		backendNamespace := namespace
		if backendRef.Namespace != nil {
			backendNamespace = string(*backendRef.Namespace)
		}

		port := int32(80)
		if backendRef.Port != nil {
			port = int32(*backendRef.Port)
		}

		endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", backendRef.Name, backendNamespace, port)

		toolPrefix := targetRef.ToolPrefix
		if toolPrefix == "" {
			toolPrefix = mcpGateway.Spec.ToolPrefix
		}

		// Extract hostname from HTTPRoute
		if len(httpRoute.Spec.Hostnames) != 1 {
			return nil, fmt.Errorf("HTTPRoute %s/%s must have exactly one hostname for MCP backend routing, found %d",
				namespace, targetRef.Name, len(httpRoute.Spec.Hostnames))
		}
		hostname := string(httpRoute.Spec.Hostnames[0])

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

func (r *MCPGatewayReconciler) updateStatus(ctx context.Context, mcpGateway *mcpv1alpha1.MCPGateway, ready bool, message string) error {
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
	for i, cond := range mcpGateway.Status.Conditions {
		if cond.Type == condition.Type {
			mcpGateway.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		mcpGateway.Status.Conditions = append(mcpGateway.Status.Conditions, condition)
	}

	return r.Status().Update(ctx, mcpGateway)
}

func (r *MCPGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPGateway{}).
		Complete(r)
}
