package path

import (
	"data/internal/algorithms/k-shortest"
	"fmt"
	"log"
	"testing"
)

func TestPrintKShortestPaths1(t *testing.T) {

	net := k_shortest.Network{
		Nodes: []k_shortest.Node{{}, {}, {}, {}, {}, {}}, // 6 nodes
		Links: [][]int{
			//   0   1   2   3   4   5
			{0, 3, -1, 5, 7, -1},  // Node 0
			{3, 0, 2, -1, 4, -1},  // Node 1
			{-1, 2, 0, 4, -1, -1}, // Node 2
			{5, -1, 4, 0, 3, 8},   // Node 3
			{7, 4, -1, 3, 0, 6},   // Node 4
			{-1, -1, -1, 8, 6, 0}, // Node 5
		},
	}

	var allPaths []k_shortest.Path
	iterations := 0

	flow := k_shortest.Flow{Source: 0, Destination: 5}
	k := 4
	graph := k_shortest.NewGraph(6)
	paths := k_shortest.KShortest(net, flow, k, 3, 1)

	for _, path := range paths {
		graph.UpdateCapacity(path)
	}

	source, sink := flow.Source, flow.Destination
	for len(allPaths) < k && iterations < 3 {
		fmt.Println("K Shortest Paths:")
		for i, p := range paths {
			fmt.Printf("Path %d: Nodes: %v, Latency: %d\n", i+1, p.Nodes, p.Latency)
		}
		maxFlow := graph.EdmondsKarp(source, sink)
		fmt.Printf("Maximum Flow: %d\n", maxFlow)

		fmt.Println("Flow matrix:")
		for i := 0; i < graph.Nodes; i++ {
			fmt.Println(graph.Capacity[i])
		}
		if maxFlow == 1 {

			if !k_shortest.ContainsPath(allPaths, paths[0]) {
				allPaths = append(allPaths, paths[0])
			}
			if !k_shortest.ContainsPath(allPaths, paths[1]) {
				allPaths = append(allPaths, paths[1])
			}
			minCut := graph.FindMinCut(source)
			fmt.Println("Minimum Cut Edges (bottleneck edges):")
			for _, edge := range minCut {
				fmt.Printf("Edge from %d to %d with capacity %d\n", edge.From, edge.To, edge.Capacity)
				net.ChangeEdge(edge.From, edge.To)
			}

			paths = k_shortest.KShortest(net, flow, k, 3, 1)

			graph.ClearFlowMatrix()
			for _, path := range paths {
				graph.UpdateCapacity(path)
			}
			fmt.Println("Flow matrix after updating capacity:", paths)
			for i := 0; i < graph.Nodes; i++ {
				fmt.Println(graph.Flow[i])
			}
			iterations++
		} else {

			currentLen := len(allPaths)
			for i := 0; i < k-currentLen; i++ {
				if !k_shortest.ContainsPath(allPaths, paths[i]) {
					allPaths = append(allPaths, paths[i])
				}
			}
		}
	}

	fmt.Println("Final K Shortest Paths (no duplicates):")
	for i, p := range allPaths {
		fmt.Printf("Path %d: Nodes: %v, Latency: %d\n", i+1, p.Nodes, p.Latency)
	}
}

func TestNewWeightedRoundRobin(t *testing.T) {
	validIpPaths := CalculateKShortestPathsPeriodically(1)
	wr := NewWeightedRoundRobin(validIpPaths)

	path1 := wr.Next()
	path2 := wr.Next()
	path3 := wr.Next()

	fmt.Println(path1, path2, path3)
}

func TestPathManager(t *testing.T) {
	pathManager := GetInstance()
	paths := pathManager.GetPaths()

	if len(paths) > 0 {
		wrr := NewWeightedRoundRobin(paths)
		nextPath := wrr.Next()
		log.Printf(": %v, : %d", nextPath.IPList, nextPath.Latency)
	} else {
		log.Println("")
	}
}
