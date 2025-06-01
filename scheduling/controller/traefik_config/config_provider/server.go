package traefik_config

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

// The original RunServer function remains unchanged for backward compatibility
func RunServer() {
	ctx := context.Background()
	RunServerWithContext(ctx)
}

// New version with context support
func RunServerWithContext(ctx context.Context) {
	// Initialize dynamic configuration into memory for the first time
	log.Println("Dynamically initializing Traefik configuration based on BPR results...")
	initializeStaticConfig()
	log.Println("Dynamic configuration initialized/updated and loaded into memory.")

	// Define the interval for re-initializing the configuration.
	configRefreshInterval := 5 * time.Second

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

	// Start HTTP server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Starting API server with dynamic in-memory config on %s", listenAddr)
		log.Printf("Traefik should poll: http://<this-server-ip>:%s/traefik-dynamic-config", port)
		log.Printf("Configuration will be re-initialized every %v.", configRefreshInterval)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	// Start configuration refresh goroutine
	configTicker := time.NewTicker(configRefreshInterval)
	defer configTicker.Stop()

	// Main loop - handle context cancellation, config refresh, and server errors
	for {
		select {
		case <-ctx.Done():
			log.Println("Context canceled. Shutting down Traefik config server...")

			// Create shutdown context with timeout
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Gracefully shutdown the HTTP server
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("Server forced to shutdown: %v", err)
			} else {
				log.Println("Traefik config server stopped gracefully.")
			}
			return

		case err := <-serverErrors:
			log.Printf("Server error: %v", err)
			return

		case <-configTicker.C:
			log.Println("Periodically re-initializing Traefik dynamic configuration...")
			log.Println("Dynamically initializing Traefik configuration based on BPR results...")
			initializeStaticConfig() // This will call GetDomainTargets again
			log.Println("Dynamic configuration initialized/updated and loaded into memory.")
			log.Println("Traefik dynamic configuration re-initialized.")

			// BPR results polling (moved from main loop)
			log.Println("Polling BPR results...")
			// TODO: If you want to poll BPR results here as well, you can add the corresponding code
			// For example: bpr.GetAllBPRResults()
			log.Println("  No BPR results available yet.") // Placeholder, modify according to actual needs
		}
	}
}

// If you need a version that can be stopped externally
func RunServerWithShutdown() (*http.Server, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	// Setup server (same as above)
	mux := http.NewServeMux()
	mux.HandleFunc("/traefik-dynamic-config", traefikConfigHandler)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8090"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	// Start server with context in goroutine
	go RunServerWithContext(ctx)

	return server, cancel
}
