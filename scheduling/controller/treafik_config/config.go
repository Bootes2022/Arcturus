package traefik_config

import (
	"strings"
)

// generateTraefikConfig creates a Traefik configuration from domain mappings
func generateTraefikConfig(mappings []DomainMapping) TraefikConfig {
	config := TraefikConfig{
		HTTP: HTTPConfig{
			Routers:     make(map[string]Router),
			Services:    make(map[string]Service),
			Middlewares: make(map[string]Middleware),
		},
	}

	// Add the global redirect middleware
	config.HTTP.Middlewares["ip-redirect"] = Middleware{
		RedirectRegex: &RedirectRegex{
			Regex:       "^https?://([^/]+)(.*)",
			Replacement: "${url}$2",
			Permanent:   false,
		},
	}

	// Create routers and services for each domain mapping
	for _, mapping := range mappings {
		routerName := sanitizeName(mapping.Domain) + "-router"
		serviceName := sanitizeName(mapping.Domain) + "-service"

		// Create the router
		config.HTTP.Routers[routerName] = Router{
			Rule:        "Host(`" + mapping.Domain + "`)",
			Service:     serviceName,
			Middlewares: []string{"ip-redirect"},
		}

		// Create the servers list
		var servers []Server
		for _, ip := range mapping.IPs {
			servers = append(servers, Server{
				URL: "http://" + ip,
			})
		}

		// Create the service with load balancer
		config.HTTP.Services[serviceName] = Service{
			LoadBalancer: &LoadBalancer{
				Servers: servers,
			},
		}
	}

	return config
}

// sanitizeName converts a domain to a valid configuration key name
func sanitizeName(domain string) string {
	return strings.ReplaceAll(domain, ".", "-")
}
