// A simple MCP server that implements a few tools
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kagenti/mcp-gateway/internal/tests/server2"
)

func main() {
	// Choose transport based on environment
	transport := os.Getenv("MCP_TRANSPORT")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	startFunc, shutdownFunc, err := server2.RunServer(transport, port)
	if err != nil {
		fmt.Printf("Server error: %v\n", err)
		return
	}

	go func() {
		_ = startFunc()
	}()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("Shutting down server...")
	err = shutdownFunc()
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
		return
	}

	fmt.Print("Server completed\n")
}
