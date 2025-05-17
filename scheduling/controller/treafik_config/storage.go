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
	// In a production environment, this would initialize a database connection
	domainMappings = make(map[string][]string)
}

// addDomainMapping adds or updates a domain mapping
func addDomainMapping(domain string, ips []string) {
	domainMappingsLock.Lock()
	defer domainMappingsLock.Unlock()

	domainMappings[domain] = ips
}

// removeDomainMapping removes a domain mapping
func removeDomainMapping(domain string) bool {
	domainMappingsLock.Lock()
	defer domainMappingsLock.Unlock()

	_, exists := domainMappings[domain]
	if exists {
		delete(domainMappings, domain)
		return true
	}
	return false
}

// getDomainMapping retrieves a domain mapping by domain name
func getDomainMapping(domain string) ([]string, bool) {
	domainMappingsLock.RLock()
	defer domainMappingsLock.RUnlock()

	ips, exists := domainMappings[domain]
	return ips, exists
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
