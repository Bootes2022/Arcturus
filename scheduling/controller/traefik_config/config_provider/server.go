package traefik_config

import (
	"log"
	"net/http"
	"os"
	"time"
)

func RunServer() {
	// Initialize dynamic configuration into memory for the first time
	initializeStaticConfig()

	// Define the interval for re-initializing the configuration.
	// For example, every 5 minutes. Adjust as needed.
	configRefreshInterval := 5 * time.Second

	// Start a goroutine to periodically re-initialize the configuration
	go func() {
		ticker := time.NewTicker(configRefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Println("Periodically re-initializing Traefik dynamic configuration...")
				initializeStaticConfig() // This will call GetDomainTargets again
				log.Println("Traefik dynamic configuration re-initialized.")
				// TODO: Add a way to stop this goroutine gracefully when the server stops,
				// e.g., by using a done channel or context cancellation.
				// This is important for production environments to prevent goroutine leaks.
			}
		}
	}()

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/traefik-dynamic-config", traefikConfigHandler) // Endpoint polled by Traefik

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8090" // Default port
	}
	listenAddr := ":" + port

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	log.Printf("Starting API server with dynamic in-memory config on %s", listenAddr)
	log.Printf("Traefik should poll: http://<this-server-ip>:%s/traefik-dynamic-config", port)
	log.Printf("Configuration will be re-initialized every %v.", configRefreshInterval) // Log the refresh interval using the variable

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}
