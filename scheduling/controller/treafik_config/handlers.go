package traefik_config

import (
	"encoding/json"
	"log"
	"net/http"
)

// handleTraefikConfig handles GET requests for Traefik configuration
func handleTraefikConfig(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

// handleUpdateDomainMapping handles POST requests to update domain mappings
func handleUpdateDomainMapping(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req DomainMapping
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Domain == "" || len(req.IPs) == 0 {
		http.Error(w, "Domain and IPs cannot be empty", http.StatusBadRequest)
		return
	}

	// Update domain mapping
	addDomainMapping(req.Domain, req.IPs)

	log.Printf("Domain mapping updated: %s -> %v", req.Domain, req.IPs)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Domain mapping updated successfully",
	})
}

// handleDeleteDomainMapping handles DELETE requests to remove domain mappings
func handleDeleteDomainMapping(w http.ResponseWriter, r *http.Request) {
	// Only allow DELETE requests
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get domain parameter
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "Domain parameter is required", http.StatusBadRequest)
		return
	}

	// Remove domain mapping
	success := removeDomainMapping(domain)

	// Return appropriate response
	w.Header().Set("Content-Type", "application/json")
	if success {
		log.Printf("Domain mapping deleted: %s", domain)
		json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Message: "Domain mapping deleted successfully",
		})
	} else {
		json.NewEncoder(w).Encode(APIResponse{
			Success: false,
			Message: "Domain mapping not found",
		})
	}
}

// handleListDomainMappings handles GET requests to list all domain mappings
func handleListDomainMappings(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all domain mappings
	mappings := getAllDomainMappings()

	// Return as JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Domain mappings retrieved successfully",
		Data:    mappings,
	})
}
