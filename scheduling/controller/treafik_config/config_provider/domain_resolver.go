package traefik_config

// TargetEntry defines the target IP and its weight, consistent with the definition in config.go
// type TargetEntry struct {
// 	IP     string `json:"ip"`
// 	Weight int    `json:"weight"`
// }

// domainTargetsMap stores the mapping from domain names to target IPs and weights.
// In a real application, this might come from configuration, a database, or service discovery.
var domainTargetsMap = map[string][]TargetEntry{
	"example.com": {
		{IP: "192.168.1.100", Weight: 60},
		{IP: "192.168.1.101", Weight: 40},
	},
	"test.com": {
		{IP: "10.0.0.5", Weight: 50},
		{IP: "10.0.0.6", Weight: 30},
		{IP: "10.0.0.7", Weight: 20},
	},
	"api.example.com": {
		{IP: "203.0.113.10", Weight: 100},
	},
}

// GetDomainTargets returns a list of IP addresses and their weights for the provided domain.
// Returns nil if the domain is not found.
func GetDomainTargets(domain string) []TargetEntry {
	targets, ok := domainTargetsMap[domain]
	if !ok {
		return nil // Alternatively, could return an empty slice: make([]TargetEntry, 0)
	}
	return targets
}
