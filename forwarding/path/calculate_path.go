package path

import (
	"data/algorithms/k_shortest"
	"data/common"
	"log"
	"sync/atomic"
	"time"
)

const (
	addPath      int = 2
	maxIteration int = 3
	hopThreshold int = 3
	theta        int = 2
)

type WeightedRoundRobin struct {
	paths       []k_shortest.PathWithIP
	cumulative  []int
	totalWeight int
	current     uint32
}

func CalculateKShortestPathsPeriodically(k int) []k_shortest.PathWithIP {

	topologyManager := common.GetInstance()

	for i := 0; i < 10; i++ {
		if topologyManager.IsInitialized() {
			break
		}
		log.Printf("... (%d/10)", i+1)
		time.Sleep(500 * time.Millisecond)
	}

	net, ipToIndex, indexToIp, err := common.GetTopologyForPath()
	if err != nil {
		log.Printf(": %v", err)
		return []k_shortest.PathWithIP{}
	}

	var validIpPaths []k_shortest.PathWithIP

	s := "104.238.153.192"
	destinationIP := "144.202.97.206"

	destinationIdx, ok := ipToIndex[destinationIP]
	sIdx, ok := ipToIndex[s]
	if !ok {
		log.Printf("IP %s ", destinationIP)
		return []k_shortest.PathWithIP{}
	}

	flow := k_shortest.Flow{Source: sIdx, Destination: destinationIdx}

	paths := k_shortest.KShortest(net, flow, k, hopThreshold, theta)

	var totalLatency int
	for _, p := range paths {
		totalLatency += p.Latency
	}

	if len(paths) == 0 || totalLatency == 0 {
		log.Println("")
		return []k_shortest.PathWithIP{}
	}

	for i, p := range paths {

		ipNodes := make([]string, len(p.Nodes))
		for j, node := range p.Nodes {
			ipNodes[j] = indexToIp[node]
		}

		weight := totalLatency / p.Latency

		validIpPaths = append(validIpPaths, k_shortest.PathWithIP{
			IPList:  ipNodes,
			Latency: p.Latency,
			Weight:  weight,
		})

		log.Printf(" %d: : %v, : %d, : %d",
			i+1, ipNodes, p.Latency, weight)
	}

	return validIpPaths
}

func NewWeightedRoundRobin(paths []k_shortest.PathWithIP) *WeightedRoundRobin {

	cumulative := make([]int, len(paths))
	total := 0
	for i, p := range paths {
		total += p.Weight
		cumulative[i] = total
	}
	return &WeightedRoundRobin{
		paths:       paths,
		cumulative:  cumulative,
		totalWeight: total,
		current:     0,
	}
}

func (w *WeightedRoundRobin) Next() k_shortest.PathWithIP {
	if w.totalWeight == 0 || len(w.paths) == 0 {
		return k_shortest.PathWithIP{} //
	}

	n := atomic.AddUint32(&w.current, 1) - 1
	mod := int(n) % w.totalWeight

	for i, c := range w.cumulative {
		if mod < c {
			return w.paths[i]
		}
	}
	return k_shortest.PathWithIP{}
}
