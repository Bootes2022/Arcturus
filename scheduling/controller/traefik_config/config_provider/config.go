package traefik_config

import (
	"log"
	"sync"
)

// --- Traefik Dynamic Configuration Structure Definitions ---
// (These structures are exactly the same as previously discussed)

// TraefikDynamicConfiguration The top-level structure expected by Traefik
type TraefikDynamicConfiguration struct {
	HTTP *HTTPConfiguration `json:"http,omitempty"` // omitempty: do not output if HTTP is nil
}

// HTTPConfiguration Traefik HTTP configuration
type HTTPConfiguration struct {
	Routers     map[string]Router     `json:"routers"`
	Middlewares map[string]Middleware `json:"middlewares"`
	Services    map[string]Service    `json:"services"`
}

// Router Traefik router definition
type Router struct {
	Rule        string   `json:"rule"`
	Service     string   `json:"service"`
	EntryPoints []string `json:"entryPoints"`
	Middlewares []string `json:"middlewares,omitempty"`
}

// PluginMiddlewareConfig is the generic structure for plugin configurations,
// where the key is the plugin registration name
type PluginMiddlewareConfig map[string]interface{}

// Middleware Traefik middleware definition
type Middleware struct {
	Plugin PluginMiddlewareConfig `json:"plugin,omitempty"`
}

// Service Traefik service definition (noop-service)
type Service struct {
	LoadBalancer LoadBalancer `json:"loadBalancer"`
}

// LoadBalancer Traefik load balancer definition
type LoadBalancer struct {
	Servers []Server `json:"servers"`
}

// Server Traefik backend server definition
type Server struct {
	URL string `json:"url"`
}

// --- Custom Plugin Configuration Structure (myWeightedRedirector) ---
// (These structures are consistent with your plugin definition)

// WeightedRedirectorPluginConfig Corresponds to the configuration of the myWeightedRedirector plugin
type WeightedRedirectorPluginConfig struct {
	DefaultScheme        string        `json:"defaultScheme,omitempty"`
	DefaultPort          int           `json:"defaultPort,omitempty"`
	PermanentRedirect    bool          `json:"permanentRedirect,omitempty"`
	PreservePathAndQuery bool          `json:"preservePathAndQuery,omitempty"`
	Targets              []TargetEntry `json:"targets"` // Defines the target IPs and their weights
}

// TargetEntry Defines each target IP and its weight
type TargetEntry struct {
	IP     string `json:"ip"`     // Target IP address
	Weight int    `json:"weight"` // Corresponding weight
}

// --- Global Variables ---
var (
	currentTraefikConfig *TraefikDynamicConfiguration
	configLock           sync.RWMutex
	// The name under which the plugin is registered in Traefik,
	// must match experimental.localPlugins.<name> in traefik.yml.template
	pluginRegistrationName = "myWeightedRedirector"
)

// initializeStaticConfig initializes the configuration by calling GetDomainTargets
// and loads it into memory.
func initializeStaticConfig() {
	log.Println("Initializing dynamic configuration by calling GetDomainTargets...")

	// --- Get target configuration for example.com ---
	domainForExample := "example.com"
	exampleTargets := GetDomainTargets(domainForExample)
	if exampleTargets == nil {
		log.Printf("Warning: Domain '%s' not found in domainTargetsMap. Using an empty target list.", domainForExample)
		exampleTargets = []TargetEntry{}
	}

	// Plugin-specific configuration for weighted-redirect-for-example
	examplePluginSpecificConfig := WeightedRedirectorPluginConfig{
		DefaultScheme:        "http",
		DefaultPort:          50055,
		PreservePathAndQuery: false,
		PermanentRedirect:    false,
		Targets:              exampleTargets, // Use dynamically obtained Targets
	}

	// --- Get target configuration for test.com ---
	domainForTest := "test.com"
	testTargets := GetDomainTargets(domainForTest)
	if testTargets == nil {
		log.Printf("Warning: Domain '%s' not found in domainTargetsMap. Using an empty target list.", domainForTest)
		testTargets = []TargetEntry{}
	}

	// Plugin-specific configuration for weighted-redirect-for-test
	testPluginSpecificConfig := WeightedRedirectorPluginConfig{
		DefaultScheme:        "http",
		DefaultPort:          8080,
		PreservePathAndQuery: false,
		PermanentRedirect:    false,
		Targets:              testTargets, // Use dynamically obtained Targets
	}

	tdc := &TraefikDynamicConfiguration{
		HTTP: &HTTPConfiguration{
			Routers:     make(map[string]Router),
			Middlewares: make(map[string]Middleware),
			Services:    make(map[string]Service),
		},
	}

	// Add the fixed noop-service
	tdc.HTTP.Services["noop-service"] = Service{
		LoadBalancer: LoadBalancer{
			Servers: []Server{{URL: "http://127.0.0.1:1"}}, // Invalid address, will not be called
		},
	}

	// --- Configure /resolve/example.com ---
	routerNameExample := "router-for-example-path"
	middlewareNameExample := "weighted-redirect-for-example"

	tdc.HTTP.Routers[routerNameExample] = Router{
		Rule:        "Path(`/resolve/example.com`)",
		Service:     "noop-service",
		EntryPoints: []string{"web"},
		Middlewares: []string{middlewareNameExample},
	}
	tdc.HTTP.Middlewares[middlewareNameExample] = Middleware{
		Plugin: PluginMiddlewareConfig{
			pluginRegistrationName: examplePluginSpecificConfig,
		},
	}

	// --- Configure /resolve/test.com ---
	routerNameTest := "router-for-test-path"
	middlewareNameTest := "weighted-redirect-for-test"

	tdc.HTTP.Routers[routerNameTest] = Router{
		Rule:        "Path(`/resolve/test.com`)",
		Service:     "noop-service",
		EntryPoints: []string{"web"},
		Middlewares: []string{middlewareNameTest},
	}
	tdc.HTTP.Middlewares[middlewareNameTest] = Middleware{
		Plugin: PluginMiddlewareConfig{
			pluginRegistrationName: testPluginSpecificConfig,
		},
	}

	configLock.Lock()
	currentTraefikConfig = tdc
	configLock.Unlock()

	log.Println("Dynamic configuration initialized and loaded into memory.")
}
