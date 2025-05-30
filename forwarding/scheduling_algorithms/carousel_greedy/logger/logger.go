package logger

import (
	"fmt"
	"forwarding/scheduling_algorithms/carousel_greedy/graph"
	"strings"
)

var Enabled = true

const (
	INFO  = 0
	DEBUG = 1
)

var LogLevel = INFO

func Info(format string, args ...interface{}) {
	if !Enabled {
		return
	}
	fmt.Printf("Msg%s\n", fmt.Sprintf(format, args...))
}

func FormatPath(pathNodes []int, g *graph.Graph) string {
	if len(pathNodes) < 2 {
		return ""
	}

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("%d", pathNodes[0]))

	for i := 1; i < len(pathNodes); i++ {
		prev := pathNodes[i-1]
		curr := pathNodes[i]

		edge := g.Edges[prev][curr]
		builder.WriteString(fmt.Sprintf("-%.1f/%.1f->%d", edge.Flow, edge.Capacity, curr))
	}

	return builder.String()
}

func FormatResidualPath(pathNodes []int, g *graph.Graph) string {
	if len(pathNodes) < 2 {
		return ""
	}

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("%d", pathNodes[0]))

	for i := 1; i < len(pathNodes); i++ {
		prev := pathNodes[i-1]
		curr := pathNodes[i]

		edge := g.Edges[prev][curr]
		remainingCapacity := edge.Capacity - edge.Flow
		builder.WriteString(fmt.Sprintf("-%.1f->%d", remainingCapacity, curr))
	}

	return builder.String()
}

func LogPathFinding(path *graph.Path, g *graph.Graph) {
	if !Enabled {
		return
	}

	pathStr := FormatPath(path.Nodes, g)
	residualPathStr := FormatResidualPath(path.Nodes, g)

	Info(": %s", pathStr)
	Info(": %s", residualPathStr)
	Info(": %.2f, : %.2f", path.Flow, path.Latency)
}

func LogResidualGraphChange(path *graph.Path, g *graph.Graph) {
	if !Enabled {
		return
	}

	pathStr := FormatPath(path.Nodes, g)
	Info(": %s， %.2f", pathStr, path.Flow)
}

func LogBannedArc(from, to int, reason string) {
	if !Enabled {
		return
	}

	Info(": %d -> %d, : %s", from, to, reason)
}

func LogPathRemoval(path *graph.Path, reason string) {
	if !Enabled {
		return
	}

	pathStr := pathToString(path.Nodes)
	Info(": %s, : %s", pathStr, reason)
}

func pathToString(pathNodes []int) string {
	if len(pathNodes) < 2 {
		return ""
	}

	var builder strings.Builder
	for i, node := range pathNodes {
		if i > 0 {
			builder.WriteString("->")
		}
		builder.WriteString(fmt.Sprintf("%d", node))
	}

	return builder.String()
}

func LogCarouselIteration(iteration int, totalIterations int) {
	if !Enabled {
		return
	}

	Info("Carousel: %d/%d", iteration+1, totalIterations)
}

func LogSectionStart(sectionName string) {
	if !Enabled {
		return
	}
	fmt.Printf(": %s\n", sectionName)
}

func LogSectionEnd(sectionName string) {
	if !Enabled {
		return
	}

	fmt.Printf(": %s\n", sectionName)
}
