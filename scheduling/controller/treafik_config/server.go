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

	// Add some initial mappings for testing
	addDomainMapping("example.com", []string{"192.168.1.100", "192.168.1.101"})
	addDomainMapping("test.com", []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})

	// Set up the HTTP routes
	http.HandleFunc("/api/traefik/config", handleTraefikConfig)
	http.HandleFunc("/api/domains/update", handleUpdateDomainMapping)
	http.HandleFunc("/api/domains/delete", handleDeleteDomainMapping)
	http.HandleFunc("/api/domains/list", handleListDomainMappings)

	// Configure the HTTP server
	addr := ":" + port
	server := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start the server
	log.Println("Starting control plane API server on", addr)
	log.Println("Traefik config available at: http://localhost" + addr + "/api/traefik/config")
	log.Fatal(server.ListenAndServe())
}
