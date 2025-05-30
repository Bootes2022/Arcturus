package traefik_config

import (
	"control/controller/last_mile_scheduling/bpr"
	"log"
	"strings"
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
	log.Println("Dynamically initializing Traefik configuration based on BPR results...")

	allBprData := bpr.GetAllBPRResults() // Get all current BPR results

	tdc := &TraefikDynamicConfiguration{
		HTTP: &HTTPConfiguration{
			Routers:     make(map[string]Router),
			Middlewares: make(map[string]Middleware),
			Services:    make(map[string]Service),
		},
	}

	hasActiveRouters := false // Flag indicating whether at least one router was created

	for domain, _ := range allBprData {
		targets := GetDomainTargets(domain) // GetDomainTargets internally retrieves from allBprData again (could be optimized)

		if len(targets) > 0 { // Only create configuration if domain has valid targets
			hasActiveRouters = true
			safeDomainNamePart := strings.ReplaceAll(domain, ".", "-") // Create safe name component
			routerName := "router-resolve-" + safeDomainNamePart
			middlewareName := "weighted-redirect-" + safeDomainNamePart
			rule := "Path(`/resolve/" + domain + "`)" // Dynamically generate rule

			pluginSpecificConfig := WeightedRedirectorPluginConfig{
				DefaultScheme: "http", // These could be defaults or read from more general configuration
				DefaultPort:   50055,  // These could be defaults or read from more general configuration
				Targets:       targets,
			}

			tdc.HTTP.Routers[routerName] = Router{
				Rule:        rule,
				Service:     "noop-service",
				EntryPoints: []string{"web"},
				Middlewares: []string{middlewareName},
			}
			tdc.HTTP.Middlewares[middlewareName] = Middleware{
				Plugin: PluginMiddlewareConfig{
					pluginRegistrationName: pluginSpecificConfig,
				},
			}
		} else {
			log.Printf("initializeStaticConfig: No valid targets for domain '%s' from BPR results. Skipping Traefik config for it.", domain)
		}
	}

	// Only add noop-service if at least one router was created
	if hasActiveRouters {
		tdc.HTTP.Services["noop-service"] = Service{
			LoadBalancer: LoadBalancer{
				Servers: []Server{{URL: "http://127.0.0.1:1"}},
			},
		}
	}

	configLock.Lock()
	currentTraefikConfig = tdc
	configLock.Unlock()

	log.Println("Dynamic configuration initialized/updated and loaded into memory.")
}
