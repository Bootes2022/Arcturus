package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Constants for CPU-request relationship
const (
	// 1-core nodes
	CPU0to60_Slope_1C      = 36.87
	CPU0to60_Intercept_1C  = 0
	CPU60to70_Slope_1C     = 35.08
	CPU60to70_Intercept_1C = 107.50
	CPU70to80_Slope_1C     = 33.43
	CPU70to80_Intercept_1C = -57.55

	// 2-core nodes
	CPU0to60_Slope_2C      = 43.61
	CPU0to60_Intercept_2C  = 0
	CPU60to70_Slope_2C     = 48.47
	CPU60to70_Intercept_2C = -291.55
	CPU70to80_Slope_2C     = 43.37
	CPU70to80_Intercept_2C = -2.06

	// CPU thresholds
	CPULowThreshold    = 60
	CPUTargetThreshold = 20
	V                  = 0.001
)

type ServerMetrics struct {
	IP              string  `json:"ip"`
	NumOfCores      int     `json:"num_of_cores"`
	CPUUsage        float64 `json:"cpu_usage"`
	RequestsHandled int64   `json:"requests_handled"`
	Latency         float64 `json:"latency,omitempty"`
	Time            string  `json:"time,omitempty"`
}

type Node struct {
	id           int
	ip           string  // IP address of the node
	reqRate      int     // Current request count (req_k^{t,in})
	onsetReq     int     // Initial request count at the beginning of slot t (req_k^{t,onset})
	dppValue     float64 // Drift-plus-penalty value (v_k^{t,dpp})
	cpuUsage     float64 // CPU usage/allocation (cpu_k^{t,in})
	queueBacklog float64 // Virtual queue backlog Q_k(t)
	delay        float64 // delay ms
	isActive     bool    // Whether the node is active
	coefficient  float64 // Coefficient C_k for redistribution
	CoreNum      int     //  1/2 -core node
}

type StatsRecord struct {
	Timestamp      string
	CPUThreshold   float64
	TotalRequests  int
	IPs            []string
	CPUUsages      []float64
	Variance       float64
	AvgLatency     float64
	ThresholdIndex int
}

// BPRClient bpr
type BPRClient struct {
	serverAddrs         []string
	serverMetrics       map[string]ServerMetrics
	nodes               []*Node
	initialRequests     int
	currentRequests     int
	requestIncrement    int
	maxRequests         int
	cpuThresholds       []float64
	client              *http.Client
	statsRecords        []StatsRecord
	mutex               sync.RWMutex
	thresholdCounts     map[int]int
	redistribProportion float64
}

// Modified NewBPRClient function to accept initial queue backlog values
func NewBPRClient(
	serverAddrs []string,
	initialRequests, requestIncrement, maxRequests int,
	cpuThresholds []float64,
	redistribProportion float64,
	initialQueueBacklogs []float64) *BPRClient {

	serverMetrics := make(map[string]ServerMetrics)
	nodes := make([]*Node, len(serverAddrs))

	for i, addr := range serverAddrs {

		var queueBacklog float64 = 0
		if i < len(initialQueueBacklogs) {
			queueBacklog = initialQueueBacklogs[i]
		}

		serverMetrics[addr] = ServerMetrics{IP: addr}
		nodes[i] = &Node{
			id:           i,
			ip:           addr,
			isActive:     true,
			coefficient:  0.0,
			queueBacklog: queueBacklog,
		}

		log.Printf(" %d (IP: %s):  %.4f", i, addr, queueBacklog)
	}

	sort.Float64s(cpuThresholds)

	return &BPRClient{
		serverAddrs:         serverAddrs,
		serverMetrics:       serverMetrics,
		nodes:               nodes,
		initialRequests:     initialRequests,
		currentRequests:     initialRequests,
		requestIncrement:    requestIncrement,
		maxRequests:         maxRequests,
		cpuThresholds:       cpuThresholds,
		redistribProportion: redistribProportion,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		statsRecords:    []StatsRecord{},
		thresholdCounts: make(map[int]int),
	}
}

// collectMetrics
func (bc *BPRClient) collectMetrics() {
	var wg sync.WaitGroup
	for i, addr := range bc.serverAddrs {
		wg.Add(1)
		go func(idx int, serverAddr string) {
			defer wg.Done()

			start := time.Now()

			url := fmt.Sprintf("http://%s/metrics", serverAddr)

			resp, err := bc.client.Get(url)
			if err != nil {
				log.Printf(" %s : %v", serverAddr, err)

				bc.mutex.Lock()
				if idx < len(bc.nodes) {
					bc.nodes[idx].isActive = false
				}
				bc.mutex.Unlock()

				return
			}
			defer resp.Body.Close()

			latency := float64(time.Since(start).Milliseconds())

			var metrics ServerMetrics
			if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
				log.Printf(" %s : %v", serverAddr, err)
				return
			}

			metrics.Latency = latency

			bc.mutex.Lock()
			bc.serverMetrics[serverAddr] = metrics

			if idx < len(bc.nodes) {
				node := bc.nodes[idx]
				node.CoreNum = metrics.NumOfCores
				node.cpuUsage = metrics.CPUUsage
				node.reqRate = int(metrics.RequestsHandled)
				fmt.Println("", metrics.RequestsHandled)
				node.onsetReq = int(metrics.RequestsHandled)
				node.delay = latency
				node.isActive = true
				node.ip = serverAddr
			}
			bc.mutex.Unlock()

			log.Printf(" %s: CPU=%.2f%%, =%d, =%d, =%.2fms",
				serverAddr, metrics.CPUUsage, metrics.NumOfCores, metrics.RequestsHandled, metrics.Latency)
		}(i, addr)
	}
	wg.Wait()
}

func (bc *BPRClient) BPRAlgorithm() map[string]int {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()

	activeNodes := []*Node{}
	for _, node := range bc.nodes {
		if node.isActive {

			if node.onsetReq == 0 {
				node.onsetReq = 10
			}
			activeNodes = append(activeNodes, node)
		}
	}

	if len(activeNodes) == 0 {
		log.Printf(": ，")
		return make(map[string]int)
	}

	totalReqIncrement := bc.currentRequests
	if totalReqIncrement < 0 {
		totalReqIncrement = 0
	}

	log.Printf(": %d, : %d, : %d", bc.currentRequests, bc.initialRequests, totalReqIncrement)

	distribution := runBPRCore(activeNodes, totalReqIncrement, bc.redistribProportion)

	for ip, count := range distribution {
		log.Printf("bpr:  %s : %d", ip, count)
	}

	return distribution
}

// Replace the current runBPRCore function in client1.go with this updated version:

func runBPRCore(nodes []*Node, totalReqIncrement int, redistributionProportion float64) map[string]int {

	nodesCopy := make([]*Node, len(nodes))
	for i, node := range nodes {

		nodeCopy := &Node{
			id:           node.id,
			ip:           node.ip,
			reqRate:      node.reqRate,
			onsetReq:     node.onsetReq,
			dppValue:     node.dppValue,
			cpuUsage:     node.cpuUsage,
			queueBacklog: node.queueBacklog,
			delay:        node.delay,
			isActive:     node.isActive,
			coefficient:  node.coefficient,
			CoreNum:      node.CoreNum,
		}
		nodesCopy[i] = nodeCopy
	}

	log.Printf("bpr，: %d, : %d", len(nodesCopy), totalReqIncrement)

	// Line 1: Initial proportional allocation of request increment
	totalOnsetReq := 0
	for _, node := range nodesCopy {
		totalOnsetReq += node.onsetReq
	}

	// Keep track of allocated requests to handle rounding
	remainingIncrement := totalReqIncrement

	// Allocate increment proportionally based on initial request rate
	for i, node := range nodesCopy {
		var increment int
		if i == len(nodesCopy)-1 {
			// Last node gets the remainder to ensure total is exactly matched
			increment = remainingIncrement
		} else if totalOnsetReq > 0 {
			// Calculate proportional share and round to nearest integer
			proportion := float64(node.onsetReq) / float64(totalOnsetReq)
			increment = int(math.Round(proportion * float64(totalReqIncrement)))
			remainingIncrement -= increment
		}
		node.reqRate = increment
	}

	// Line 2: Compute dppValue for each node
	computeDPPAndCPU(nodesCopy)

	// Step 3: Instead of using MAD to identify outliers, we just mark all nodes as active
	// This way, our main loop will directly work with the maximum DPP node
	deactivatedNodes := make(map[int]bool)
	for _, node := range nodesCopy {
		node.isActive = true
	}

	// Lines 4-14: Main iteration - redistribute requests from nodes with highest DPP
	maxIterations := 3 // Safety limit to prevent infinite loops
	iteration := 0

	for iteration < maxIterations {
		// Line 5: Find node with maximum DPP value among active nodes
		maxDPPNode := findMaxDPPNode(nodesCopy, deactivatedNodes)
		if maxDPPNode == nil {
			break
		}

		// Line 6: Remove p% of requests from maxDPPNode for redistribution
		redistributionPool := int(math.Round(float64(maxDPPNode.reqRate) * redistributionProportion))

		// Store original values for potential rollback
		originalReqRates := make(map[int]int)
		for _, node := range nodesCopy {
			originalReqRates[node.id] = node.reqRate
		}

		maxDPPNode.reqRate -= redistributionPool

		// Line 7: Redistribute the pool_manager to non-deactivated nodes except the max DPP node
		redistributeRequests(nodesCopy, maxDPPNode.id, deactivatedNodes, redistributionPool)

		// Line 8: Update DPP values and CPU usage after redistribution
		computeDPPAndCPU(nodesCopy)

		// Line 9: Check if DPP improved globally
		improved := checkGlobalDPPImprovement(nodesCopy, originalReqRates)

		if !improved {
			// Line 11-12: Rollback changes and deactivate the node
			for _, node := range nodesCopy {
				node.reqRate = originalReqRates[node.id]
			}
			deactivatedNodes[maxDPPNode.id] = true
			computeDPPAndCPU(nodesCopy) // Recompute after rollback
		}

		// Check if all nodes are deactivated
		activeNodesExist := false
		for _, node := range nodesCopy {
			if !deactivatedNodes[node.id] {
				activeNodesExist = true
				break
			}
		}

		if !activeNodesExist {
			break
		}

		iteration++
	}

	log.Println(":")
	for _, node := range nodesCopy {
		// Calculate next queue backlog according to Equation (1)
		// Qk(t+1) = max[Qk(t) + (cpu_k^t,in) - θ, 0]
		nextQueueBacklog := node.queueBacklog + node.cpuUsage - CPUTargetThreshold
		if nextQueueBacklog < 0 {
			nextQueueBacklog = 0
		}

		// Update the queue backlog
		log.Printf(" %d (IP: %s):  %.4f  %.4f",
			node.id, node.ip, node.queueBacklog, nextQueueBacklog)
		node.queueBacklog = nextQueueBacklog
	}

	distribution := make(map[string]int)
	for _, node := range nodesCopy {
		distribution[node.ip] = node.reqRate
	}

	log.Println("bpr：")
	log.Println("ID\tIP\t\t\t\t\t\tDPP\t")
	for _, node := range nodesCopy {
		log.Printf("%d\t%s\t\t%d\t%.4f\t%.4f",
			node.id, node.ip, node.reqRate, node.dppValue, node.queueBacklog)
		fmt.Printf("%d\t%s\t\t%d\t%.4f\t%.4f\n",
			node.id, node.ip, node.reqRate, node.dppValue, node.queueBacklog)
	}

	return distribution
}

func computeDPPAndCPU(nodes []*Node) {
	for _, node := range nodes {
		// 1. Get the known initial CPU value
		onsetCPU := node.cpuUsage

		// 2. Calculate estimated CPU change based on request rate change
		deltaReq := float64(node.reqRate - node.onsetReq)
		deltaCPU := 0.0

		// Apply the appropriate model based on node type and CPU range
		if node.CoreNum == 1 {
			// 1-core nodes
			if onsetCPU <= CPULowThreshold {
				// Below 60% CPU - use 0-60% model
				deltaCPU = (float64(node.reqRate)-CPU0to60_Intercept_1C)/CPU0to60_Slope_1C - onsetCPU
			} else if onsetCPU <= CPUTargetThreshold && onsetCPU > CPULowThreshold {
				// 60-70% CPU range - use 60-70% model
				deltaCPU = (float64(node.reqRate)-CPU60to70_Intercept_1C)/CPU60to70_Slope_1C - onsetCPU
			} else {
				// 70-80% CPU range (or higher) - use 70-80% model
				deltaCPU = (float64(node.reqRate)-CPU70to80_Intercept_1C)/CPU70to80_Slope_1C - onsetCPU
			}
		} else {
			// 2-core nodes
			if onsetCPU < CPULowThreshold {
				// Below 60% CPU - use 0-60% model
				deltaCPU = (float64(node.reqRate)-CPU0to60_Intercept_2C)/CPU0to60_Slope_2C - onsetCPU
			} else if onsetCPU <= CPUTargetThreshold && onsetCPU > CPULowThreshold {
				// 60-70% CPU range - use 60-70% model
				deltaCPU = (float64(node.reqRate)-CPU60to70_Intercept_2C)/CPU60to70_Slope_2C - onsetCPU
			} else {
				// 70-80% CPU range (or higher) - use 70-80% model
				deltaCPU = (float64(node.reqRate)-CPU70to80_Intercept_2C)/CPU70to80_Slope_2C - onsetCPU
			}
		}

		// Update the CPU value based on estimated change
		node.cpuUsage = onsetCPU + deltaCPU

		// 3. Update virtual queue backlog according to Equation (1)
		// Qk(t+1) = max[Qk(t) + (cpu_k^t,onset + δcpu_k^t,in) - θ, 0]
		nextQueueBacklog := node.queueBacklog + onsetCPU + deltaCPU - CPUTargetThreshold
		if nextQueueBacklog < 0 {
			nextQueueBacklog = 0
		}

		// 4. Set node weight based on CPU cores
		weight := 1.0
		if node.CoreNum == 1 {
			weight = 1.0 // 1-core weight is 1
		} else {
			weight = 0.5 // 2-core weight is 0.5 (1/2)
		}

		// 5. Calculate stability component (drift term)
		stabilityComponent := weight * node.queueBacklog * deltaCPU

		// 6. Calculate performance component (penalty term)
		performanceComponent := V * node.delay * deltaReq

		// 7. Total DPP value
		node.dppValue = stabilityComponent + performanceComponent

		// 8. Update the queue backlog for the next iteration
		//node.queueBacklog = nextQueueBacklog
	}
}

// Find node with maximum DPP value among active nodes
func findMaxDPPNode(nodes []*Node, deactivated map[int]bool) *Node {
	var maxNode *Node
	maxDPP := -math.MaxFloat64

	for _, node := range nodes {
		if !deactivated[node.id] {
			if node.dppValue > maxDPP {
				maxDPP = node.dppValue
				maxNode = node
			}
		}
	}

	return maxNode
}

// Redistribute requests to non-deactivated nodes except the specified node
func redistributeRequests(nodes []*Node, excludeNodeID int, deactivated map[int]bool, pool int) {
	// Calculate total coefficient for redistribution
	totalCoef := 0.0
	eligibleNodes := []*Node{}

	// Find eligible nodes and calculate their coefficients
	for _, node := range nodes {
		if node.id != excludeNodeID && !deactivated[node.id] {
			// Coefficient based on available CPU capacity
			node.coefficient = (100 - node.cpuUsage) * float64(node.CoreNum)
			totalCoef += node.coefficient
			eligibleNodes = append(eligibleNodes, node)
		}
	}

	// If no eligible nodes or zero total coefficient, return without redistribution
	if len(eligibleNodes) == 0 || totalCoef <= 0 {
		return
	}

	// Redistribute based on coefficients
	remainingPool := pool

	// First pass - allocate based on proportional coefficients
	for i, node := range eligibleNodes {
		var share int

		if i == len(eligibleNodes)-1 {
			// Last eligible node gets all remaining requests
			share = remainingPool
		} else {
			// Calculate proportional share
			share = int(math.Floor((node.coefficient / totalCoef) * float64(pool)))
		}

		node.reqRate += share
		remainingPool -= share
	}

	// Safety check: If we still have remaining requests, give them to the first eligible node
	if remainingPool > 0 && len(eligibleNodes) > 0 {
		eligibleNodes[0].reqRate += remainingPool
	}
}

// Check if global DPP improved after redistribution
func checkGlobalDPPImprovement(nodes []*Node, originalReqRates map[int]int) bool {
	// Store current values
	currentReqRates := make(map[int]int)
	currentDPPValues := make(map[int]float64)
	currentCPUValues := make(map[int]float64)
	currentBacklogs := make(map[int]float64)

	for _, node := range nodes {
		currentReqRates[node.id] = node.reqRate
		currentDPPValues[node.id] = node.dppValue
		currentCPUValues[node.id] = node.cpuUsage
		currentBacklogs[node.id] = node.queueBacklog
	}

	// Temporarily restore original values to calculate original DPP
	for _, node := range nodes {
		node.reqRate = originalReqRates[node.id]
	}

	// Recalculate DPP with original request rates
	computeDPPAndCPU(nodes)

	// Calculate original DPP sum
	originalDPPSum := 0.0
	for _, node := range nodes {
		originalDPPSum += node.dppValue
	}

	// Restore current values
	for _, node := range nodes {
		node.reqRate = currentReqRates[node.id]
	}

	// Recalculate DPP with current request rates
	computeDPPAndCPU(nodes)

	// Calculate new DPP sum
	newDPPSum := 0.0
	for _, node := range nodes {
		newDPPSum += node.dppValue
	}

	return newDPPSum < originalDPPSum
}

func (bc *BPRClient) sendRequests(distribution map[string]int) {
	var wg sync.WaitGroup

	for addr, count := range distribution {
		if count <= 0 {
			continue
		}

		wg.Add(1)
		go func(serverAddr string, requestCount int) {
			defer wg.Done()

			log.Printf(" %s  %d ", serverAddr, requestCount)

			interval := time.Second / time.Duration(requestCount)

			for i := 0; i < requestCount; i++ {
				go func() {
					url := fmt.Sprintf("http://%s/work?size=500000", serverAddr)
					resp, err := bc.client.Get(url)
					if err != nil {
						log.Printf(" %s : %v", serverAddr, err)
						return
					}
					defer resp.Body.Close()

					_, _ = io.Copy(io.Discard, resp.Body)
				}()

				time.Sleep(interval)
			}
		}(addr, count)
	}

	wg.Wait()
}

// calculateStats
func (bc *BPRClient) calculateStats() bool {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()

	var ips []string
	var cpuUsages []float64
	var latencies []float64

	for _, addr := range bc.serverAddrs {
		metrics, ok := bc.serverMetrics[addr]
		if !ok {
			continue
		}

		ips = append(ips, metrics.IP)
		cpuUsages = append(cpuUsages, metrics.CPUUsage)
		latencies = append(latencies, metrics.Latency)
	}

	if len(cpuUsages) == 0 {
		return false
	}

	variance := calculateVariance(cpuUsages)

	avgLatency := calculateAverage(latencies)
	log.Printf(" %.2f  %.2f ", variance, avgLatency)

	thresholdReached := false
	thresholdIndex := -1

	for i := len(bc.cpuThresholds) - 1; i >= 0; i-- {
		threshold := bc.cpuThresholds[i]
		for _, usage := range cpuUsages {
			if usage >= threshold {
				thresholdReached = true
				thresholdIndex = i
				break
			}
		}
		if thresholdReached {
			break
		}
	}

	if thresholdReached && thresholdIndex >= 0 {
		threshold := bc.cpuThresholds[thresholdIndex]

		bc.thresholdCounts[thresholdIndex]++

		record := StatsRecord{
			Timestamp:      time.Now().Format(time.RFC3339),
			CPUThreshold:   threshold,
			TotalRequests:  bc.currentRequests,
			IPs:            ips,
			CPUUsages:      cpuUsages,
			Variance:       variance,
			AvgLatency:     avgLatency,
			ThresholdIndex: thresholdIndex,
		}

		bc.statsRecords = append(bc.statsRecords, record)

		log.Printf("CPU %.2f%% ( %d) ! : %d, : %.4f, : %.2fms",
			threshold, thresholdIndex, bc.currentRequests, variance, avgLatency)

		return true
	}

	return false
}

func (bc *BPRClient) increaseRequestRate() {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()

	if bc.currentRequests < bc.maxRequests {
		newRate := bc.currentRequests + bc.requestIncrement
		if newRate > bc.maxRequests {
			newRate = bc.maxRequests
		}

		log.Printf(": %d -> %d /", bc.currentRequests, newRate)
		bc.currentRequests = newRate
	}
}

func (bc *BPRClient) saveStatsToCSV(filename string) error {
	if len(bc.statsRecords) == 0 {
		return nil
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("CSV: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"timestamp", "cpu_threshold", "threshold_index", "total_requests", "ips", "cpu_usages", "variance", "avg_latency"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("CSV: %v", err)
	}

	for _, record := range bc.statsRecords {
		ipsStr := strings.Join(record.IPs, ";")
		cpuUsagesStr := joinFloats(record.CPUUsages)

		row := []string{
			record.Timestamp,
			fmt.Sprintf("%.2f", record.CPUThreshold),
			strconv.Itoa(record.ThresholdIndex),
			strconv.Itoa(record.TotalRequests),
			ipsStr,
			cpuUsagesStr,
			fmt.Sprintf("%.4f", record.Variance),
			fmt.Sprintf("%.2f", record.AvgLatency),
		}

		if err := writer.Write(row); err != nil {
			return fmt.Errorf("CSV: %v", err)
		}
	}

	log.Printf(" %s", filename)
	return nil
}

func (bc *BPRClient) printThresholdStats() {
	log.Println("\n:")
	for i, threshold := range bc.cpuThresholds {
		count := bc.thresholdCounts[i]
		log.Printf(" %.2f%% ( %d)  %d ", threshold, i, count)
	}
}

func (bc *BPRClient) Start(duration time.Duration, rateIncreaseInterval time.Duration) error {
	log.Printf("bpr")
	log.Printf(": %d/, : %d/, : %d/",
		bc.initialRequests, bc.requestIncrement, bc.maxRequests)
	log.Printf("CPU: %v", bc.cpuThresholds)
	log.Printf(": %v, bpr: %.2f", rateIncreaseInterval, bc.redistribProportion)

	metricsTicker := time.NewTicker(1 * time.Second)
	requestTicker := time.NewTicker(1 * time.Second)
	statsTicker := time.NewTicker(1 * time.Second)
	rateIncreaseTicker := time.NewTicker(rateIncreaseInterval)

	durationTimer := time.NewTimer(duration)

	defer metricsTicker.Stop()
	defer requestTicker.Stop()
	defer statsTicker.Stop()
	defer rateIncreaseTicker.Stop()
	defer durationTimer.Stop()

	bc.collectMetrics()

	for {
		select {
		case <-metricsTicker.C:
			bc.collectMetrics()

		case <-requestTicker.C:

			distribution := bc.BPRAlgorithm()

			bc.sendRequests(distribution)

		case <-statsTicker.C:

			bc.calculateStats()

		case <-rateIncreaseTicker.C:

			bc.increaseRequestRate()

		case <-durationTimer.C:

			log.Println("，...")

			bc.printThresholdStats()

			timestamp := time.Now().Format("20060102_150405")
			filename := fmt.Sprintf("bpr_stats_%s.csv", timestamp)
			if err := bc.saveStatsToCSV(filename); err != nil {
				log.Printf(": %v", err)
			}

			return nil
		}
	}
}

func calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateVariance(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	mean := calculateAverage(values)
	sumSquaredDiff := 0.0

	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}

	return sumSquaredDiff / float64(len(values))
}

func joinFloats(values []float64) string {
	result := ""
	for i, v := range values {
		if i > 0 {
			result += ";"
		}
		result += fmt.Sprintf("%.2f", v)
	}
	return result
}

func parseFloatSlice(s string) ([]float64, error) {
	parts := strings.Split(s, ",")
	result := make([]float64, len(parts))

	for i, part := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, fmt.Errorf(" '%s' : %v", part, err)
		}
		result[i] = val
	}

	return result, nil
}

func main() {

	if len(os.Args) < 7 {
		log.Fatalf(": %s <> <> <> <CPU> <> <> <bpr> []\n", os.Args[0])
		log.Fatalf(": %s 10 5 100 \"60,70,80\" 30m 1m 0.2 \"10,15,20,0,20\"\n", os.Args[0])
	}

	initialRequests, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalf(": %v", err)
	}

	requestIncrement, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatalf(": %v", err)
	}

	maxRequests, err := strconv.Atoi(os.Args[3])
	if err != nil {
		log.Fatalf(": %v", err)
	}

	cpuThresholds, err := parseFloatSlice(os.Args[4])
	if err != nil {
		log.Fatalf("CPU: %v", err)
	}

	duration, err := time.ParseDuration(os.Args[5])
	if err != nil {
		log.Fatalf(": %v", err)
	}

	rateIncreaseInterval, err := time.ParseDuration(os.Args[6])
	if err != nil {
		log.Fatalf(": %v", err)
	}

	redistribProportion := 0.5 // bpr
	if len(os.Args) >= 8 {
		redistribProportion, err = strconv.ParseFloat(os.Args[7], 64)
		if err != nil {
			log.Fatalf("bpr: %v", err)
		}
	}

	var initialQueueBacklogs []float64
	if len(os.Args) >= 9 {
		initialQueueBacklogs, err = parseFloatSlice(os.Args[8])
		if err != nil {
			log.Fatalf(": %v", err)
		}
	}

	serverAddrs := []string{
		"64.23.249.66:8080",
		"144.202.97.206:8080",
		"149.248.11.11:8080",
		"147.182.230.15:8080",
		"104.238.153.192:8080",
	}

	if len(initialQueueBacklogs) < len(serverAddrs) {
		// ，bpr1.go
		defaultValues := []float64{10, 16, 15, 0, 20}
		for i := len(initialQueueBacklogs); i < len(serverAddrs); i++ {
			defaultIdx := i % len(defaultValues)
			initialQueueBacklogs = append(initialQueueBacklogs, defaultValues[defaultIdx])
		}
	}

	log.Printf(": %v", initialQueueBacklogs)

	bprClient := NewBPRClient(
		serverAddrs,
		initialRequests, requestIncrement, maxRequests,
		cpuThresholds,
		redistribProportion,
		initialQueueBacklogs)

	if err := bprClient.Start(duration, rateIncreaseInterval); err != nil {
		log.Fatalf("bpr: %v", err)
	}
}
