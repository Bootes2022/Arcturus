package metrics

import (
	"database/sql"
	"log"
	pb "scheduling/controller/heartbeats/proto"
	"scheduling/models"
	"time"
)

type Processor struct {
	db *sql.DB
}

func NewProcessor(db *sql.DB) *Processor {
	return &Processor{db: db}
}

func (p *Processor) ProcessProbeResults(sourceIP string, results []*pb.RegionProbeResult) error {
	probeTime := time.Now()

	sourceRegion, err := models.GetNodeRegion(p.db, sourceIP)
	if err != nil {
		log.Printf("Error getting source region for IP %s: %v. Using 'unknown'.", sourceIP, err)
		sourceRegion = "unknown"
	}
	log.Printf("Processing %d region results for source IP: %s (Region: %s)", len(results), sourceIP, sourceRegion)
	for _, regionResult := range results {
		targetRegion := regionResult.Region
		if targetRegion == "" {
			log.Printf("Warning: Received probe result with empty target region for source IP %s. Setting to 'unknown'.", sourceIP)
			targetRegion = "unknown"
		}
		log.Printf("  Processing %d probes for target region: %s", len(regionResult.IpProbes), targetRegion)
		for _, probe := range regionResult.IpProbes {
			if probe.TargetIp == "" {
				log.Printf("Warning: Received probe with empty TargetIp for source %s, target region %s. Skipping.", sourceIP, targetRegion)
				continue
			}
			if probe.TcpDelay < 0 {
				log.Printf("Probe failed for TargetIp %s in region %s (from %s). Skipping storage or storing as error.", probe.TargetIp, targetRegion, sourceIP)
				continue
			}
			dbEntry := &models.ProbeResult{
				SourceIP:     sourceIP,
				SourceRegion: sourceRegion,
				TargetIP:     probe.TargetIp,
				TargetRegion: targetRegion,
				TCPDelay:     probe.TcpDelay,
				ProbeTime:    probeTime,
			}
			err := models.InsertProbeResult(p.db, dbEntry)
			if err != nil {
				log.Printf("Error inserting probe result into DB for [%s (%s) -> %s (%s)]: %v",
					sourceIP, sourceRegion, probe.TargetIp, targetRegion, err)
			}
		}
	}
	log.Printf("Finished processing probe results for source IP: %s", sourceIP)
	return nil
}

/*func (p *Processor) ProcessProbeResults(sourceIP string, results []*pb.RegionProbeResult, fileManager *storage.FileManager) error {
	probeTime := time.Now()
	sourceRegion, err := models.GetNodeRegion(p.db, sourceIP)
	if err != nil {
		log.Printf("IP: %v", err)
		sourceRegion = "unknown"
	}
	processedPairs := make(map[string]map[string]bool)
	regionAvgDelays := make(map[string]int64)
	p.extractProbeResults(results, sourceIP, sourceRegion, probeTime, processedPairs, regionAvgDelays)
	p.fillMissingProbeResults(sourceIP, sourceRegion, probeTime, processedPairs, regionAvgDelays, fileManager)
	log.Printf(" %s ，", sourceIP)
	return nil
}

func (p *Processor) extractProbeResults(
	regionProbeResults []*pb.RegionProbeResult,
	sourceIP string,
	sourceRegion string,
	probeTime time.Time,
	processedPairs map[string]map[string]bool,
	regionAvgDelays map[string]int64,
) {
	for _, regionResult := range regionProbeResults {
		targetRegion := regionResult.Region
		for _, probe := range regionResult.IpProbes {
			if probe.TargetIp == "normal_avg" {
				regionAvgDelays[targetRegion] = probe.TcpDelay
				err := models.InsertProbeResult(p.db, &models.ProbeResult{
					SourceIP:     sourceIP,
					SourceRegion: sourceRegion,
					TargetIP:     "normal_avg",
					TargetRegion: targetRegion,
					TCPDelay:     probe.TcpDelay,
					ProbeTime:    probeTime,
				})
				if err != nil {
					log.Printf(" [%s->%s]: %v",
						sourceRegion, targetRegion, err)
				}
			} else {
				targetIP := probe.TargetIp
				if _, exists := processedPairs[targetIP]; !exists {
					processedPairs[targetIP] = make(map[string]bool)
				}
				processedPairs[targetIP][targetRegion] = true
				err := models.InsertProbeResult(p.db, &models.ProbeResult{
					SourceIP:     sourceIP,
					SourceRegion: sourceRegion,
					TargetIP:     targetIP,
					TargetRegion: targetRegion,
					TCPDelay:     probe.TcpDelay,
					ProbeTime:    probeTime,
				})
				if err != nil {
					log.Printf(" [%s->%s, %s]: %v",
						sourceIP, targetIP, targetRegion, err)
				}
			}
		}
	}
}
func (p *Processor) fillMissingProbeResults(
	sourceIP string,
	sourceRegion string,
	probeTime time.Time,
	processedPairs map[string]map[string]bool,
	regionAvgDelays map[string]int64,
	fileManager *storage.FileManager,
) {
	probeTasks := fileManager.GetNodeTasks(sourceIP)
	if probeTasks == nil || len(probeTasks) == 0 {
		log.Printf(" %s ，", sourceIP)
		return
	}
	nodeList := fileManager.GetNodeList()
	if nodeList == nil || len(nodeList.Nodes) == 0 {
		log.Printf("，")
		return
	}
	ipToRegion := make(map[string]string)
	for _, node := range nodeList.Nodes {
		ipToRegion[node.Ip] = node.Region
	}
	for _, task := range probeTasks {
		targetIP := task.TargetIp
		targetRegion, exists := ipToRegion[targetIP]
		if !exists {
			log.Printf(": IP %s ，unknown", targetIP)
			targetRegion = "unknown"
		}
		if regionsProcessed, exists := processedPairs[targetIP]; exists && regionsProcessed[targetRegion] {
			continue
		}
		avgDelay, hasAvg := regionAvgDelays[targetRegion]
		if !hasAvg || avgDelay < 0 {
			log.Printf(":  %s ，IP %s ",
				targetRegion, targetIP)
			continue
		}
		err := models.InsertProbeResult(p.db, &models.ProbeResult{
			SourceIP:     sourceIP,
			SourceRegion: sourceRegion,
			TargetIP:     targetIP,
			TargetRegion: targetRegion,
			TCPDelay:     avgDelay,
			ProbeTime:    probeTime,
		})
		if err != nil {
			log.Printf(" [%s->%s, %s]: %v",
				sourceIP, targetIP, targetRegion, err)
		}
	}
}
*/
