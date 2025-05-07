package path

import (
	"data/algorithms/k_shortest"
	"data/metric/collector"
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

func GetInstance() *PathManager {
	once.Do(func() {
		ip, err := collector.GetIP()
		if err != nil {
			return
		}

		instance = &PathManager{
			pathChan:      make(chan []k_shortest.PathWithIP, 1),
			latestPaths:   nil,
			sourceIP:      ip,
			destinationIP: "143.198.130.37",
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
