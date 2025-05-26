package traefik_config

import (
	"encoding/json"
	"log"
	"net/http"
)

// traefikConfigHandler Handles configuration requests from Traefik
func traefikConfigHandler(w http.ResponseWriter, r *http.Request) {
	configLock.RLock() // Use read lock to allow concurrent reads from multiple Traefik instances
	configToServe := currentTraefikConfig
	configLock.RUnlock()

	if configToServe == nil {
		log.Println("Error: Configuration not initialized or is nil!")
		// Return a minimal valid empty configuration to prevent Traefik errors
		emptyConfig := &TraefikDynamicConfiguration{
			HTTP: &HTTPConfiguration{ // Ensure the HTTP field is not nil
				Routers:     make(map[string]Router),
				Middlewares: make(map[string]Middleware),
				Services:    make(map[string]Service),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Should return 200 OK even for an empty configuration
		if err := json.NewEncoder(w).Encode(emptyConfig); err != nil {
			log.Printf("Error encoding empty configuration to JSON: %v", err)
			// Cannot write http.Error at this point as headers may already be sent
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(configToServe); err != nil {
		log.Printf("Error encoding configuration to JSON: %v", err)
		// Cannot write http.Error at this point as headers may already be sent
	}
}
