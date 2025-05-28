package bpr

import (
	"database/sql"
	"fmt"
	"log"
	"math"
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
	fmt.Println("\n[BPR] Algorithm execution started:")
	fmt.Printf("[BPR] Input parameters: Node count=%d, Total request increment=%d, Redistribution proportion=%.2f\n", len(nodes), totalReqIncrement, redistributionProportion)

	// Line 1: Initial proportional allocation of request increment
	fmt.Println("\n[BPR] Step 1: Initial proportional allocation of request increment")
	totalOnsetReq := 0
	for _, node := range nodes {
		totalOnsetReq += node.onsetReq
	}
	fmt.Printf("[BPR] Total initial requests across all nodes: %d\n", totalOnsetReq)

	// Keep track of allocated requests to handle rounding
	remainingIncrement := totalReqIncrement

	// Allocate increment proportionally based on initial request rate
	fmt.Println("[BPR] Allocating request increment proportionally:")
	for i, node := range nodes {
		var increment int
		if i == len(nodes)-1 {
			// Last node gets the remainder to ensure total is exactly matched
			increment = remainingIncrement
			fmt.Printf("[BPR]  - Node %d (IP: %s): Gets remaining increment %d\n", node.id, node.ip, increment)
		} else if totalOnsetReq > 0 {
			// Calculate proportional share and round to nearest integer
			proportion := float64(node.onsetReq) / float64(totalOnsetReq)
			increment = int(math.Round(proportion * float64(totalReqIncrement)))
			remainingIncrement -= increment
			fmt.Printf("[BPR]  - Node %d (IP: %s): Proportion=%.4f, Increment=%d, Remaining increment=%d\n",
				node.id, node.ip, proportion, increment, remainingIncrement)
		}
		node.reqRate = node.onsetReq + increment
		fmt.Printf("[BPR]  - Node %d final initial allocation: Original=%d + Increment=%d = New request count=%d\n",
			node.id, node.onsetReq, increment, node.reqRate)
	}

	// Line 2: Compute dppValue for each node
	fmt.Println("\n[BPR] Step 2: Calculate DPP value for each node")
	computeDPPAndCPU(nodes)

	fmt.Println("[BPR] Initial DPP values after calculation:")
	for _, node := range nodes {
		fmt.Printf("[BPR]  - Node %d (IP: %s): DPP=%.4f, CPU=%.2f%%, Requests=%d, Queue backlog=%.4f\n",
			node.id, node.ip, node.dppValue, node.cpuUsage, node.reqRate, node.queueBacklog)
	}

	// Step 3: Instead of using MAD to identify outliers, we just mark all nodes as active
	// This way, our main loop will directly work with the maximum DPP node
	fmt.Println("\n[BPR] Step 3: Mark all nodes as active")
	deactivatedNodes := make(map[int]bool)
	for _, node := range nodes {
		node.isActive = true
		fmt.Printf("[BPR]  - Node %d (IP: %s): Marked as active\n", node.id, node.ip)
	}

	// Lines 4-14: Main iteration - redistribute requests from nodes with highest DPP
	fmt.Println("\n[BPR] Steps 4-14: Main loop - Redistribute requests from highest DPP nodes")
	maxIterations := 3 // Safety limit to prevent infinite loops
	iteration := 0

	for iteration < maxIterations {
		fmt.Printf("\n[BPR] Starting iteration %d:\n", iteration+1)

		// Line 5: Find node with maximum DPP value among active nodes
		maxDPPNode := findMaxDPPNode(nodes, deactivatedNodes)
		if maxDPPNode == nil {
			fmt.Println("[BPR] No max DPP node found, exiting main loop")
			break
		}
		fmt.Printf("[BPR] Found max DPP node: ID=%d, IP=%s, DPP=%.4f, CPU=%.2f%%, Requests=%d\n",
			maxDPPNode.id, maxDPPNode.ip, maxDPPNode.dppValue, maxDPPNode.cpuUsage, maxDPPNode.reqRate)

		// Line 6: Remove p% of requests from maxDPPNode for redistribution
		redistributionPool := int(math.Round(float64(maxDPPNode.reqRate) * redistributionProportion))
		fmt.Printf("[BPR] Removing %.2f%% of requests from max DPP node for redistribution: %d\n",
			redistributionProportion*100, redistributionPool)

		// Store original values for potential rollback
		originalReqRates := make(map[int]int)
		for _, node := range nodes {
			originalReqRates[node.id] = node.reqRate
			fmt.Printf("[BPR] Saving original request count: Node %d = %d\n", node.id, node.reqRate)
		}

		maxDPPNode.reqRate -= redistributionPool
		fmt.Printf("[BPR] Max DPP node new request count: %d\n", maxDPPNode.reqRate)

		// Line 7: Redistribute the pool to non-deactivated nodes except the max DPP node
		fmt.Println("[BPR] Redistributing requests to other active nodes:")
		redistributeRequests(nodes, maxDPPNode.id, deactivatedNodes, redistributionPool)

		// Print current distribution after redistribution
		fmt.Println("[BPR] Request distribution after redistribution:")
		for _, node := range nodes {
			fmt.Printf("[BPR]  - Node %d (IP: %s): Requests=%d\n", node.id, node.ip, node.reqRate)
		}

		// Line 8: Update DPP values and CPU usage after redistribution
		fmt.Println("[BPR] Updating DPP values and CPU usage after redistribution:")
		computeDPPAndCPU(nodes)
		for _, node := range nodes {
			fmt.Printf("[BPR]  - Node %d (IP: %s): New DPP=%.4f, New CPU=%.2f%%, Requests=%d\n",
				node.id, node.ip, node.dppValue, node.cpuUsage, node.reqRate)
		}

		// Line 9: Check if DPP improved globally
		improved := checkGlobalDPPImprovement(nodes, originalReqRates)
		fmt.Printf("[BPR] Global DPP improvement: %v\n", improved)

		if !improved {
			// Line 11-12: Rollback changes and deactivate the node
			fmt.Println("[BPR] No improvement, rolling back changes and deactivating max DPP node:")
			for _, node := range nodes {
				node.reqRate = originalReqRates[node.id]
				fmt.Printf("[BPR]  - Node %d rolled back request count: %d\n", node.id, node.reqRate)
			}
			deactivatedNodes[maxDPPNode.id] = true
			fmt.Printf("[BPR] Deactivated node: ID=%d, IP=%s\n", maxDPPNode.id, maxDPPNode.ip)

			fmt.Println("[BPR] Recalculating DPP values after rollback:")
			computeDPPAndCPU(nodes) // Recompute after rollback
			for _, node := range nodes {
				fmt.Printf("[BPR]  - Node %d (IP: %s): DPP=%.4f, CPU=%.2f%%, Requests=%d\n",
					node.id, node.ip, node.dppValue, node.cpuUsage, node.reqRate)
			}
		} else {
			fmt.Println("[BPR] Global DPP improved, keeping current allocation")
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
			fmt.Println("[BPR] All nodes deactivated, exiting main loop")
			break
		}

		iteration++
		fmt.Printf("[BPR] Iteration %d completed\n", iteration)
	}
	// Update virtual queue values ****
	fmt.Println("\n[BPR] Updating virtual queue values for all nodes:")
	for _, node := range nodes {
		// Calculate next queue backlog according to Equation (1)
		// Qk(t+1) = max[Qk(t) + (cpu_k^t,in) - θ, 0]
		nextQueueBacklog := node.queueBacklog + node.cpuUsage - CPUTargetThreshold
		if nextQueueBacklog < 0 {
			nextQueueBacklog = 0
		}

		// Update the queue backlog
		fmt.Printf("[BPR]  - Node %d (IP: %s): Queue backlog updated from %.4f to %.4f\n",
			node.id, node.ip, node.queueBacklog, nextQueueBacklog)
		node.queueBacklog = nextQueueBacklog
	}

	fmt.Println("\n[BPR] Main loop completed, collecting final allocation results")
	// Create the distribution map with IP addresses as keys and FinalReq values
	distribution := make(map[string]int)
	for _, node := range nodes {
		distribution[node.ip] = node.reqRate
		fmt.Printf("[BPR] Final allocation: IP=%s, Requests=%d\n", node.ip, node.reqRate)
	}

	fmt.Println("[BPR] Algorithm execution completed")
	return distribution
}

func computeDPPAndCPU(nodes []*Node) {
	fmt.Println("[DPP Calculation] Starting DPP and CPU calculation:")
	for _, node := range nodes {
		fmt.Printf("[DPP Calculation] Calculating for node %d (IP: %s):\n", node.id, node.ip)
		// 1. Get the known initial CPU value
		onsetCPU := node.cpuUsage
		fmt.Printf("[DPP Calculation]  - Initial CPU: %.2f%%\n", onsetCPU)

		// 2. Calculate estimated CPU change based on request rate change
		deltaReq := float64(node.reqRate - node.onsetReq)
		deltaCPU := 0.0
		fmt.Printf("[DPP Calculation]  - Request change: %d - %d = %.0f\n", node.reqRate, node.onsetReq, deltaReq)

		// Apply the appropriate model based on node type and CPU range
		if node.CoreNum == 1 {
			// 1-core nodes
			fmt.Println("[DPP Calculation]  - Using single-core node model")
			if onsetCPU <= CPULowThreshold {
				// Below 60% CPU - use 0-60% model
				deltaCPU = (float64(node.reqRate)-CPU0to60_Intercept_1C)/CPU0to60_Slope_1C - onsetCPU
			} else if onsetCPU <= CPUTargetThreshold && onsetCPU > CPULowThreshold {
				// 60-70% CPU range - use 60-70% model
				deltaCPU = (float64(node.reqRate)-CPU60to70_Intercept_1C)/CPU60to70_Slope_1C - onsetCPU
				fmt.Printf("[DPP Calculation]  - 60%% < CPU ≤ 70%%, using 60-70%% model: (%.0f - %.2f)/%.2f - %.2f = %.4f\n",
					float64(node.reqRate), CPU60to70_Intercept_1C, CPU60to70_Slope_1C, onsetCPU, deltaCPU)
			} else {
				// 70-80% CPU range (or higher) - use 70-80% model
				deltaCPU = (float64(node.reqRate)-CPU70to80_Intercept_1C)/CPU70to80_Slope_1C - onsetCPU
				fmt.Printf("[DPP Calculation]  - CPU > 70%%, using 70-80%% model: (%.0f - %.2f)/%.2f - %.2f = %.4f\n",
					float64(node.reqRate), CPU70to80_Intercept_1C, CPU70to80_Slope_1C, onsetCPU, deltaCPU)
			}
		} else {
			// 2-core nodes
			fmt.Println("[DPP Calculation]  - Using dual-core node model")
			if onsetCPU < CPULowThreshold {
				// Below 60% CPU - use 0-60% model
				deltaCPU = (float64(node.reqRate)-CPU0to60_Intercept_2C)/CPU0to60_Slope_2C - onsetCPU
			} else if onsetCPU <= CPUTargetThreshold && onsetCPU > CPULowThreshold {
				// 60-70% CPU range - use 60-70% model
				deltaCPU = (float64(node.reqRate)-CPU60to70_Intercept_2C)/CPU60to70_Slope_2C - onsetCPU
				fmt.Printf("[DPP Calculation]  - 60%% < CPU ≤ 70%%, using 60-70%% model: (%.0f - %.2f)/%.2f - %.2f = %.4f\n",
					float64(node.reqRate), CPU60to70_Intercept_2C, CPU60to70_Slope_2C, onsetCPU, deltaCPU)
			} else {
				// 70-80% CPU range (or higher) - use 70-80% model
				deltaCPU = (float64(node.reqRate)-CPU70to80_Intercept_2C)/CPU70to80_Slope_2C - onsetCPU
				fmt.Printf("[DPP Calculation]  - CPU > 70%%, using 70-80%% model: (%.0f - %.2f)/%.2f - %.2f = %.4f\n",
					float64(node.reqRate), CPU70to80_Intercept_2C, CPU70to80_Slope_2C, onsetCPU, deltaCPU)
			}
		}

		// Update the CPU value based on estimated change
		node.cpuUsage = onsetCPU + deltaCPU
		fmt.Printf("[DPP Calculation]  - Updated CPU value: %.2f%% + %.2f%% = %.2f%%\n", onsetCPU, deltaCPU, node.cpuUsage)

		// 3. Update virtual queue backlog according to Equation (1)
		// Qk(t+1) = max[Qk(t) + (cpu_k^t,onset + δcpu_k^t,in) - θ, 0]
		nextQueueBacklog := node.queueBacklog + onsetCPU + deltaCPU - CPUTargetThreshold
		if nextQueueBacklog < 0 {
			nextQueueBacklog = 0
		}
		fmt.Printf("[DPP Calculation]  - Calculated queue backlog: max[%.4f + %.2f%% + %.2f%% - %d%%, 0] = %.4f\n",
			node.queueBacklog, onsetCPU, deltaCPU, CPUTargetThreshold, nextQueueBacklog)

		// 4. Set node weight based on CPU cores
		weight := 1.0
		if node.CoreNum == 1 {
			weight = 1.0 // 1-core weight is 1
			fmt.Println("[DPP Calculation]  - Single-core node weight: 1.0")
		} else {
			weight = 0.5 // 2-core weight is 0.5 (1/2)
			fmt.Println("[DPP Calculation]  - Dual-core node weight: 0.5")
		}

		// 5. Calculate stability component (drift term)
		stabilityComponent := weight * node.queueBacklog * deltaCPU
		fmt.Printf("[DPP Calculation]  - Stability component (drift term): %.2f × %.4f × %.2f = %.4f\n",
			weight, node.queueBacklog, deltaCPU, stabilityComponent)

		// 6. Calculate performance component (penalty term)
		performanceComponent := V * node.delay * deltaReq
		fmt.Printf("[DPP Calculation]  - Performance component (penalty term): %.4f × %.2f × %.0f = %.4f\n",
			V, node.delay, deltaReq, performanceComponent)

		// 7. Total DPP value
		node.dppValue = stabilityComponent + performanceComponent
		fmt.Printf("[DPP Calculation]  - Total DPP value: %.4f + %.4f = %.4f\n",
			stabilityComponent, performanceComponent, node.dppValue)

		// 8. Update the queue backlog for the next iteration
		//node.queueBacklog = nextQueueBacklog
		//fmt.Printf("[DPP Calculation]  - Updated queue backlog: %.4f\n", node.queueBacklog)
	}
	fmt.Println("[DPP Calculation] DPP and CPU calculation completed")
}

// Find node with maximum DPP value among active nodes
func findMaxDPPNode(nodes []*Node, deactivated map[int]bool) *Node {
	fmt.Println("[Find Max DPP] Starting search for max DPP node:")
	var maxNode *Node
	maxDPP := -math.MaxFloat64

	for _, node := range nodes {
		if !deactivated[node.id] {
			fmt.Printf("[Find Max DPP] Checking node %d (IP: %s): DPP=%.4f, Deactivated=%v\n",
				node.id, node.ip, node.dppValue, deactivated[node.id])
			if node.dppValue > maxDPP {
				maxDPP = node.dppValue
				maxNode = node
				fmt.Printf("[Find Max DPP] Found new max DPP node: ID=%d, DPP=%.4f\n", node.id, maxDPP)
			}
		} else {
			fmt.Printf("[Find Max DPP] Skipping deactivated node %d (IP: %s)\n", node.id, node.ip)
		}
	}

	if maxNode != nil {
		fmt.Printf("[Find Max DPP] Final selected max DPP node: ID=%d, IP=%s, DPP=%.4f\n",
			maxNode.id, maxNode.ip, maxNode.dppValue)
	} else {
		fmt.Println("[Find Max DPP] No active nodes found")
	}

	return maxNode
}

// Redistribute requests to non-deactivated nodes except the specified node
func redistributeRequests(nodes []*Node, excludeNodeID int, deactivated map[int]bool, pool int) {
	fmt.Printf("[Redistribute Requests] Starting redistribution of %d requests:\n", pool)
	// Calculate total coefficient for redistribution
	totalCoef := 0.0
	eligibleNodes := []*Node{}

	// Find eligible nodes and calculate their coefficients
	fmt.Println("[Redistribute Requests] Calculating coefficients for eligible nodes:")
	for _, node := range nodes {
		if node.id != excludeNodeID && !deactivated[node.id] {
			// Coefficient based on available CPU capacity
			node.coefficient = (100 - node.cpuUsage) * float64(node.CoreNum)
			totalCoef += node.coefficient
			eligibleNodes = append(eligibleNodes, node)
		} else {
			fmt.Printf("[Redistribute Requests] Excluding node %d: Exclusion reason=%v\n",
				node.id, node.id == excludeNodeID || deactivated[node.id])
		}
	}

	// If no eligible nodes or zero total coefficient, return without redistribution
	if len(eligibleNodes) == 0 || totalCoef <= 0 {
		fmt.Println("[Redistribute Requests] No eligible nodes or zero total coefficient, skipping redistribution")
		return
	}

	// Redistribute based on coefficients
	remainingPool := pool
	fmt.Printf("[Redistribute Requests] Redistributing %d requests based on coefficients:\n", remainingPool)

	// First pass - allocate based on proportional coefficients
	for i, node := range eligibleNodes {
		var share int

		if i == len(eligibleNodes)-1 {
			// Last eligible node gets all remaining requests
			share = remainingPool
			fmt.Printf("[Redistribute Requests] Last eligible node %d gets all remaining requests: %d\n", node.id, share)
		} else {
			// Calculate proportional share
			share = int(math.Floor((node.coefficient / totalCoef) * float64(pool)))
			fmt.Printf("[Redistribute Requests] Node %d gets proportional share: (%.2f/%.2f)×%d=%.2f≈%d\n",
				node.id, node.coefficient, totalCoef, pool, (node.coefficient/totalCoef)*float64(pool), share)
		}

		node.reqRate += share
		remainingPool -= share
		fmt.Printf("[Redistribute Requests] Node %d new request count: %d, Remaining unallocated requests: %d\n", node.id, node.reqRate, remainingPool)
	}

	// Safety check: If we still have remaining requests, give them to the first eligible node
	if remainingPool > 0 && len(eligibleNodes) > 0 {
		fmt.Printf("[Redistribute Requests] Still have %d unallocated requests, assigning to first eligible node %d\n",
			remainingPool, eligibleNodes[0].id)
		eligibleNodes[0].reqRate += remainingPool
		fmt.Printf("[Redistribute Requests] Node %d final request count: %d\n", eligibleNodes[0].id, eligibleNodes[0].reqRate)
	}

	fmt.Println("[Redistribute Requests] Request redistribution completed")
}

// Check if global DPP improved after redistribution
func checkGlobalDPPImprovement(nodes []*Node, originalReqRates map[int]int) bool {
	fmt.Println("[Check DPP Improvement] Checking for global DPP improvement:")
	// Store current values
	currentReqRates := make(map[int]int)
	currentDPPValues := make(map[int]float64)
	currentCPUValues := make(map[int]float64)
	currentBacklogs := make(map[int]float64)

	fmt.Println("[Check DPP Improvement] Saving current values:")
	for _, node := range nodes {
		currentReqRates[node.id] = node.reqRate
		currentDPPValues[node.id] = node.dppValue
		currentCPUValues[node.id] = node.cpuUsage
		currentBacklogs[node.id] = node.queueBacklog
		fmt.Printf("[Check DPP Improvement] Node %d: Requests=%d, DPP=%.4f, CPU=%.2f%%, Queue backlog=%.4f\n",
			node.id, node.reqRate, node.dppValue, node.cpuUsage, node.queueBacklog)
	}

	// Temporarily restore original values to calculate original DPP
	fmt.Println("[Check DPP Improvement] Temporarily restoring original request counts to calculate original DPP:")
	for _, node := range nodes {
		node.reqRate = originalReqRates[node.id]
		fmt.Printf("[Check DPP Improvement] Node %d: Restored original request count=%d\n", node.id, node.reqRate)
	}

	// Recalculate DPP with original request rates
	fmt.Println("[Check DPP Improvement] Recalculating DPP with original request counts:")
	computeDPPAndCPU(nodes)

	// Calculate original DPP sum
	originalDPPSum := 0.0
	for _, node := range nodes {
		originalDPPSum += node.dppValue
		fmt.Printf("[Check DPP Improvement] Node %d original DPP=%.4f, Cumulative sum=%.4f\n", node.id, node.dppValue, originalDPPSum)
	}
	fmt.Printf("[Check DPP Improvement] Original DPP total: %.4f\n", originalDPPSum)

	// Restore current values
	fmt.Println("[Check DPP Improvement] Restoring current request counts:")
	for _, node := range nodes {
		node.reqRate = currentReqRates[node.id]
		fmt.Printf("[Check DPP Improvement] Node %d: Restored current request count=%d\n", node.id, node.reqRate)
	}

	// Recalculate DPP with current request rates
	fmt.Println("[Check DPP Improvement] Recalculating DPP with current request counts:")
	computeDPPAndCPU(nodes)

	// Calculate new DPP sum
	newDPPSum := 0.0
	for _, node := range nodes {
		newDPPSum += node.dppValue
		fmt.Printf("[Check DPP Improvement] Node %d new DPP=%.4f, Cumulative sum=%.4f\n", node.id, node.dppValue, newDPPSum)
	}
	fmt.Printf("[Check DPP Improvement] New DPP total: %.4f\n", newDPPSum)

	improved := newDPPSum < originalDPPSum
	fmt.Printf("[Check DPP Improvement] DPP improvement: %.4f < %.4f = %v\n", newDPPSum, originalDPPSum, improved)
	return improved
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
			delay:        50.0 - float64(i*10), // Default value for delay, or fetch from another source if available
			isActive:     true,                 // Always active at the start
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
