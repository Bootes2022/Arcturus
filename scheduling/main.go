package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	lms "scheduling/controller/last_mile_scheduling"
	"scheduling/middleware"
	"scheduling/models"
	"sync"
	"syscall"
	"time"
)

var (
	weightSvc         *lms.WeightService
	weightServiceOnce sync.Once    // To ensure weightSvc is initialized only once
	isWeightSvcReady  bool         // Flag to indicate if service is initialized
	svcMutex          sync.RWMutex // To protect isWeightSvcReady
)

// GetCurrentDomainWeights provides direct function access to the latest weights.
// This is the function your proxy/other Go code would call.
func GetCurrentDomainWeights() (*lms.DomainIPWeights, error) {
	svcMutex.RLock()
	ready := isWeightSvcReady
	svcMutex.RUnlock()

	if !ready || weightSvc == nil {
		return nil, fmt.Errorf("WeightService is not yet initialized or ready")
	}

	latestWeights := weightSvc.GetLatestWeights()
	if latestWeights == nil || latestWeights.Domain == "" {
		// This could happen if the first update hasn't completed or found no data
		return nil, fmt.Errorf("no weight data available yet or no domain found from WeightService")
	}
	return latestWeights, nil
}

// handleProxyRequest simulates a proxy calling GetCurrentDomainWeights
func handleProxyRequest() {
	weights, err := GetCurrentDomainWeights()
	if err != nil {
		log.Printf("Proxy Request : Could not get current weights: %v", err)
		return
	}

	if weights == nil { // Should be caught by error handling in GetCurrentDomainWeights
		return
	}

	if len(weights.Nodes) == 0 {
		log.Printf("Proxy Request: No nodes found for domain '%s'.", weights.Domain)
	}
	log.Println(weights)
}
func main() {
	rand.Seed(time.Now().UnixNano())
	// prepare data - Register domain name and IP usage
	cfg, _ := middleware.LoadConfig()
	// Connect to the database
	db := middleware.ConnectToDB(cfg.Database)

	// Insert data into domain_origin
	if err := models.InsertDomainOrigins(db, cfg.DomainOrigins); err != nil {
		log.Printf("Error during domain_origin insertion: %v", err)
		// Decide if you want to stop or continue
	} else {
		log.Println("Domain origins processing completed.")
	}
	// Insert data into node_region
	if err := models.InsertNodeRegions(db, cfg.NodeRegions); err != nil {
		log.Printf("Error during node_region insertion: %v", err)
		// Decide if you want to stop or continue
	} else {
		log.Println("Node regions processing completed.")
	}
	// --- Initialize and start the WeightService ---
	updateInterval := 10 * time.Second
	weightServiceOnce.Do(func() {
		weightSvc = lms.NewWeightService(db, updateInterval)
		svcMutex.Lock()
		isWeightSvcReady = true
		svcMutex.Unlock()
		log.Println("WeightService initialized and started.")
	})

	// --- Example of how your proxy or other Go code might use it ---
	// This could be in a different goroutine or part of your proxy's request handling logic
	go func() {
		// Wait a bit for the first update to potentially complete
		time.Sleep(5 * time.Second) // Adjust as needed, or use a more sophisticated readiness signal

		retrievedWeights, err := GetCurrentDomainWeights()
		if err != nil {
			log.Printf("PROXY_SIMULATION: Error getting weights: %v", err)
		} else {
			log.Printf("PROXY_SIMULATION: Successfully retrieved weights for domain '%s': %d nodes",
				retrievedWeights.Domain, len(retrievedWeights.Nodes))
			for _, node := range retrievedWeights.Nodes {
				log.Printf("  PROXY_SIMULATION: IP: %s, Weight: %d", node.IP, node.Weight)
			}
		}
	}()
	// --- Simulate proxy requests periodically ---
	proxyRequestInterval := 7 * time.Second // Proxy checks every 7 seconds
	proxyTicker := time.NewTicker(proxyRequestInterval)
	defer proxyTicker.Stop()

	// Perform one immediate proxy request simulation after a short delay for WeightService's first run
	go func() {
		// Wait slightly longer than the WeightService's first potential update cycle
		// NewWeightService calls updateWeights() synchronously once.
		time.Sleep(1 * time.Second)
		handleProxyRequest() // Initial "proxy" call
	}()

	// Goroutine to handle periodic proxy requests
	doneProxySimulation := make(chan bool) // To stop this goroutine on shutdown
	go func() {
		for {
			select {
			case <-proxyTicker.C:
				handleProxyRequest()
			case <-doneProxySimulation:
				log.Println("Proxy simulation ticker stopped.")
				return
			}
		}
	}()

	log.Println("Application is running. Simulating proxy requests. Press Ctrl+C to exit.")
	// --- Wait for shutdown signal ---
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan // Block until a signal is received

	log.Println("Shutdown signal received, initiating graceful shutdown...")

	// Signal proxy simulation to stop
	close(doneProxySimulation)
	// Stop the WeightService
	svcMutex.RLock() // Ensure weightSvc is read safely
	localWeightSvc := weightSvc
	svcMutex.RUnlock()
	if localWeightSvc != nil {
		localWeightSvc.Stop()
		log.Println("WeightService stopped.")
	}

	// Close Database
	middleware.CloseDB() // Assuming this closes the 'db' instance correctly
	log.Println("Database connection closed.")

	log.Println("Application exited.")
	// Add a small delay to allow background goroutines' logs to flush before main exits fully
	time.Sleep(100 * time.Millisecond)
	// run server
	/*ctx := context.Background()
	heartbeats.StartServer(ctx, db)*/
}
