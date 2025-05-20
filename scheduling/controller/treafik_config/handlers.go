package traefik_config

import (
	"encoding/json"
	"log"
	"net/http"
)

// handleTraefikConfig handles GET requests for Traefik configuration
func handleTraefikConfig(w http.ResponseWriter, r *http.Request) {
	// Get all domain mappings
	mappings := getAllDomainMappings()

	// Generate Traefik configuration
	config := generateTraefikConfig(mappings)

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Write JSON response
	if err := json.NewEncoder(w).Encode(config); err != nil {
		log.Printf("Failed to encode Traefik config: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("Traefik configuration provided with %d domain mappings", len(mappings))
}
