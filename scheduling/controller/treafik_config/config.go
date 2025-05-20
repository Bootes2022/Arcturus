package traefik_config

import (
	"strings"
)

func generateTraefikConfig(mappings []DomainMapping) TraefikConfig {
	config := TraefikConfig{
		HTTP: HTTPConfig{
			Routers:     make(map[string]Router),
			Services:    make(map[string]Service),
			Middlewares: make(map[string]Middleware),
		},
	}

	config.HTTP.Services["dummy-service"] = Service{
		LoadBalancer: &LoadBalancer{
			Servers: []Server{
				{URL: "http://localhost:8080"},
			},
		},
	}

	for _, mapping := range mappings {
		if len(mapping.IPs) == 0 {
			continue
		}

		targetIP := mapping.IPs[0]
		routerName := sanitizeName(mapping.Domain) + "-router"
		middlewareName := "redirect-to-" + sanitizeName(mapping.Domain)

		config.HTTP.Middlewares[middlewareName] = Middleware{
			RedirectRegex: &RedirectRegex{
				Regex:       ".*",
				Replacement: "http://" + targetIP + "/",
				Permanent:   false,
			},
		}

		config.HTTP.Routers[routerName] = Router{
			Rule:        "Path(`/resolve/" + mapping.Domain + "`)",
			Service:     "dummy-service",
			Middlewares: []string{middlewareName},
			EntryPoints: []string{"web"},
		}
	}

	return config
}

// sanitizeName converts a domain to a valid configuration key name
func sanitizeName(domain string) string {
	return strings.ReplaceAll(domain, ".", "-")
}
