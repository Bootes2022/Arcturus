package algorithm

import (
	"data/scheduling_algorithms/carousel_greedy/graph"
	"data/scheduling_algorithms/carousel_greedy/logger"
)

func GreedyMFPC(g *graph.Graph, thetaA, thetaL float64, maxEdgeUsage int) []*graph.Path {
	logger.LogSectionStart("MFPC")
	logger.Info("MFPC，: %.2f, : %.2f, : %d",
		thetaA, thetaL, maxEdgeUsage)

	paths := []*graph.Path{}
	workingGraph := g.Copy()

	pathCount := 0
	for {
		pathCount++

		pathFinder := NewPathFinder(workingGraph, thetaA, thetaL, maxEdgeUsage)
		path := pathFinder.FindPath()

		if path == nil {
			logger.Info("，")
			break
		}

		paths = append(paths, path)

		UpdateResidualGraph(workingGraph, path)
	}

	totalFlow := CalculateTotalFlow(paths)
	logger.Info("MFPC， %d ，: %.2f", len(paths), totalFlow)
	logger.LogSectionEnd("MFPC")

	return paths
}

func CalculateTotalFlow(paths []*graph.Path) float64 {
	totalFlow := 0.0
	for _, path := range paths {
		totalFlow += path.Flow
	}
	return totalFlow
}
