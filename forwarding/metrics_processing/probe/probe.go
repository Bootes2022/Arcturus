package probe

import (
	"forwarding/common"
	"forwarding/metrics_processing/collector"
	"forwarding/metrics_processing/protocol"
	"forwarding/metrics_processing/storage"
	"forwarding/router"
	"forwarding/scheduling_algorithms/k_shortest"
	"log"
	"math"
	"net"
	"sync"
	"time"
)

// ProbeTimeout is the maximum time to wait for a TCP probe
var ProbeTimeout = 2 * time.Second

func performTCPProbe(targetIP string, port string) (int64, error) {
	timeoutDuration := ProbeTimeout
	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", targetIP+":"+port, timeoutDuration)
	if err != nil {
		return -1, nil
	}
	defer conn.Close()

	delay := time.Since(startTime).Milliseconds()
	return delay, nil
}

func processRegionProbeResults(probes []*protocol.ProbeResult) ([]*protocol.ProbeResult, error) {
	if len(probes) == 0 {
		return []*protocol.ProbeResult{}, nil
	}

	delays := make([]float64, 0, len(probes))
	ipToIndex := make(map[int]string)
	indexToIP := make(map[string]int)

	for _, probe := range probes {
		if probe.TcpDelay > 0 {
			delays = append(delays, float64(probe.TcpDelay))
			ipToIndex[len(delays)-1] = probe.TargetIp
			indexToIP[probe.TargetIp] = len(delays) - 1
		}
	}

	if len(delays) == 0 {
		return probes, nil // ，
	}

	k := 3
	sensitivity := 1.5
	outliers := k_shortest.DetectOutliersAdaptive(delays, k, sensitivity)

	outlierIndices := make(map[int]bool)
	for _, outlier := range outliers {
		outlierIndices[outlier.Index] = true
	}

	var sumNormal float64
	var countNormal int

	for i, delay := range delays {
		if !outlierIndices[i] {
			sumNormal += delay
			countNormal++
		}
	}

	var avgNormal float64
	if countNormal > 0 {
		avgNormal = sumNormal / float64(countNormal)
	}

	optimizedProbes := make([]*protocol.ProbeResult, 0)

	if countNormal > 0 {
		normalProbe := &protocol.ProbeResult{
			TargetIp: "normal_avg", //
			TcpDelay: int64(math.Round(avgNormal)),
		}
		optimizedProbes = append(optimizedProbes, normalProbe)

	}

	for _, outlier := range outliers {
		targetIP := ipToIndex[outlier.Index]
		outlierProbe := &protocol.ProbeResult{
			TargetIp: targetIP,
			TcpDelay: int64(math.Round(outlier.Value)),
		}
		optimizedProbes = append(optimizedProbes, outlierProbe)

	}

	return optimizedProbes, nil
}

func CollectRegionProbeResults(fileManager *storage.FileManager) ([]*protocol.RegionProbeResult, error) {
	nodeList := fileManager.GetNodeList()
	probeTasks := fileManager.GetProbeTasks()
	if nodeList == nil || len(nodeList.Nodes) == 0 {

		return []*protocol.RegionProbeResult{}, nil
	}
	if len(probeTasks) == 0 {

		return []*protocol.RegionProbeResult{}, nil
	}
	ipToRegion := make(map[string]string)
	for _, node := range nodeList.Nodes {
		ipToRegion[node.Ip] = node.Region
	}
	regionProbesMap := make(map[string][]*protocol.ProbeResult)
	var resultsLock sync.Mutex
	var wg sync.WaitGroup
	poolConfig := common.PoolConfig{
		MaxWorkers: 50,
	}
	pool, err := common.NewPool(poolConfig)
	if err != nil {
		log.Printf(": %v", err)
		return nil, err
	}
	defer pool.Release()
	log.Printf(" %d ", len(probeTasks))
	for _, task := range probeTasks {
		wg.Add(1)
		taskCopy := task
		err := pool.Submit(func() {
			defer wg.Done()
			targetRegion, exists := ipToRegion[taskCopy.TargetIp]
			if !exists {
				log.Printf(": IP %s ，'unknown'", taskCopy.TargetIp)
				targetRegion = "unknown"
			}
			tcpDelay, err := performTCPProbe(taskCopy.TargetIp, "50051")
			if err != nil {
				log.Printf(" %s : %v", taskCopy.TargetIp, err)
				return
			}
			probeResult := &protocol.ProbeResult{
				TargetIp: taskCopy.TargetIp,
				TcpDelay: tcpDelay,
			}
			resultsLock.Lock()
			regionProbesMap[targetRegion] = append(regionProbesMap[targetRegion], probeResult)
			resultsLock.Unlock()
		})
		if err != nil {
			log.Printf(": %v", err)
		}
	}
	// --- New probing logic for DomainIPMappings ---
	domainMappings := router.GetAllDomainMapIP()
	if domainMappings == nil {
		log.Println("No domain mappings found or error retrieving them. Skipping domain IP probes.")
	} else {
		for _, mapping := range domainMappings {
			if mapping.Ip == "" { // Skip if IP is empty
				log.Printf("Skipping domain mapping for %s, IP is empty.", mapping.Domain)
				continue
			}
			wg.Add(1)
			mappingCopy := mapping // Capture loop variable for goroutine
			submitErr := pool.Submit(func() {
				defer wg.Done()
				targetIP := mappingCopy.Ip
				// change source ip
				sourceIp, _ := collector.GetIP()
				sourceRegion, exists := ipToRegion[sourceIp]
				if !exists {
					log.Printf("Region lookup: IP %s (from domain %s), Region 'unknown'", targetIP, mappingCopy.Domain)
					sourceRegion = "unknown"
				}
				tcpDelay, probeErr := performTCPProbe(targetIP, "80")
				if probeErr != nil {
					log.Printf("TCP Probe Error for %s (from domain %s): %v", targetIP, mappingCopy.Domain, probeErr)
					return
				}
				probeResult := &protocol.ProbeResult{
					TargetIp: targetIP,
					TcpDelay: tcpDelay,
				}
				resultsLock.Lock()
				regionProbesMap[sourceRegion] = append(regionProbesMap[sourceRegion], probeResult)
				resultsLock.Unlock()
				log.Printf("Probe successful for %s (from domain %s, region %s): %dms", targetIP, mappingCopy.Domain, sourceRegion, tcpDelay)
			})
			if submitErr != nil {
				log.Printf("Failed to submit task for Domain IP %s (domain %s) to pool: %v", mappingCopy.Ip, mappingCopy.Domain, submitErr)
				wg.Done() // Decrement counter if submit fails
			}
		}
	}
	wg.Wait()
	log.Println("All probes finished. Aggregating results...")

	var regionProbeResults []*protocol.RegionProbeResult
	resultsLock.Lock()
	for region, probes := range regionProbesMap {
		if len(probes) == 0 {
			log.Printf("No successful probes for region: %s, skipping.", region)
			continue
		}

		regionResult := &protocol.RegionProbeResult{
			Region:   region,
			IpProbes: probes,
		}
		regionProbeResults = append(regionProbeResults, regionResult)
		log.Printf("Aggregated %d probes for region: %s", len(probes), region)
	}
	resultsLock.Unlock()
	log.Printf("Total regions with probe results: %d", len(regionProbeResults))
	return regionProbeResults, nil
}

/*var regionProbeResults []*protocol.RegionProbeResult
for region, probes := range regionProbesMap {
	optimizedProbes, err := processRegionProbeResults(probes)
	if err != nil {

		optimizedProbes = probes
	}
	regionResult := &protocol.RegionProbeResult{
		Region:   region,
		IpProbes: optimizedProbes,
	}
	regionProbeResults = append(regionProbeResults, regionResult)
}
return regionProbeResults, nil
}*/
