package bpr

import (
	"database/sql"
	"log"
	"math"
	"math/rand"
	"scheduling/models"
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
	CPUTargetThreshold = 40
	V                  = 0.01
)

// Node represents a computing node in the system
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

// BPR Algorithm implementation - Modified to use Max DPP instead of MAD
func BPRAlgorithm(nodes []*Node, totalReqIncrement int, redistributionProportion float64) map[string]int {
	// Line 1: Initial proportional allocation of request increment
	totalOnsetReq := 0
	for _, node := range nodes {
		totalOnsetReq += node.onsetReq
	}

	remainingIncrement := totalReqIncrement
	for i, node := range nodes {
		var increment int
		if i == len(nodes)-1 {
			increment = remainingIncrement
		} else if totalOnsetReq > 0 {
			proportion := float64(node.onsetReq) / float64(totalOnsetReq)
			increment = int(math.Round(proportion * float64(totalReqIncrement)))
			remainingIncrement -= increment
		}
		node.reqRate = node.onsetReq + increment
	}

	// Line 2: Compute dppValue for each node
	computeDPPAndCPU(nodes)

	// Step 3: Mark all nodes as active
	deactivatedNodes := make(map[int]bool)
	for _, node := range nodes {
		node.isActive = true
	}

	// Lines 4-14: Main iteration
	maxIterations := 3
	iteration := 0

	for iteration < maxIterations {
		// Line 5: Find node with maximum DPP value
		maxDPPNode := findMaxDPPNode(nodes, deactivatedNodes)
		if maxDPPNode == nil {
			break
		}

		// Line 6: Remove p% of requests from maxDPPNode
		redistributionPool := int(math.Round(float64(maxDPPNode.reqRate) * redistributionProportion))
		originalReqRates := make(map[int]int)
		for _, node := range nodes {
			originalReqRates[node.id] = node.reqRate
		}

		maxDPPNode.reqRate -= redistributionPool

		// Line 7: Redistribute the pool
		redistributeRequests(nodes, maxDPPNode.id, deactivatedNodes, redistributionPool)

		// Line 8: Update DPP values
		computeDPPAndCPU(nodes)

		// Line 9: Check if DPP improved globally
		improved := checkGlobalDPPImprovement(nodes, originalReqRates)

		if !improved {
			// Line 11-12: Rollback changes
			for _, node := range nodes {
				node.reqRate = originalReqRates[node.id]
			}
			deactivatedNodes[maxDPPNode.id] = true
			computeDPPAndCPU(nodes)
		}

		// Check if all nodes are deactivated
		activeNodesExist := false
		for _, node := range nodes {
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

	// Update virtual queue values
	for _, node := range nodes {
		nextQueueBacklog := node.queueBacklog + node.cpuUsage - CPUTargetThreshold
		if nextQueueBacklog < 0 {
			nextQueueBacklog = 0
		}
		node.queueBacklog = nextQueueBacklog
	}

	// Create the distribution map
	distribution := make(map[string]int)
	for _, node := range nodes {
		distribution[node.ip] = node.reqRate
	}

	return distribution
}

func computeDPPAndCPU(nodes []*Node) {
	for _, node := range nodes {
		onsetCPU := node.cpuUsage
		deltaReq := float64(node.reqRate - node.onsetReq)
		deltaCPU := 0.0

		if node.CoreNum == 1 {
			if onsetCPU <= CPULowThreshold {
				deltaCPU = (float64(node.reqRate)-CPU0to60_Intercept_1C)/CPU0to60_Slope_1C - onsetCPU
			} else if onsetCPU <= CPUTargetThreshold && onsetCPU > CPULowThreshold {
				deltaCPU = (float64(node.reqRate)-CPU60to70_Intercept_1C)/CPU60to70_Slope_1C - onsetCPU
			} else {
				deltaCPU = (float64(node.reqRate)-CPU70to80_Intercept_1C)/CPU70to80_Slope_1C - onsetCPU
			}
		} else {
			if onsetCPU < CPULowThreshold {
				deltaCPU = (float64(node.reqRate)-CPU0to60_Intercept_2C)/CPU0to60_Slope_2C - onsetCPU
			} else if onsetCPU <= CPUTargetThreshold && onsetCPU > CPULowThreshold {
				deltaCPU = (float64(node.reqRate)-CPU60to70_Intercept_2C)/CPU60to70_Slope_2C - onsetCPU
			} else {
				deltaCPU = (float64(node.reqRate)-CPU70to80_Intercept_2C)/CPU70to80_Slope_2C - onsetCPU
			}
		}

		node.cpuUsage = onsetCPU + deltaCPU
		nextQueueBacklog := node.queueBacklog + onsetCPU + deltaCPU - CPUTargetThreshold
		if nextQueueBacklog < 0 {
			nextQueueBacklog = 0
		}

		weight := 1.0
		if node.CoreNum == 1 {
			weight = 1.0
		} else {
			weight = 0.5
		}

		stabilityComponent := weight * node.queueBacklog * deltaCPU
		performanceComponent := V * node.delay * deltaReq
		node.dppValue = stabilityComponent + performanceComponent
	}
}

func findMaxDPPNode(nodes []*Node, deactivated map[int]bool) *Node {
	var maxNode *Node
	maxDPP := -math.MaxFloat64

	for _, node := range nodes {
		if !deactivated[node.id] && node.dppValue > maxDPP {
			maxDPP = node.dppValue
			maxNode = node
		}
	}

	return maxNode
}

func redistributeRequests(nodes []*Node, excludeNodeID int, deactivated map[int]bool, pool int) {
	totalCoef := 0.0
	eligibleNodes := []*Node{}

	for _, node := range nodes {
		if node.id != excludeNodeID && !deactivated[node.id] {
			node.coefficient = (100 - node.cpuUsage) * float64(node.CoreNum)
			totalCoef += node.coefficient
			eligibleNodes = append(eligibleNodes, node)
		}
	}

	if len(eligibleNodes) == 0 || totalCoef <= 0 {
		return
	}

	remainingPool := pool
	for i, node := range eligibleNodes {
		var share int
		if i == len(eligibleNodes)-1 {
			share = remainingPool
		} else {
			share = int(math.Floor((node.coefficient / totalCoef) * float64(pool)))
		}

		node.reqRate += share
		remainingPool -= share
	}

	if remainingPool > 0 && len(eligibleNodes) > 0 {
		eligibleNodes[0].reqRate += remainingPool
	}
}

func checkGlobalDPPImprovement(nodes []*Node, originalReqRates map[int]int) bool {
	currentReqRates := make(map[int]int)
	currentDPPValues := make(map[int]float64)

	for _, node := range nodes {
		currentReqRates[node.id] = node.reqRate
		currentDPPValues[node.id] = node.dppValue
	}

	for _, node := range nodes {
		node.reqRate = originalReqRates[node.id]
	}

	computeDPPAndCPU(nodes)
	originalDPPSum := 0.0
	for _, node := range nodes {
		originalDPPSum += node.dppValue
	}

	for _, node := range nodes {
		node.reqRate = currentReqRates[node.id]
	}

	computeDPPAndCPU(nodes)
	newDPPSum := 0.0
	for _, node := range nodes {
		newDPPSum += node.dppValue
	}

	return newDPPSum < originalDPPSum
}

func Bpr(db *sql.DB, region string, totalReqIncrement int, redistributionProportion float64) (map[string]int, error) {
	// prepare data
	dbNodes, err := models.GetLatestNodeInfoByRegion(db, region)
	if err != nil {
		log.Printf("Error fetching latest node info for region '%s': %v", region, err)
		return nil, nil
	}

	if len(dbNodes) == 0 {
		log.Printf("No nodes found in region '%s'. Aborting BPR.", region)
		return nil, nil
	}

	log.Printf("Fetched %d nodes from database for region '%s'.", len(dbNodes), region)

	bprNodes := make([]*Node, len(dbNodes))
	for i, dbNode := range dbNodes {
		currentQueueBacklog := GetNodeQueueBacklog(dbNode.IP)
		currentOnsetReq := 0
		bprNodes[i] = &Node{
			id:           i,
			ip:           dbNode.IP,
			onsetReq:     currentOnsetReq, // Default value
			cpuUsage:     dbNode.CPUUsage,
			queueBacklog: currentQueueBacklog,
			delay:        float64(rand.Intn(100) + 1), // Default value for delay, or fetch from another source if available
			isActive:     true,                        // Always active at the start
			CoreNum:      dbNode.CPUCores,
		}
		log.Printf("Prepared BPR Node: IP=%s, OnsetCPU=%.2f, CoreNum=%d, OnsetReq=%d, QueueBacklog=%.2f",
			bprNodes[i].ip, bprNodes[i].cpuUsage, bprNodes[i].CoreNum, bprNodes[i].onsetReq, bprNodes[i].queueBacklog)
	}
	// Ensure BPRAlgorithm is accessible
	log.Printf("Running BPRAlgorithm with TotalReqIncrement=%d, RedistributionProportion=%.2f for %d nodes.",
		totalReqIncrement, redistributionProportion, len(bprNodes))

	// Call BPRAlgorithm. Your BPRAlgorithm function returns map[string]int.
	finalDistribution := BPRAlgorithm(bprNodes, totalReqIncrement, redistributionProportion)
	log.Println("BPR Algorithm finished. Final distribution:")
	totalAllocated := 0
	for ip, req := range finalDistribution {
		log.Printf("  IP: %s -> Allocated Requests: %d", ip, req)
		totalAllocated += req
	}
	log.Printf("Total requests allocated by BPR: %d", totalAllocated)

	for _, updatedNode := range bprNodes {
		log.Printf("Updated Node State: IP=%s, Final ReqRate=%d, Final CPUUsage=%.2f, Final QueueBacklog=%.4f, Final DPP=%.4f",
			updatedNode.ip, updatedNode.reqRate, updatedNode.cpuUsage, updatedNode.queueBacklog, updatedNode.dppValue)

		// Update persistent QueueBacklog for this node
		UpdateNodeQueueBacklog(updatedNode.ip, updatedNode.queueBacklog)
	}

	log.Printf("BPR process for region %s completed.", region)
	return finalDistribution, nil
}
