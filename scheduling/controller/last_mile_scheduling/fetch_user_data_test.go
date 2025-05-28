package last_mile_scheduling

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"scheduling/middleware"
	"syscall"
	"testing"
)

func TestFetchUserData(t *testing.T) {
	cfg, _ := middleware.LoadConfig("../../scheduling_config.toml")
	db := middleware.ConnectToDB(cfg.Database)
	//FetchUserData(db)
	// --- Channel for receiving parameters ---
	// Use a buffered channel if you anticipate the receiver might be slower
	// than submissions, to prevent blocking HTTP handlers.
	// A buffer of 10 means it can hold 10 items before blocking the sender.
	paramsChannel := make(chan SubmittedParams, 10)

	// --- Goroutine to listen for parameters ---
	go func() {
		for params := range paramsChannel {
			fmt.Printf("[Main App] Received parameters: Domain=%s, Increment=%d, Proportion=%.2f\n",
				params.Domain, params.TotalReqIncrement, params.RedistributionProportion)
			// TODO: Do something with these parameters in your main application logic
			// For example, trigger a recalculation, update some in-memory state, etc.
		}
		fmt.Println("[Main App] Parameter channel closed.")
	}()

	// --- Start the Gin server ---
	// FetchUserData now starts the server in a goroutine and returns.
	log.Println("Starting FetchUserData web server...")
	if err := FetchUserData(db, paramsChannel); err != nil {
		log.Fatalf("Failed to start FetchUserData: %v", err)
	}
	log.Println("FetchUserData server is running in the background.")

	// --- Keep main alive / Implement graceful shutdown for the application ---
	// The web server itself (srv.ListenAndServe) has its own lifecycle.
	// This main goroutine can now do other things or just wait for a shutdown signal
	// for the entire application.
	fmt.Println("Application is running. Press Ctrl+C to exit.")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // Block until a signal is received

	log.Println("Shutting down application...")
	// Close the paramsChannel to signal the listening goroutine to stop
	close(paramsChannel)

	log.Println("Application exited.")
}
