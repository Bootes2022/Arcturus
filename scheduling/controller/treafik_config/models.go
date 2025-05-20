package traefik_config

// TraefikConfig represents the top-level Traefik configuration
type TraefikConfig struct {
	HTTP HTTPConfig `json:"http"`
}

// HTTPConfig contains the HTTP-specific configuration
type HTTPConfig struct {
	Routers     map[string]Router     `json:"routers"`
	Services    map[string]Service    `json:"services"`
	Middlewares map[string]Middleware `json:"middlewares"`
}

// Router defines a Traefik router configuration
type Router struct {
	Rule        string   `json:"rule"`
	Service     string   `json:"service"`
	Middlewares []string `json:"middlewares,omitempty"`
	EntryPoints []string `json:"entryPoints,omitempty"`
}

// Service defines a Traefik service configuration
type Service struct {
	LoadBalancer *LoadBalancer `json:"loadBalancer,omitempty"`
}

// LoadBalancer defines the load balancer configuration for a service
type LoadBalancer struct {
	Servers []Server `json:"servers"`
}

// Server defines a backend server configuration
type Server struct {
	URL string `json:"url"`
}

// Middleware defines a Traefik middleware configuration
type Middleware struct {
	RedirectRegex *RedirectRegex `json:"redirectRegex,omitempty"`
}

// RedirectRegex defines the regex redirect middleware configuration
type RedirectRegex struct {
	Regex       string `json:"regex"`
	Replacement string `json:"replacement"`
	Permanent   bool   `json:"permanent"`
}

// DomainMapping represents a mapping between a domain and multiple IP addresses
type DomainMapping struct {
	Domain string   `json:"domain"`
	IPs    []string `json:"ips"`
}
