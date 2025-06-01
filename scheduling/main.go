package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"scheduling/controller/heartbeats"
	lms "scheduling/controller/last_mile_scheduling"
	"scheduling/controller/last_mile_scheduling/bpr"
	traefik_config "scheduling/controller/traefik_config/config_provider"
	"scheduling/middleware"
	"scheduling/models"
	"syscall"
	"time"
)

func main() {
	// prepare data - Register domain name and IP usage
	cfg, _ := middleware.LoadConfig("scheduling_config.toml")
	// Connect to the database
	db := middleware.ConnectToDB(cfg.Database)
	// fetch bpr arg
	paramsChannel := make(chan lms.SubmittedParams, 10)
	//lms.FetchUserData(db)
	if err := lms.FetchUserData(db, paramsChannel); err != nil {
		log.Fatalf("Failed to start FetchUserData: %v", err)
	}
	log.Println("FetchUserData server is running in the background.")
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
	ctx := context.Background()
	heartbeats.StartServer(ctx, db)
	go func() {
		for params := range paramsChannel {
			fmt.Printf("[Main App] Received parameters: Domain=%s, Increment=%d, Proportion=%.2f\n",
				params.Domain, params.TotalReqIncrement, params.RedistributionProportion)
			nodesCount, _ := models.CountMetricsNodes(db)
			if nodesCount > 0 {
				bpr.ScheduleBPRRuns(db, 5*time.Second, params.Domain, "US-East")
			}

		}
		fmt.Println("[Main App] Parameter channel closed.")
	}()
	// --- Ticker for GetAllBPRResults and Shutdown Handling ---
	// Create a ticker that fires every 5 seconds
	resultsTicker := time.NewTicker(5 * time.Second)
	defer resultsTicker.Stop() // Ensure ticker is stopped when main exits

	// Create a channel to listen for OS signals (Ctrl+C, SIGTERM)
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	log.Println("Application started. BPR scheduling and result polling active.")
	log.Println("Press Ctrl+C to exit gracefully.")
	go traefik_config.RunServer()
	// Main loop to handle ticker events and shutdown signals
	for {
		select {
		case <-resultsTicker.C:
			log.Println("Polling BPR results...")
			allResults := bpr.GetAllBPRResults()
			if len(allResults) == 0 {
				log.Println("  No BPR results available yet.")
			} else {
				log.Printf("  Current BPR Results (%d domains):", len(allResults))
				for domain, bprMap := range allResults {
					log.Printf("    Domain: %s -> %v", domain, bprMap)
				}
			}

		case sig := <-shutdownSignal:
			log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)

			// Perform cleanup tasks
			log.Println("Closing database connection...")
			middleware.CloseDB() // Call your database closing function
			log.Println("Database connection closed.")

			// Add any other cleanup you need here

			log.Println("Shutdown complete. Exiting.")
			return // Exit the main function, terminating the program
		}
	}
}
