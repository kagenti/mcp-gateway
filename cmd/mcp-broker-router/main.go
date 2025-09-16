// main implements the CLI for the MCP broker.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/fsnotify/fsnotify"
	"github.com/kagenti/mcp-gateway/internal/broker"
	config "github.com/kagenti/mcp-gateway/internal/config"
	mcpRouter "github.com/kagenti/mcp-gateway/internal/mcp-router"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	"github.com/kagenti/mcp-gateway/pkg/controller"
)

var (
	mcpConfig = &config.MCPServersConfig{}
	mutex     sync.RWMutex
	logger    = slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheme    = runtime.NewScheme()
)

// OAuthProtectedResource represents the OAuth protected resource response
type OAuthProtectedResource struct {
	ResourceName           string   `json:"resource_name"`
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported"`
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = mcpv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)
}

// getOAuthConfig parses OAuth configuration from environment variables
func getOAuthConfig() *OAuthProtectedResource {
	// Set defaults
	oauthConfig := &OAuthProtectedResource{
		ResourceName:           "MCP Server",
		Resource:               "/mcp",
		AuthorizationServers:   []string{},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{"basic"},
	}

	// Override with environment variables if provided
	if resourceName := os.Getenv("OAUTH_RESOURCE_NAME"); resourceName != "" {
		oauthConfig.ResourceName = resourceName
	}

	if resource := os.Getenv("OAUTH_RESOURCE"); resource != "" {
		oauthConfig.Resource = resource
	}

	if authServers := os.Getenv("OAUTH_AUTHORIZATION_SERVERS"); authServers != "" {
		// Split by comma and trim whitespace
		servers := strings.Split(authServers, ",")
		oauthConfig.AuthorizationServers = make([]string, len(servers))
		for i, server := range servers {
			oauthConfig.AuthorizationServers[i] = strings.TrimSpace(server)
		}
	}

	if bearerMethods := os.Getenv("OAUTH_BEARER_METHODS_SUPPORTED"); bearerMethods != "" {
		// Split by comma and trim whitespace
		methods := strings.Split(bearerMethods, ",")
		oauthConfig.BearerMethodsSupported = make([]string, len(methods))
		for i, method := range methods {
			oauthConfig.BearerMethodsSupported[i] = strings.TrimSpace(method)
		}
	}

	if scopes := os.Getenv("OAUTH_SCOPES_SUPPORTED"); scopes != "" {
		// Split by comma and trim whitespace
		scopeList := strings.Split(scopes, ",")
		oauthConfig.ScopesSupported = make([]string, len(scopeList))
		for i, scope := range scopeList {
			oauthConfig.ScopesSupported[i] = strings.TrimSpace(scope)
		}
	}

	return oauthConfig
}

// oauthProtectedResourceHandler handles the /.well-known/oauth-protected-resource endpoint
func oauthProtectedResourceHandler(oauthConfig *OAuthProtectedResource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, Origin, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Set content type and return JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(oauthConfig); err != nil {
			logger.Error("Failed to encode OAuth protected resource response", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

func main() {

	var (
		mcpRouterAddrFlag string
		mcpBrokerAddrFlag string
		mcpConfigAddrFlag string
		mcpConfigFile     string
		loglevel          int
		logFormat         string
		controllerMode    bool
	)
	flag.StringVar(
		&mcpRouterAddrFlag,
		"mcp-router-address",
		"0.0.0.0:50051",
		"The address for MCP router",
	)
	flag.StringVar(
		&mcpBrokerAddrFlag,
		"mcp-broker-public-address",
		"0.0.0.0:8080",
		"The public address for MCP broker",
	)
	flag.StringVar(
		&mcpConfigAddrFlag,
		"mcp-broker-config-address",
		"0.0.0.0:8181",
		"The internal address for config API",
	)
	flag.StringVar(
		&mcpConfigFile,
		"mcp-gateway-config",
		"./config/mcp-system/config.yaml",
		"where to locate the mcp server config",
	)
	flag.IntVar(
		&loglevel,
		"log-level",
		int(slog.LevelInfo),
		"set the log level 0=info, 4=warn , 8=error and -4=debug",
	)
	flag.StringVar(&logFormat, "log-format", "txt", "switch to json logs with --log-format=json")
	flag.BoolVar(&controllerMode, "controller", false, "Run in controller mode")
	flag.Parse()

	slog.SetLogLoggerLevel(slog.Level(loglevel))

	if logFormat == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	if controllerMode {
		fmt.Println("Starting in controller mode...")
		go func() {
			if err := runController(); err != nil {
				log.Fatalf("Controller failed: %v", err)
			}
		}()
		// Controller doesn't need to run broker/router
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
		<-stop
		logger.Info("shutting down controller")
		return
	}
	ctx := context.Background()
	brokerServer, mcpBroker, mcpServer := setUpBroker(mcpBrokerAddrFlag)
	configServer := setUpConfigServer(mcpConfigAddrFlag)
	routerGRPCServer, router := setUpRouter(mcpBroker)
	mcpConfig.RegisterObserver(router)
	mcpConfig.RegisterObserver(mcpBroker)
	// Only load config and run broker/router in standalone mode
	LoadConfig(mcpConfigFile)
	logger.Info("config: notifying observers of config change")
	mcpConfig.Notify(ctx)
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		logger.Info("mcp servers config changed ", "config file", in.Name)
		mutex.Lock()
		defer mutex.Unlock()
		LoadConfig(mcpConfigFile)
		mcpConfig.Notify(ctx)
	})
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	grpcAddr := mcpRouterAddrFlag
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("[grpc] listen error: %v", err)
	}

	go func() {
		logger.Info("[grpc] starting MCP Router", "listening", grpcAddr)
		log.Fatal(routerGRPCServer.Serve(lis))
	}()

	go func() {
		logger.Info("[http] starting MCP Broker (public)", "listening", brokerServer.Addr)
		if err := brokerServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] Cannot start public broker: %v", err)
		}
	}()

	go func() {
		logger.Info("[http] starting Config API (internal)", "listening", configServer.Addr)
		if err := configServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] Cannot start config server: %v", err)
		}
	}()

	<-stop
	// handle shutdown
	logger.Info("shutting down MCP Broker and MCP Router")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()
	if err := brokerServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}
	if err := mcpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("MCP shutdown error: %v; ignoring", err)
	}
	if err := configServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Config server shutdown error: %v", err)
	}
	routerGRPCServer.GracefulStop()
}

func setUpBroker(address string) (*http.Server, broker.MCPBroker, *server.StreamableHTTPServer) {

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "Hello, World!  BTW, the MCP server is on /mcp")
	})

	// Add OAuth protected resource endpoint
	oauthConfig := getOAuthConfig()
	mux.HandleFunc("/.well-known/oauth-protected-resource", oauthProtectedResourceHandler(oauthConfig))
	logger.Info("OAuth protected resource endpoint configured", "config", oauthConfig)

	httpSrv := &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	mcpBroker := broker.NewBroker(logger)

	streamableHTTPServer := server.NewStreamableHTTPServer(
		mcpBroker.MCPServer(),
		server.WithStreamableHTTPServer(httpSrv),
	)

	// Wrap the MCP handler with virtual server filtering
	virtualServerHandler := broker.NewVirtualServerHandler(streamableHTTPServer, mcpConfig, logger)
	mux.Handle("/mcp", virtualServerHandler)

	return httpSrv, mcpBroker, streamableHTTPServer
}

func setUpConfigServer(address string) *http.Server {
	mux := http.NewServeMux()

	authToken := os.Getenv("CONFIG_UPDATE_TOKEN")
	if authToken == "" {
		logger.Warn("CONFIG_UPDATE_TOKEN not set, config updates will be unauthenticated")
	}

	configHandler := broker.NewConfigUpdateHandler(mcpConfig, authToken, logger)
	mux.Handle("POST /config", configHandler)

	// health check endpoint for internal API
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

func setUpRouter(broker broker.MCPBroker) (*grpc.Server, *mcpRouter.ExtProcServer) {
	grpcSrv := grpc.NewServer()

	// Create the ExtProcServer instance
	server := &mcpRouter.ExtProcServer{
		RoutingConfig: mcpConfig,
		// TODO this seems wrong. Why does the router need to be passed an instance of the broker?
		Broker: broker,
	}

	// Setup the session cache with proper initialization
	server.SetupSessionCache()

	extProcV3.RegisterExternalProcessorServer(grpcSrv, server)
	return grpcSrv, server
}

// config

func LoadConfig(path string) {
	viper.SetConfigFile(path)
	logger.Debug("loading config", "path", viper.ConfigFileUsed())
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Error reading config file: %s", err)
	}
	err = viper.UnmarshalKey("servers", &mcpConfig.Servers)
	if err != nil {
		log.Fatalf("Unable to decode server config into struct: %s", err)
	}

	// Load virtualServers if present - this is optional
	if viper.IsSet("virtualServers") {
		err = viper.UnmarshalKey("virtualServers", &mcpConfig.VirtualServers)
		if err != nil {
			logger.Warn("Failed to parse virtualServers configuration", "error", err)
		}
	} else {
		logger.Debug("No virtualServers section found in configuration")
	}

	logger.Debug("config successfully loaded ")

	for _, s := range mcpConfig.Servers {
		logger.Debug(
			"server config",
			"server name",
			s.Name,
			"server prefix",
			s.ToolPrefix,
			"enabled",
			s.Enabled,
			"backend url",
			s.URL,
			"routable host",
			s.Hostname,
		)
	}
}

func runController() error {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	fmt.Println("Controller starting (health: :8081, metrics: :8082)...")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: ":8082"},
		LeaderElection:         false,
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	if err = (&controller.MCPReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	fmt.Println("Starting controller manager...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}

	return nil
}
