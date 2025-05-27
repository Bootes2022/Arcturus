package last_mile_scheduling

import (
	"log"
	"sync"
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
