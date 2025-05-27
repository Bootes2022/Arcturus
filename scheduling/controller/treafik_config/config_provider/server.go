package traefik_config

import (
	"log"
	"net/http"
	"os"
	"time"
)

func RunServer() {
	// Initialize static configuration into memory
	initializeStaticConfig()

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

	log.Printf("Starting API server with static in-memory config on %s", listenAddr)
	log.Printf("Traefik should poll: http://<this-server-ip>:%s/traefik-dynamic-config", port)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}
