package traefik_config

import (
	"log"
	"net/http"
	"time"
)

// RunServer starts the Traefik configuration API server
func RunServer(port string) {
	// Initialize the domain mappings storage
	initStorage()

	addDomainMapping("example.com", []string{"1.92.150.161:50055"})

	// Set up only the necessary HTTP route
	http.HandleFunc("/api/traefik/config", handleTraefikConfig)

	// Configure the HTTP server
	addr := ":" + port
	server := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start the server
	log.Println("Starting Traefik config provider on", addr)
	log.Println("Traefik config available at: http://localhost" + addr + "/api/traefik/config")
	log.Fatal(server.ListenAndServe())
}
