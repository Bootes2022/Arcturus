package main

import (
	"data/algorithms/carousel_greedy/algorithm"
	"data/algorithms/carousel_greedy/graph"
	"data/algorithms/carousel_greedy/logger"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GraphJSON JSON
type GraphJSON struct {
	Nodes  int        `json:"nodes"`
	Source int        `json:"source"`
	Sink   int        `json:"sink"`
	Edges  []EdgeJSON `json:"edges"`
}

// EdgeJSON JSON
type EdgeJSON struct {
	From     int     `json:"from"`
	To       int     `json:"to"`
	Capacity float64 `json:"capacity"`
	Latency  float64 `json:"latency"`
}

// PathResult
type PathResult struct {
	Nodes   []int     `json:"nodes"`
	Flow    float64   `json:"flow"`
	Latency []float64 `json:"latency"`
}

// CarouselResult Carousel
type CarouselResult struct {
	Parameters  map[string]interface{} `json:"parameters"`
	Performance map[string]interface{} `json:"performance"`
	Paths       []PathResult           `json:"paths"`
}

// GraphResults
type GraphResults struct {
	GraphInfo string                    `json:"graph_info"`
	Results   map[string]CarouselResult `json:"results"`
}

func main() {
	var (
		graphFile     string
		thetaA        float64
		thetaL        float64
		maxEdgeUsage  int
		alpha         int
		beta          int
		logEnabled    bool
		onlyGreedy    bool
		onlyCarousel  bool
		outputSummary bool
		summaryFile   string
		savePaths     bool
		pathsOutDir   string
	)

	flag.StringVar(&graphFile, "graph", "", "(JSON)")
	flag.Float64Var(&thetaA, "thetaA", 0.8, "(0-1)")
	flag.Float64Var(&thetaL, "thetaL", 20.0, "")
	flag.IntVar(&maxEdgeUsage, "maxEdgeUsage", 2, "")
	flag.IntVar(&alpha, "alpha", 2, "Carousel( = alpha * ||)")
	flag.IntVar(&beta, "beta", 2, "")
	flag.BoolVar(&logEnabled, "log", false, "")
	flag.BoolVar(&onlyGreedy, "greedy", false, "")
	flag.BoolVar(&onlyCarousel, "carousel", false, "Carousel")
	flag.BoolVar(&outputSummary, "summary", false, "")
	flag.StringVar(&summaryFile, "summary-file", "results_summary.csv", "")
	flag.BoolVar(&savePaths, "save-paths", true, "")
	flag.StringVar(&pathsOutDir, "paths-dir", "paths_results", "")

	flag.Parse()

	logger.Enabled = logEnabled
	logger.LogLevel = logger.INFO

	var g *graph.Graph

	if graphFile != "" {

		var err error
		g, err = loadGraphFromJSON(graphFile)
		if err != nil {
			fmt.Printf(": %v\n", err)
			return
		}
		fmt.Printf(": %s\n", graphFile)
		fmt.Printf(": =%d, =%d\n", g.Nodes, countEdges(g))
	} else {

		g = createSampleGraph()
		fmt.Println("")
	}

	fmt.Printf(": =%.2f, =%.2f, =%d, Alpha=%d, Beta=%d\n",
		thetaA, thetaL, maxEdgeUsage, alpha, beta)

	var summary struct {
		GraphFile         string
		NodeCount         int
		EdgeCount         int
		ThetaA            float64
		ThetaL            float64
		MaxEdgeUsage      int
		Alpha             int
		Beta              int
		GreedyTime        time.Duration
		GreedyPathCount   int
		GreedyTotalFlow   float64
		CarouselTime      time.Duration
		CarouselPathCount int
		CarouselTotalFlow float64
		Improvement       float64 // CarouselGreedy
	}

	summary.GraphFile = graphFile
	summary.NodeCount = g.Nodes
	summary.EdgeCount = countEdges(g)
	summary.ThetaA = thetaA
	summary.ThetaL = thetaL
	summary.MaxEdgeUsage = maxEdgeUsage
	summary.Alpha = alpha
	summary.Beta = beta

	if !onlyCarousel {

		greedySolution, greedyTime := runGreedyMFPC(g.Copy(), thetaA, thetaL, maxEdgeUsage)

		summary.GreedyTime = greedyTime
		summary.GreedyPathCount = len(greedySolution)
		summary.GreedyTotalFlow = algorithm.CalculateTotalFlow(greedySolution)
	}

	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	if !onlyGreedy {
		// Carousel Greedy
		carouselSolution, carouselTime := runCarouselGreedy(g.Copy(), thetaA, thetaL, maxEdgeUsage, alpha, beta)

		summary.CarouselTime = carouselTime
		summary.CarouselPathCount = len(carouselSolution)
		summary.CarouselTotalFlow = algorithm.CalculateTotalFlow(carouselSolution)

		if savePaths && len(carouselSolution) > 0 {
			fmt.Println("Carousel...")

			params := map[string]interface{}{
				"thetaA":       thetaA,
				"thetaL":       thetaL,
				"maxEdgeUsage": maxEdgeUsage,
				"alpha":        alpha,
				"beta":         beta,
			}

			performance := map[string]interface{}{
				"executionTime": float64(carouselTime.Milliseconds()),
				"totalFlow":     summary.CarouselTotalFlow,
				"pathCount":     len(carouselSolution),
			}

			if err := SaveCarouselPaths(g, graphFile, carouselSolution, params, performance, pathsOutDir); err != nil {
				fmt.Printf(": %v\n", err)
			}
		}
	}

	if summary.GreedyTotalFlow > 0 && summary.CarouselTotalFlow > 0 {
		summary.Improvement = (summary.CarouselTotalFlow - summary.GreedyTotalFlow) / summary.GreedyTotalFlow * 100
	}

	fmt.Println(":")

	if !onlyCarousel {
		fmt.Printf("MFPC: =%d, =%.2f, =%v\n",
			summary.GreedyPathCount, summary.GreedyTotalFlow, summary.GreedyTime)
	}
	if !onlyGreedy {
		fmt.Printf("Carousel Greedy: =%d, =%.2f, =%v\n",
			summary.CarouselPathCount, summary.CarouselTotalFlow, summary.CarouselTime)
	}
	if !onlyGreedy && !onlyCarousel {
		fmt.Printf(": %.2f%%\n", summary.Improvement)
	}

	if outputSummary {
		writeResultSummary(summary, summaryFile)
	}
}

// SaveCarouselPaths CarouselJSON
func SaveCarouselPaths(g *graph.Graph, graphPath string, solution []*graph.Path,
	params map[string]interface{}, performance map[string]interface{},
	outputDir string) error {

	graphBaseName := filepath.Base(graphPath)
	graphBaseName = strings.TrimSuffix(graphBaseName, filepath.Ext(graphBaseName))
	outputPath := filepath.Join(outputDir, graphBaseName+"_paths.json")

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf(": %v", err)
	}

	var graphResults GraphResults

	if _, err := os.Stat(outputPath); err == nil {

		data, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf(": %v", err)
		}

		if err := json.Unmarshal(data, &graphResults); err != nil {
			return fmt.Errorf(": %v", err)
		}
	} else {

		graphResults = GraphResults{
			GraphInfo: fmt.Sprintf("=%d, =%d, =%d", g.Nodes, g.Source, g.Sink),
			Results:   make(map[string]CarouselResult),
		}
	}

	paramsKey := fmt.Sprintf("thetaA%.2f_thetaL%.2f_maxEdge%d_alpha%d_beta%d",
		params["thetaA"], params["thetaL"], params["maxEdgeUsage"], params["alpha"], params["beta"])

	pathsData := make([]PathResult, len(solution))
	for i, path := range solution {

		edgeLatencies := make([]float64, 0, len(path.Nodes)-1)
		for j := 0; j < len(path.Nodes)-1; j++ {
			fromNode := path.Nodes[j]
			toNode := path.Nodes[j+1]
			if edge, exists := g.Edges[fromNode][toNode]; exists {
				edgeLatencies = append(edgeLatencies, edge.Latency)
			}
		}

		pathsData[i] = PathResult{
			Nodes:   path.Nodes,
			Flow:    path.Flow,
			Latency: edgeLatencies,
		}
	}

	graphResults.Results[paramsKey] = CarouselResult{
		Parameters:  params,
		Performance: performance,
		Paths:       pathsData,
	}

	outputData, err := json.MarshalIndent(graphResults, "", "  ")
	if err != nil {
		return fmt.Errorf(": %v", err)
	}

	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return fmt.Errorf(": %v", err)
	}

	fmt.Printf("Carousel: %s\n", outputPath)
	return nil
}

func runGreedyMFPC(g *graph.Graph, thetaA, thetaL float64, maxEdgeUsage int) ([]*graph.Path, time.Duration) {
	startTime := time.Now()

	solution := algorithm.GreedyMFPC(g, thetaA, thetaL, maxEdgeUsage)

	execTime := time.Since(startTime)

	fmt.Printf("\nMFPC %v \n", execTime)
	fmt.Printf(": %d\n", len(solution))
	totalFlow := algorithm.CalculateTotalFlow(solution)
	fmt.Printf(": %.2f\n", totalFlow)

	return solution, execTime
}

func runCarouselGreedy(g *graph.Graph, thetaA, thetaL float64, maxEdgeUsage, alpha, beta int) ([]*graph.Path, time.Duration) {
	startTime := time.Now()

	solution := algorithm.CarouselGreedy(g, thetaA, thetaL, maxEdgeUsage, alpha, beta)

	execTime := time.Since(startTime)

	fmt.Printf("\nCarousel Greedy %v \n", execTime)
	fmt.Printf(": %d\n", len(solution))
	totalFlow := algorithm.CalculateTotalFlow(solution)
	fmt.Printf(": %.2f\n", totalFlow)

	return solution, execTime
}

func countEdges(g *graph.Graph) int {
	count := 0
	for _, edges := range g.Edges {
		count += len(edges)
	}
	return count
}

func loadGraphFromJSON(filename string) (*graph.Graph, error) {

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var graphJSON GraphJSON
	if err := json.Unmarshal(data, &graphJSON); err != nil {
		return nil, err
	}

	g := graph.NewGraph(graphJSON.Nodes, graphJSON.Source, graphJSON.Sink)

	for _, edge := range graphJSON.Edges {
		g.AddEdge(edge.From, edge.To, edge.Capacity, edge.Latency)
	}

	return g, nil
}

func writeResultSummary(summary struct {
	GraphFile         string
	NodeCount         int
	EdgeCount         int
	ThetaA            float64
	ThetaL            float64
	MaxEdgeUsage      int
	Alpha             int
	Beta              int
	GreedyTime        time.Duration
	GreedyPathCount   int
	GreedyTotalFlow   float64
	CarouselTime      time.Duration
	CarouselPathCount int
	CarouselTotalFlow float64
	Improvement       float64
}, filename string) {

	fileExists := false
	if _, err := os.Stat(filename); err == nil {
		fileExists = true
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf(": %v\n", err)
		return
	}
	defer file.Close()

	if !fileExists {
		header := "GraphFile,NodeCount,EdgeCount,ThetaA,ThetaL,MaxEdgeUsage,Alpha,Beta," +
			"GreedyTime(ms),GreedyPathCount,GreedyTotalFlow," +
			"CarouselTime(ms),CarouselPathCount,CarouselTotalFlow,Improvement(%)\n"
		if _, err := file.WriteString(header); err != nil {
			fmt.Printf(": %v\n", err)
			return
		}
	}

	data := fmt.Sprintf("%s,%d,%d,%.2f,%.2f,%d,%d,%d,%.2f,%d,%.2f,%.2f,%d,%.2f,%.2f\n",
		summary.GraphFile, summary.NodeCount, summary.EdgeCount,
		summary.ThetaA, summary.ThetaL, summary.MaxEdgeUsage, summary.Alpha, summary.Beta,
		float64(summary.GreedyTime.Milliseconds()), summary.GreedyPathCount, summary.GreedyTotalFlow,
		float64(summary.CarouselTime.Milliseconds()), summary.CarouselPathCount, summary.CarouselTotalFlow,
		summary.Improvement)

	if _, err := file.WriteString(data); err != nil {
		fmt.Printf(": %v\n", err)
		return
	}

	fmt.Printf(": %s\n", filename)
}

func createSampleGraph() *graph.Graph {

	g := graph.NewGraph(8, 0, 7)

	g.AddEdge(0, 1, 10.0, 1.0)
	g.AddEdge(0, 2, 8.0, 2.0)
	g.AddEdge(0, 3, 5.0, 1.0)

	g.AddEdge(1, 4, 6.0, 2.0)
	g.AddEdge(1, 5, 4.0, 1.0)
	g.AddEdge(2, 4, 5.0, 1.0)
	g.AddEdge(2, 5, 7.0, 3.0)
	g.AddEdge(3, 5, 9.0, 2.0)
	g.AddEdge(3, 6, 4.0, 1.0)

	g.AddEdge(4, 7, 8.0, 3.0)
	g.AddEdge(5, 7, 10.0, 2.0)
	g.AddEdge(6, 7, 6.0, 1.0)

	g.AddEdge(1, 2, 3.0, 1.0)
	g.AddEdge(2, 3, 2.0, 1.0)
	g.AddEdge(4, 5, 2.0, 1.0)
	g.AddEdge(5, 6, 3.0, 1.0)

	return g
}
