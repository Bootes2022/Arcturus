package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"scheduling/config"
	"scheduling/controller/heartbeats"
	lms "scheduling/controller/last_mile_scheduling"
	"scheduling/controller/last_mile_scheduling/bpr"
	traefik_config "scheduling/controller/traefik_config/config_provider"
	"scheduling/middleware"
	"scheduling/models"
	"sync"
	"syscall"
	"time"
)

func main() {
	// Create a cancellable root context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure resource cleanup

	// Set up signal listening (only listen in main)
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	// Create a wait group to coordinate the shutdown of all services
	var wg sync.WaitGroup

	// Start graceful shutdown handler
	go func() {
		<-shutdownSignal
		log.Println("Received shutdown signal. Initiating graceful shutdown...")
		cancel() // Cancel context to notify all services to stop
	}()

	// Prepare data - for registering domain names and IPs
	cfg, _ := middleware.LoadConfig("scheduling_config.toml")
	db := middleware.ConnectToDB(cfg.Database)

	// Get BPR parameters
	paramsChannel := make(chan lms.SubmittedParams, 10)
	if err := lms.FetchUserData(db, paramsChannel); err != nil {
		log.Fatalf("Failed to start FetchUserData: %v", err)
	}
	log.Println("FetchUserData server is running in the background.")

	// Insert domain origin data
	if err := models.InsertDomainOrigins(db, cfg.DomainOrigins); err != nil {
		log.Printf("Error during domain_origin insertion: %v", err)
	} else {
		log.Println("Domain origins processing completed.")
	}

	// Insert node region data
	if err := models.InsertNodeRegions(db, cfg.NodeRegions); err != nil {
		log.Printf("Error during node_region insertion: %v", err)
	} else {
		log.Println("Node regions processing completed.")
	}
	if cfg.DomainConfigurations != nil {
		for _, dc := range cfg.DomainConfigurations {
			err := models.SaveOrUpdateDomainConfig(db, dc.DomainName, dc.TotalReqIncrement, dc.RedistributionProportion)
			if err != nil {
				log.Printf("Error saving domain configuration for '%s' from config: %v", dc.DomainName, err)
			}
		}
	} else {
		log.Println("No DomainConfigurations found in config.")
		return
	}
	// Start heartbeats server (pass context)
	wg.Add(1)
	go func() {
		defer wg.Done()
		heartbeats.StartServer(ctx, db)
		log.Println("Heartbeats server stopped.")
	}()

	if cfg.BPRSchedulingTasks != nil && len(cfg.BPRSchedulingTasks) > 0 {
		nodesCount, errDb := models.CountMetricsNodes(db)
		if errDb != nil {
			log.Printf("Failed to count metrics nodes: %v. BPR scheduling might not start.", errDb)
		}

		if nodesCount > 0 {
			for _, task := range cfg.BPRSchedulingTasks {
				wg.Add(1)
				go func(t config.BPRSchedulingTaskConfig) {
					defer wg.Done()
					interval := time.Duration(t.IntervalSeconds) * time.Second
					if t.IntervalSeconds <= 0 {
						interval = 10 * time.Second
						log.Printf("Warning: Invalid IntervalSeconds (%d) for domain %s, region %s. Using default: %v", t.IntervalSeconds, t.DomainName, t.Region, interval)
					}
					log.Printf("[Main App] Starting BPR scheduling for Domain=%s, Region=%s, Interval=%v",
						t.DomainName, t.Region, interval)
					bpr.ScheduleBPRRuns(ctx, db, interval, t.DomainName, t.Region)
					log.Printf("BPR scheduling for domain %s, region %s stopped.", t.DomainName, t.Region)
				}(task)
			}
		} else {
			log.Println("[Main App] No metric nodes found. Skipping BPR scheduling based on config.")
		}
	} else {
		log.Println("[Main App] No BPRSchedulingTasks found in configuration. BPR scheduling will not start.")
	}

	// Start Traefik config server
	wg.Add(1)
	go func() {
		defer wg.Done()
		traefik_config.RunServerWithContext(ctx)
		log.Println("Traefik config server stopped.")
	}()

	log.Println("Application started. BPR scheduling and result polling active.")
	log.Println("Press Ctrl+C to exit gracefully.")

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("Context canceled. Waiting for all services to stop...")

	// Give services some time to shut down gracefully
	shutdownTimeout := time.NewTimer(10 * time.Second)
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All services stopped gracefully.")
	case <-shutdownTimeout.C:
		log.Println("Shutdown timeout reached. Some services may not have stopped gracefully.")
	}

	// Clean up resources
	log.Println("Closing database connection...")
	middleware.CloseDB()
	log.Println("Database connection pool closed.")
	log.Println("Shutdown complete. Exiting.")
}
