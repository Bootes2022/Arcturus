package main

import (
	"log"
	lms "scheduling/controller/last_mile_scheduling"
	b "scheduling/controller/last_mile_scheduling/bpr"
	"scheduling/middleware"
	"scheduling/models"
	"time"
)

func main() {
	// prepare data - Register domain name and IP usage
	cfg, _ := middleware.LoadConfig("scheduling_config.toml")
	// Connect to the database
	db := middleware.ConnectToDB(cfg.Database)
	// fetch bpr arg
	domainName := lms.FetchUserData(db)
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
	b.ScheduleBPRRuns(db, 5*time.Second, domainName, "es")
	middleware.CloseDB() // Assuming this closes the 'db' instance correctly
	/*ctx := context.Background()
	heartbeats.StartServer(ctx, db)*/
}
