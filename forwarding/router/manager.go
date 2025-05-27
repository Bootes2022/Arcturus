package router

import (
	"forwarding/metrics_processing/collector"
	"forwarding/metrics_processing/storage"
	"forwarding/scheduling_algorithms/k_shortest"
	"log"
	"sync"
)

type PathManager struct {
	pathChan chan []k_shortest.PathWithIP

	latestPaths []k_shortest.PathWithIP

	mu sync.RWMutex

	sourceIP      string
	destinationIP string

	k int
}

var (
	instance *PathManager
	once     sync.Once
)

// fileManagerInstance is the FileManager instance used by the current application
var (
	fileManagerInstance *storage.FileManager
	fileManagerOnce     sync.Once
)

// getFileManager retrieves the singleton instance of FileManager
func getFileManager() *storage.FileManager {
	fileManagerOnce.Do(func() {
		var err error
		fileManagerInstance, err = storage.NewFileManager("./config")
		if err != nil {
			log.Printf("Failed to create FileManager: %v", err)
			return
		}
	})
	return fileManagerInstance
}

// GetTargetIPByDomain looks up the corresponding target IP from the domain mapping
func GetTargetIPByDomain(domain string) string {
	// Get the FileManager instance
	fileManager := getFileManager()
	if fileManager == nil {
		log.Printf("Failed to retrieve FileManager instance")
		return ""
	}

	// Get all domain mappings
	mappings := fileManager.GetDomainIPMappings()
	if mappings == nil {
		log.Printf("Domain mappings are empty")
		return ""
	}

	// Find the matching domain
	for _, mapping := range mappings {
		if mapping.Domain == domain {
			log.Printf("Found mapped IP for domain %s: %s", domain, mapping.Ip)
			return mapping.Ip
		}
	}

	// Return empty string if no mapping is found
	log.Printf("No mapping found for domain %s", domain)
	return ""
}

// GetDefaultTargetIP retrieves the default target IP
// Returns the IP of the first record in the domain mapping
// Returns an empty string if there are no mapping records
func GetDefaultTargetIP() string {
	// Get the FileManager instance
	fileManager := getFileManager()
	if fileManager == nil {
		log.Printf("Failed to retrieve FileManager instance")
		return ""
	}

	// Get all domain mappings
	mappings := fileManager.GetDomainIPMappings()

	// If there are mapping records, return the IP of the first record
	if mappings != nil && len(mappings) > 0 {
		defaultIP := mappings[0].Ip
		log.Printf("Using IP from the first domain mapping: %s as default target", defaultIP)
		return defaultIP
	}

	// Return empty string if there are no mapping records
	log.Printf("Domain mappings are empty, no default target IP available")
	return ""
}

func GetInstance() *PathManager {
	once.Do(func() {
		ip, err := collector.GetIP()
		if err != nil {
			return
		}
		// Use GetDefaultTargetIP to get the default target IP
		defaultTargetIP := GetDefaultTargetIP()
		instance = &PathManager{
			pathChan:      make(chan []k_shortest.PathWithIP, 1),
			latestPaths:   nil,
			sourceIP:      ip,
			destinationIP: defaultTargetIP,
			k:             2,
		}

		go instance.pathListener()
		log.Println("")
	})
	return instance
}

func (pm *PathManager) pathListener() {
	for paths := range pm.pathChan {
		pm.mu.Lock()
		pm.latestPaths = paths
		pm.mu.Unlock()
		log.Printf(" %d ", len(paths))
	}
}

func (pm *PathManager) CalculatePaths(network k_shortest.Network,
	ipToIndex map[string]int,
	indexToIP map[int]string) {

	sourceIdx, srcExists := ipToIndex[pm.sourceIP]
	destIdx, destExists := ipToIndex[pm.destinationIP]

	if !srcExists || !destExists {
		log.Printf(": IPIP (: %v, : %v)",
			srcExists, destExists)
		return
	}

	flow := k_shortest.Flow{Source: sourceIdx, Destination: destIdx}

	paths := k_shortest.KShortest(network, flow, pm.k, 3, 2) // theta

	var pathsWithIP []k_shortest.PathWithIP

	totalLatency := 0
	for _, p := range paths {
		totalLatency += p.Latency
	}

	if len(paths) == 0 || totalLatency == 0 {
		log.Println("")

		select {
		case pm.pathChan <- []k_shortest.PathWithIP{}:

		default:

		}
		return
	}

	for _, p := range paths {

		ipNodes := make([]string, len(p.Nodes))
		for j, node := range p.Nodes {
			ipNodes[j] = indexToIP[node]
		}

		var weight int
		if p.Latency == 0 {

			weight = 100
			log.Printf(": ，: %d", weight)
		} else {
			weight = totalLatency / p.Latency
		}

		pathsWithIP = append(pathsWithIP, k_shortest.PathWithIP{
			IPList:  ipNodes,
			Latency: p.Latency,
			Weight:  weight,
		})

		log.Printf(": : %v, : %d, : %d", ipNodes, p.Latency, weight)
	}

	select {
	case pm.pathChan <- pathsWithIP:

	default:

		<-pm.pathChan
		pm.pathChan <- pathsWithIP
	}
}

func (pm *PathManager) GetPaths() []k_shortest.PathWithIP {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.latestPaths == nil {
		return []k_shortest.PathWithIP{}
	}

	paths := make([]k_shortest.PathWithIP, len(pm.latestPaths))
	copy(paths, pm.latestPaths)
	return paths
}

func (pm *PathManager) SetSourceDestination(source, destination string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.sourceIP = source
	pm.destinationIP = destination
}
