// main implements the CLI for the MCP broker.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/fsnotify/fsnotify"
	config "github.com/kagenti/mcp-gateway/internal/config"
	mcpRouter "github.com/kagenti/mcp-gateway/internal/mcp-router"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

var (
	mcpConfig config.MCPServersConfig
	mutex     sync.RWMutex
)

func main() {

	var (
		mcpRouterAddrFlag string
		mcpBrokerAddrFlag string
		mcpConfigFile     string
	)
	flag.StringVar(&mcpRouterAddrFlag, "mcp-router-address", "0.0.0.0:50051", "The address for mcp router")
	flag.StringVar(&mcpBrokerAddrFlag, "mcp-broker-address", "0.0.0.0:8080", "The address for mcp broker")
	flag.StringVar(&mcpConfigFile, "mcp-gateway-config", "./config/mcp-system/config.yaml", "where to locate the mcp server config")
	flag.Parse()
	LoadConfig(mcpConfigFile)
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		log.Println("mcp servers config changed ", in.Name)
		mutex.Lock()
		defer mutex.Unlock()
		LoadConfig(mcpConfigFile)
	})
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	brokerServer := setUpBroker(mcpBrokerAddrFlag)
	routerServer := setUpRouter()

	grpcAddr := fmt.Sprintf(mcpRouterAddrFlag)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("[grpc] listen error: %v", err)
	}

	go func() {
		log.Printf("[grpc] starting MCP Router listening on %s", grpcAddr)
		log.Fatal(routerServer.Serve(lis))
	}()

	go func() {
		log.Printf("[http] starting MCP Broker listening on %s", brokerServer.Addr)
		if err := brokerServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] %v", err)
		}
	}()

	<-stop
	// handle shutdown
	log.Printf("shutting down MCP Broker and MCP Router")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()
	if err := brokerServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}
	routerServer.GracefulStop()
}

func setUpBroker(address string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "Hello, World!")
	})
	httpSrv := &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return httpSrv
}

func setUpRouter() *grpc.Server {
	grpcSrv := grpc.NewServer()
	extProcV3.RegisterExternalProcessorServer(grpcSrv, &mcpRouter.ExtProcServer{
		MCPConfig: &mcpConfig,
	})
	return grpcSrv
}

// config

func LoadConfig(path string) {
	viper.SetConfigFile(path)
	log.Println("loading congfig from path", viper.ConfigFileUsed())
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Error reading config file: %s", err)
	}
	err = viper.UnmarshalKey("servers", &mcpConfig.Servers)
	if err != nil {
		log.Fatalf("Unable to decode server config into struct: %s", err)
	}

	log.Println("config loaded ")

	for _, s := range mcpConfig.Servers {
		fmt.Println("server name", s.Name)
		fmt.Println("server host", s.Hostname)
		fmt.Println("server prefix", s.ToolPrefix)
		fmt.Println("server backend", s.URL)
	}
}
