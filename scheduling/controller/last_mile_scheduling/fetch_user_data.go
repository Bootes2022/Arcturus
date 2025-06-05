package last_mile_scheduling

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"scheduling/models"
	"strconv"
)

// SubmittedParams holds the data structure for parameters passed out.
type SubmittedParams struct {
	Domain                   string
	TotalReqIncrement        int
	RedistributionProportion float64
}

// FetchUserData sets up and runs the Gin web server to handle user input for domain configurations.
func FetchUserData(db *sql.DB, paramsOutChan chan<- SubmittedParams) error {
	// Initialize Gin router with default middleware
	router := gin.Default()

	// Load HTML templates
	router.LoadHTMLGlob("controller/last_mile_scheduling/templates/*.html") // Ensure path is correct

	// Route to display the main form page
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// Route to handle form submission
	router.POST("/submit-params", func(c *gin.Context) {
		// Retrieve values from the form
		domain := c.PostForm("domain")
		totalReqIncrementStr := c.PostForm("totalReqIncrement")
		redistributionProportionStr := c.PostForm("redistributionProportion")

		var totalReqIncrement int
		var redistributionProportion float64
		var err error

		if domain == "" {
			c.HTML(http.StatusBadRequest, "index.html", gin.H{"error": "Domain cannot be empty"})
			return
		}

		totalReqIncrement, err = strconv.Atoi(totalReqIncrementStr)
		if err != nil || totalReqIncrement < 0 {
			c.HTML(http.StatusBadRequest, "index.html", gin.H{"error": fmt.Sprintf("Invalid Total Request Increment: %v", err)})
			return
		}

		redistributionProportion, err = strconv.ParseFloat(redistributionProportionStr, 64)
		if err != nil || redistributionProportion < 0 || redistributionProportion > 1 {
			c.HTML(http.StatusBadRequest, "index.html", gin.H{"error": fmt.Sprintf("Invalid Redistribution Proportion (must be 0.0 - 1.0): %v", err)})
			return
		}

		// Print parameters to the server console
		fmt.Println("========== Form Parameters Received ==========")
		fmt.Printf("Domain: %s\n", domain)
		fmt.Printf("Total Request Increment: %d\n", totalReqIncrement)
		fmt.Printf("Redistribution Proportion: %.2f\n", redistributionProportion)
		fmt.Println("============================================")

		// Save or update data to the database
		err = models.SaveOrUpdateDomainConfig(db, domain, totalReqIncrement, redistributionProportion)
		if err != nil {
			log.Printf("Error saving domain configuration for '%s': %v", domain, err)
			c.HTML(http.StatusInternalServerError, "index.html", gin.H{"error": "Failed to save data to the database."})
			return
		}

		// Send parameters to the output channel if it's provided
		if paramsOutChan != nil {
			params := SubmittedParams{
				Domain:                   domain,
				TotalReqIncrement:        totalReqIncrement,
				RedistributionProportion: redistributionProportion,
			}
			// Send non-blockingly or with a timeout if the receiver might be slow,
			// or just block (like here) if the receiver is expected to be ready.
			// For simplicity, we'll do a blocking send.
			// If the channel is unbuffered and no one is reading, this will block the handler.
			// Consider a buffered channel or a select with a default case for non-blocking send.
			paramsOutChan <- params
			fmt.Println("Parameters sent to output channel.")
		}

		// Provide a success response back to the client
		c.HTML(http.StatusOK, "index.html", gin.H{
			"message": fmt.Sprintf("Parameters for domain '%s' successfully submitted and processed!", domain),
		})
	})

	// Setup HTTP server with graceful shutdown
	port := "4433"
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Start server in a goroutine so it doesn't block
	go func() {
		fmt.Printf("Server starting on http://localhost:%s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	return nil // Server started successfully in a goroutine
}
