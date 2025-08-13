// main implements the CLI for the MCP broker.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "Hello, World!")
	})

	fmt.Println("Server starting on port 8080...")
	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	err := srv.ListenAndServe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not start server: %v\n", err)
	}

}
