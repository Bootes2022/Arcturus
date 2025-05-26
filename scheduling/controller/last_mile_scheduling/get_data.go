package last_mile_scheduling

import (
	"database/sql"
	"log"
	"math/rand"
	"scheduling/models"
	"sort"
	"sync"
	"time"
)

// IPWeight represents an IP address and its assigned weight.
type IPWeight struct {
	IP     string `json:"ip"`
	Weight int    `json:"weight"`
}

// DomainIPWeights holds the domain and its associated list of IP weights.
type DomainIPWeights struct {
	Domain string     `json:"domain"`
	Nodes  []IPWeight `json:"nodes"`
}

// WeightService manages the calculation and retrieval of domain IP weights.
type WeightService struct {
	db             *sql.DB
	updateInterval time.Duration
	mu             sync.RWMutex // To protect access to latestWeights
	latestWeights  *DomainIPWeights
	stopChan       chan struct{} // To signal the updater goroutine to stop
}

// CalculateRandomWeights (from previous response, can be moved to a util package or kept here)
// For brevity, I'll assume it's defined as before.
// ... (paste CalculateRandomWeights function here or import it) ...
func CalculateRandomWeights(ips []string) map[string]int {
	weights := make(map[string]int)
	numIPs := len(ips)

	if numIPs == 0 {
		return weights
	}
	if numIPs == 1 {
		weights[ips[0]] = 100
		return weights
	}
	splitPoints := make([]int, numIPs-1)
	for i := 0; i < numIPs-1; {
		p := rand.Intn(99) + 1
		isUnique := true
		for j := 0; j < i; j++ {
			if splitPoints[j] == p {
				isUnique = false
				break
			}
		}
		if isUnique {
			splitPoints[i] = p
			i++
		}
	}
	sort.Ints(splitPoints)
	fullPoints := make([]int, numIPs+1)
	fullPoints[0] = 0
	copy(fullPoints[1:], splitPoints)
	fullPoints[numIPs] = 100
	currentWeights := make([]int, numIPs)
	currentSum := 0
	for i := 0; i < numIPs; i++ {
		weight := fullPoints[i+1] - fullPoints[i]
		currentWeights[i] = weight
		currentSum += weight
	}
	if currentSum != 100 && numIPs > 0 {
		log.Printf("Warning: Calculated weights sum to %d, adjusting last weight.", currentSum)
		diff := 100 - currentSum
		for i := len(currentWeights) - 1; i >= 0; i-- {
			if currentWeights[i]+diff >= 0 {
				currentWeights[i] += diff
				break
			}
		}
	}
	shuffledIPs := make([]string, numIPs)
	copy(shuffledIPs, ips)
	rand.Shuffle(len(shuffledIPs), func(i, j int) {
		shuffledIPs[i], shuffledIPs[j] = shuffledIPs[j], shuffledIPs[i]
	})
	for i := 0; i < numIPs; i++ {
		weights[shuffledIPs[i]] = currentWeights[i]
	}
	return weights
}

// NewWeightService creates and starts a new WeightService.
func NewWeightService(db *sql.DB, updateInterval time.Duration) *WeightService {
	service := &WeightService{
		db:             db,
		updateInterval: updateInterval,
		latestWeights:  &DomainIPWeights{Nodes: []IPWeight{}}, // Initialize with empty
		stopChan:       make(chan struct{}),
	}
	// Perform an initial calculation
	service.updateWeights()
	// Start the periodic updater
	go service.updater()
	return service
}

// updater is the goroutine that periodically updates the weights.
func (s *WeightService) updater() {
	ticker := time.NewTicker(s.updateInterval)
	defer ticker.Stop()

	log.Println("WeightService updater started.")
	for {
		select {
		case <-ticker.C:
			s.updateWeights()
		case <-s.stopChan:
			log.Println("WeightService updater stopping.")
			return
		}
	}
}

// updateWeights fetches data and recalculates weights.
func (s *WeightService) updateWeights() {
	log.Println("WeightService: Updating IP weights...")

	// 1. Fetch the first domain (or the specific domain you care about)
	domain, err := models.FetchFirstDomain(s.db)
	if err != nil {
		log.Printf("WeightService: Error fetching domain: %v. Skipping update.", err)
		return
	}
	if domain == "" { // Should be caught by FetchFirstDomain's error, but good to check
		log.Println("WeightService: No domain found. Skipping update.")
		return
	}

	// 2. Fetch all IPs from node_region (or IPs specific to the fetched domain if you had that mapping)
	ips, err := models.FetchAllNodeIPs(s.db)
	if err != nil {
		log.Printf("WeightService: Error fetching node IPs: %v. Skipping update.", err)
		return
	}

	if len(ips) == 0 {
		log.Printf("WeightService: No node IPs found. Clearing weights for domain %s.", domain)
		s.mu.Lock()
		s.latestWeights = &DomainIPWeights{Domain: domain, Nodes: []IPWeight{}}
		s.mu.Unlock()
		return
	}

	// 3. Calculate random weights for these IPs
	rawWeights := CalculateRandomWeights(ips) // Assuming CalculateRandomWeights is accessible

	// 4. Construct the DomainIPWeights structure
	newDomainWeights := &DomainIPWeights{
		Domain: domain,
		Nodes:  make([]IPWeight, 0, len(rawWeights)),
	}
	totalWeightCheck := 0
	for ip, weight := range rawWeights {
		newDomainWeights.Nodes = append(newDomainWeights.Nodes, IPWeight{IP: ip, Weight: weight})
		totalWeightCheck += weight
	}
	if totalWeightCheck != 100 && len(ips) > 0 {
		log.Printf("WeightService CRITICAL WARNING: Total weight for %s is %d, not 100!", domain, totalWeightCheck)
	}

	// 5. Atomically update the latestWeights
	s.mu.Lock()
	s.latestWeights = newDomainWeights
	s.mu.Unlock()

	log.Printf("WeightService: Successfully updated weights for domain '%s'. %d nodes.", domain, len(newDomainWeights.Nodes))
}

// GetLatestWeights returns the most recently calculated domain IP weights.
// The returned pointer should be treated as read-only by the caller.
func (s *WeightService) GetLatestWeights() *DomainIPWeights {
	s.mu.RLock() // Use RLock for read access
	defer s.mu.RUnlock()
	// Return a copy to prevent external modification of the internal slice if necessary,
	// but for a simple struct like this, returning the pointer is often fine if callers respect read-only.
	// For true immutability, you'd deep copy.
	// For this example, we return the direct pointer for efficiency.
	return s.latestWeights
}

// Stop gracefully shuts down the WeightService updater.
func (s *WeightService) Stop() {
	log.Println("WeightService: Signalling updater to stop...")
	close(s.stopChan) // Signal the updater to stop
}
