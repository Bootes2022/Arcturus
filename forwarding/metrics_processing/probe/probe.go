package probe

import (
	"forwarding/common"
	"forwarding/metrics_processing/protocol"
	"forwarding/metrics_processing/storage"
	"forwarding/scheduling_algorithms/k_shortest"
	"log"
	"math"
	"net"
	"sync"
	"time"
)

// ProbeTimeout is the maximum time to wait for a TCP probe
var ProbeTimeout = 2 * time.Second

func performTCPProbe(targetIP string) (int64, error) {
	timeoutDuration := ProbeTimeout
	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", targetIP+":50051", timeoutDuration)
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
		log.Println("，")
		return []*protocol.RegionProbeResult{}, nil
	}

	if len(probeTasks) == 0 {
		log.Println("，")
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

			tcpDelay, err := performTCPProbe(taskCopy.TargetIp)
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

	wg.Wait()

	var regionProbeResults []*protocol.RegionProbeResult
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
}
