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

// FetchUserData sets up and runs the Gin web server to handle user input for domain configurations.
// It takes a *sql.DB connection pool as an argument.
func FetchUserData(db *sql.DB) (domainName string) {
	// Initialize Gin router with default middleware
	var d string
	router := gin.Default()

	// Load HTML templates from the 'templates' directory
	// Ensure the 'templates' directory is correctly pathed relative to your executable
	// or set an absolute path if necessary.
	router.LoadHTMLGlob("templates/*.html") // Path to your HTML templates

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

		// Simple validation and data type conversion
		// (In a real application, you would perform more robust validation)
		var totalReqIncrement int
		var redistributionProportion float64
		var err error

		if domain == "" {
			c.HTML(http.StatusBadRequest, "index.html", gin.H{"error": "Domain cannot be empty"})
			// c.String(http.StatusBadRequest, "Domain cannot be empty") // Alternative simple string response
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
		// The db instance is passed from the caller of FetchUserData
		err = models.SaveOrUpdateDomainConfig(db, domain, totalReqIncrement, redistributionProportion)
		d = domain
		if err != nil {
			log.Printf("Error saving domain configuration for '%s': %v", domain, err)
			c.HTML(http.StatusInternalServerError, "index.html", gin.H{"error": "Failed to save data to the database."})
			return
		}

		// Provide a success response back to the client
		c.HTML(http.StatusOK, "index.html", gin.H{
			"message": fmt.Sprintf("Parameters for domain '%s' successfully submitted!", domain),
		})
	})

	// Run the server on port 4433
	port := "4433"
	fmt.Printf("Server running on http://localhost:%s\n", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
	fmt.Println("domain:", d)
	return d
}
