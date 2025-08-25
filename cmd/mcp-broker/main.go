package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	var (
		controllerMode = flag.Bool("controller", false, "Run in controller mode")
		configFile     = flag.String("config", "", "Path to broker configuration file")
		port           = flag.Int("port", 8080, "Port to listen on")
		bindAddr       = flag.String("bind", "0.0.0.0", "Address to bind to (e.g., 0.0.0.0, localhost, 127.0.0.1)")
	)
	flag.Parse()

	if *controllerMode {
		fmt.Println("Starting in controller mode...")
		go func() {
			if err := runController(); err != nil {
				fmt.Fprintf(os.Stderr, "Controller failed: %v\n", err)
				os.Exit(1)
			}
		}()
	}

	if *configFile != "" {
		fmt.Printf("Loading configuration from %s...\n", *configFile)
		// TODO: Load and apply configuration
	}

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "Hello, World!")
	})

	addr := fmt.Sprintf("%s:%d", *bindAddr, *port)
	fmt.Printf("Broker starting on %s...\n", addr)
	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	err := srv.ListenAndServe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not start server: %v\n", err)
	}
}
