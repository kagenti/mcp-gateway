// main implements the CLI for the MCP broker.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	goenv "github.com/caitlinelfring/go-env-default"
	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/fsnotify/fsnotify"
	"github.com/kagenti/mcp-gateway/internal/broker"
	config "github.com/kagenti/mcp-gateway/internal/config"
	mcpRouter "github.com/kagenti/mcp-gateway/internal/mcp-router"
	"github.com/kagenti/mcp-gateway/internal/session"
	mcpv1alpha1 "github.com/kagenti/mcp-gateway/pkg/apis/mcp/v1alpha1"
	"github.com/kagenti/mcp-gateway/pkg/controller"
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
)

var (
	mcpConfig = &config.MCPServersConfig{}
	mutex     sync.RWMutex
	logger    = slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheme    = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = mcpv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)
}

func main() {

	var (
		mcpRouterAddrFlag        string
		mcpBrokerAddrFlag        string
		mcpConfigAddrFlag        string
		mcpConfigFile            string
		jwtSigningKeyFlag        string
		sessionDurationInHours   int64
		loglevel                 int
		logFormat                string
		controllerMode           bool
		enforceToolFilteringFlag bool
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
	flag.StringVar(&jwtSigningKeyFlag, "session-signing-key", goenv.GetDefault("JWT_SESSION_SIGNING_KEY", "default-not-secure"), "JWT signing key for session tokens (env: JWT_SESSION_SIGNING_KEY)")
	flag.Int64Var(&sessionDurationInHours, "session-length", 24, "default session length with the gateway in hours")
	flag.BoolVar(&controllerMode, "controller", false, "Run in controller mode")
	flag.BoolVar(&enforceToolFilteringFlag, "enforce-tool-filtering", false, "when enabled an x-authorized-tools header will be needed to return any tools")
	flag.Parse()

	loggerOpts := &slog.HandlerOptions{}

	switch loglevel {
	case 0:
		loggerOpts.Level = slog.LevelInfo
	case 8:
		loggerOpts.Level = slog.LevelError
	case -4:
		loggerOpts.Level = slog.LevelDebug
	default:
		loggerOpts.Level = slog.LevelDebug
	}

	logger = slog.New(slog.NewTextHandler(os.Stdout, loggerOpts))

	if logFormat == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, loggerOpts))
	}
	if controllerMode {
		logger.Info("Starting in controller mode...")
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
	var jwtSessionMgr *session.JWTManager
	if jwtSigningKeyFlag != "" {
		jwtmgr, err := session.NewJWTManager(jwtSigningKeyFlag, sessionDurationInHours, logger)
		if err != nil {
			panic("failed to setup jwt manager " + err.Error())
		}
		jwtSessionMgr = jwtmgr
	}
	configServer := setUpConfigServer(mcpConfigAddrFlag)
	brokerServer, mcpBroker, mcpServer := setUpBroker(mcpBrokerAddrFlag, enforceToolFilteringFlag, jwtSessionMgr)
	routerGRPCServer, router := setUpRouter(mcpBroker, logger, jwtSessionMgr)
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
	lc := net.ListenConfig{}
	lis, err := lc.Listen(ctx, "tcp", grpcAddr)
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

func setUpBroker(address string, toolFiltering bool, sessionManager *session.JWTManager) (*http.Server, broker.MCPBroker, *server.StreamableHTTPServer) {

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "Hello, World!  BTW, the MCP server is on /mcp")
	})

	// Add OAuth protected resource endpoint
	oauthHandler := broker.ProtectedResourceHandler{Logger: logger}
	mux.HandleFunc("/.well-known/oauth-protected-resource", oauthHandler.Handle)

	httpSrv := &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	mcpBroker := broker.NewBroker(logger,
		broker.WithEnforceToolFilter(toolFiltering),
		broker.WithTrustedHeadersPublicKey(os.Getenv("TRUSTED_HEADER_PUBLIC_KEY")),
	)

	var streamableHTTPServer = server.NewStreamableHTTPServer(
		mcpBroker.MCPServer(),
		server.WithStreamableHTTPServer(httpSrv),
	)
	if sessionManager != nil {
		logger.Info("jwt session manager configured")
		streamableHTTPServer = server.NewStreamableHTTPServer(
			mcpBroker.MCPServer(),
			server.WithStreamableHTTPServer(httpSrv),
			server.WithSessionIdManager(sessionManager),
		)
	}
	mux.HandleFunc("/status", mcpBroker.HandleStatusRequest)
	mux.HandleFunc("/status/", mcpBroker.HandleStatusRequest)

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
	mux.HandleFunc("POST /config", configHandler.UpdateConfig)

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

func setUpRouter(broker broker.MCPBroker, logger *slog.Logger, jwtManager *session.JWTManager) (*grpc.Server, *mcpRouter.ExtProcServer) {
	grpcSrv := grpc.NewServer()

	// Create the ExtProcServer instance
	server := &mcpRouter.ExtProcServer{
		RoutingConfig: mcpConfig,
		// TODO this seems wrong. Why does the router need to be passed an instance of the broker?
		Broker:     broker,
		Logger:     logger,
		JWTManager: jwtManager,
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
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		APIReader: mgr.GetAPIReader(),
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
