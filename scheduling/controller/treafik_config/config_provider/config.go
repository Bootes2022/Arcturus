package config_provider

import (
	"log"
)

// initializeStaticConfig initializes the configuration by calling GetDomainTargets
// and loads it into memory.
func initializeStaticConfig() {
	log.Println("Initializing dynamic configuration by calling GetDomainTargets...")

	// --- Get target configuration for example.com ---
	domainForExample := "example.com"
	exampleTargets := GetDomainTargets(domainForExample) // Now returns empty slice instead of nil
	if len(exampleTargets) == 0 {                        // Check for empty slice
		log.Printf("Warning: Domain '%s' has no valid targets from BPR data. Using an empty target list for its Traefik config.", domainForExample)
		// exampleTargets is already an empty slice, no need to re-assign: exampleTargets = []TargetEntry{}
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
	testTargets := GetDomainTargets(domainForTest) // Now returns empty slice instead of nil
	if len(testTargets) == 0 {                     // Check for empty slice
		log.Printf("Warning: Domain '%s' has no valid targets from BPR data. Using an empty target list for its Traefik config.", domainForTest)
		// testTargets is already an empty slice
	}

	// Plugin-specific configuration for weighted-redirect-for-test
	testPluginSpecificConfig := WeightedRedirectorPluginConfig{
		DefaultScheme:        "http",
		DefaultPort:          8080,
		PreservePathAndQuery: false,
		PermanentRedirect:    false,
		Targets:              testTargets, // Use dynamically obtained Targets
	}

	// ... (rest of the tdc configuration, Routers, Middlewares, Services setup remains the same)
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
