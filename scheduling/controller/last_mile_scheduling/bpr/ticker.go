package bpr

import (
	"database/sql"
	"log"
	"scheduling/models"
	"sync"
	"time"
)

// NodePersistentState stores the state of a node that needs to be persisted between BPR runs.
// Now only stores QueueBacklog.
type NodePersistentState struct {
	QueueBacklog float64
}

// Global state managers
var (
	nodeStatesMap      = make(map[string]*NodePersistentState)
	nodeStatesMapMutex sync.RWMutex // Mutex to protect concurrent access to nodeStatesMap
	bprResultsCache    = make(map[string]map[string]int)
	// Mutex to protect concurrent access to bprResultsCache.
	bprResultsMutex = &sync.Mutex{}
)

// GetNodeQueueBacklog retrieves the persistent QueueBacklog for a given IP.
// It returns 0.0 if the IP is not found (default for new nodes).
func GetNodeQueueBacklog(ip string) float64 {
	nodeStatesMapMutex.RLock() // Acquire read lock
	state, found := nodeStatesMap[ip]
	nodeStatesMapMutex.RUnlock() // Release read lock

	if !found {
		log.Printf("No previous state found for IP %s, initializing QueueBacklog to 0.0.", ip)
		return 0.0 // Default for new nodes or if state not yet persisted
	}
	log.Printf("Retrieved previous QueueBacklog for IP %s: %.2f", ip, state.QueueBacklog)
	return state.QueueBacklog
}

// GetBPRResultForDomain retrieves the BPR result (map[string]int) for a specific domain.
// It returns a copy of the map to prevent modification of the cached data.
func GetBPRResultForDomain(domainName string) (map[string]int, bool) {
	bprResultsMutex.Lock()
	defer bprResultsMutex.Unlock()

	resultMap, found := bprResultsCache[domainName]
	if !found {
		return nil, false
	}

	// Return a copy of the inner map to ensure the caller cannot modify the cached map.
	// If resultMap is nil, this will correctly return a new empty (non-nil) map or nil depending on preference.
	// Here we make a new map and copy. If resultMap could be nil and you want to return nil, adjust.
	if resultMap == nil { // If BPR could return nil map on success and it was stored.
		return nil, true // Or return make(map[string]int), true if you prefer an empty map.
	}

	copiedMap := make(map[string]int, len(resultMap))
	for k, v := range resultMap {
		copiedMap[k] = v
	}
	return copiedMap, true
}

// GetAllBPRResults retrieves all stored BPR results from the cache.
// It returns a new map where keys are domainNames and values are copies of their BPR result maps.
// This is to prevent modification of the original cache or its sub-maps by the caller.
func GetAllBPRResults() map[string]map[string]int {
	bprResultsMutex.Lock()
	defer bprResultsMutex.Unlock()

	// Create a new map to hold copies of the results.
	resultsCopy := make(map[string]map[string]int, len(bprResultsCache))

	for domain, originalBprMap := range bprResultsCache {
		// For each domain, create a copy of its BPR result map.
		if originalBprMap == nil { // Handle case where a nil map might have been stored.
			resultsCopy[domain] = nil // Or an empty map: make(map[string]int)
		} else {
			copiedBprMap := make(map[string]int, len(originalBprMap))
			for k, v := range originalBprMap {
				copiedBprMap[k] = v
			}
			resultsCopy[domain] = copiedBprMap
		}
	}
	return resultsCopy
}

// UpdateNodeQueueBacklog updates the persistent QueueBacklog for a given IP.
func UpdateNodeQueueBacklog(ip string, newQueueBacklog float64) {
	nodeStatesMapMutex.Lock()         // Acquire write lock
	defer nodeStatesMapMutex.Unlock() // Release write lock

	state, found := nodeStatesMap[ip]
	if !found {
		// If state doesn't exist, create it
		state = &NodePersistentState{}
		nodeStatesMap[ip] = state
	}
	state.QueueBacklog = newQueueBacklog
	log.Printf("Updated persistent QueueBacklog for IP %s to: %.2f", ip, state.QueueBacklog)
}

// ScheduleBPRRuns schedules BPR runs and stores their map results.
func ScheduleBPRRuns(db *sql.DB, interval time.Duration, domainName string, region string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	totalReqIncrement, redistributionProportion, err := models.GetDomainConfigValues(db, domainName)
	if err != nil {
		log.Printf("Error fetching domain config for '%s': %v. BPR scheduling will not start.", domainName, err)
		return
	}

	runAndStoreBprResult := func() {
		log.Printf("Attempting BPR run for domain '%s', region '%s'...", domainName, region)
		bprResultMap, bprErr := Bpr(db, region, totalReqIncrement, redistributionProportion) // bprResultMap is map[string]int

		if bprErr != nil {
			log.Printf("Error during BPR run for domain '%s', region '%s': %v", domainName, region, bprErr)
			// Decide if you want to clear the cache for this domain on error or leave the old value.
			// For now, we just log and don't update.
			return
		}

		// Lock before accessing the shared map.
		bprResultsMutex.Lock()
		// Store the map[string]int result.
		bprResultsCache[domainName] = bprResultMap
		bprResultsMutex.Unlock()

		log.Printf("BPR run for domain '%s', region '%s' completed and result (map) stored.", domainName, region)
	}

	log.Printf("Performing initial BPR run for domain '%s', region '%s'...", domainName, region)
	runAndStoreBprResult()
	log.Printf("Initial BPR run for domain '%s', region '%s' processing done.", domainName, region)

	for {
		select {
		case <-ticker.C:
			log.Printf("Ticker triggered: Starting scheduled BPR run for domain '%s', region '%s'...", domainName, region)
			runAndStoreBprResult()
			log.Printf("Scheduled BPR run for domain '%s', region '%s' processing done.", domainName, region)
			// If config can change, re-fetch it here:
			// totalReqIncrement, redistributionProportion, err = GetDomainConfigValues(db, domainName)
			// if err != nil { ... handle error ... }
		}
	}
}
