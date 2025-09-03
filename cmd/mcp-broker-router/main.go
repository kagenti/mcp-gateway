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

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = mcpv1alpha1.AddToScheme(scheme)
	_ = gatewayv1.Install(scheme)
}

func main() {

	var (
		mcpRouterAddrFlag string
		mcpBrokerAddrFlag string
		mcpConfigFile     string
		loglevel          int
		logFormat         string
		controllerMode    bool
	)
	flag.StringVar(
		&mcpRouterAddrFlag,
		"mcp-router-address",
		"0.0.0.0:50051",
		"The address for mcp router",
	)
	flag.StringVar(
		&mcpBrokerAddrFlag,
		"mcp-broker-address",
		"0.0.0.0:8080",
		"The address for mcp broker",
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
	brokerServer, broker := setUpBroker(mcpBrokerAddrFlag)
	routerGRPCServer, router := setUpRouter()
	mcpConfig.RegisterObserver(router)
	mcpConfig.RegisterObserver(broker)
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
		logger.Info("[http] starting MCP Broker", "listening", brokerServer.Addr)
		if err := brokerServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] Cannot start broker: %v", err)
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
	routerGRPCServer.GracefulStop()
}

func setUpBroker(address string) (*http.Server, broker.MCPBroker) {

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "Hello, World!  BTW, the MCP server is on /mcp")
	})
	httpSrv := &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	broker := broker.NewBroker(logger)

	streamableHTTPServer := server.NewStreamableHTTPServer(
		broker.MCPServer(),
		server.WithStreamableHTTPServer(httpSrv),
	)
	mux.Handle("/mcp", streamableHTTPServer)

	return httpSrv, broker
}

func setUpRouter() (*grpc.Server, *mcpRouter.ExtProcServer) {
	grpcSrv := grpc.NewServer()

	// Create the ExtProcServer instance
	server := &mcpRouter.ExtProcServer{
		RoutingConfig: mcpConfig,
		// TODO this seems wrong. Why does the router need to be passed an instance of the broker?
		Broker: broker.NewBroker(logger),
	}

	// Setup the session cache with proper initialization
	server.SetupSessionCache()

	extProcV3.RegisterExternalProcessorServer(grpcSrv, server)
	return grpcSrv, server
}

// config

func LoadConfig(path string) {
	viper.SetConfigFile(path)
	logger.Debug("loading congfig", "path", viper.ConfigFileUsed())
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Error reading config file: %s", err)
	}
	err = viper.UnmarshalKey("servers", &mcpConfig.Servers)
	if err != nil {
		log.Fatalf("Unable to decode server config into struct: %s", err)
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

	if err = (&controller.MCPServerReconciler{
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
