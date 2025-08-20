// main implements the CLI for the MCP broker.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	mcpRouter "github.com/kagenti/mcp-gateway/internal/mcp-router"
	"google.golang.org/grpc"
)

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "Hello, World!") })
	httpSrv := &http.Server{
		Addr:         getEnv("HTTP_ADDR", ":8080"),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("[http] listening on %s", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] %v", err)
		}
	}()

	grpcAddr := getEnv("SERVER_ADDRESS", "0.0.0.0:9002")
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("[grpc] listen error: %v", err)
	}
	grpcSrv := grpc.NewServer()
	extProcV3.RegisterExternalProcessorServer(grpcSrv, &mcpRouter.ExtProcServer{})

	log.Printf("[grpc] listening on %s", grpcAddr)
	log.Fatal(grpcSrv.Serve(lis))
}
