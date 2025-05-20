package traefik_config

import (
	"sync"
)

// In-memory storage for domain mappings
var (
	domainMappings     = make(map[string][]string)
	domainMappingsLock sync.RWMutex
)

// initStorage initializes the storage system
func initStorage() {
	domainMappings = make(map[string][]string)
}

// addDomainMapping adds or updates a domain mapping
func addDomainMapping(domain string, ips []string) {
	domainMappingsLock.Lock()
	defer domainMappingsLock.Unlock()
	domainMappings[domain] = ips
}

// getAllDomainMappings returns all domain mappings
func getAllDomainMappings() []DomainMapping {
	domainMappingsLock.RLock()
	defer domainMappingsLock.RUnlock()

	var mappings []DomainMapping
	for domain, ips := range domainMappings {
		mappings = append(mappings, DomainMapping{
			Domain: domain,
			IPs:    ips,
		})
	}

	return mappings
}
