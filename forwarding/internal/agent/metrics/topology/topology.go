package topology

import (
	"data/internal/agent/metrics/protocol"
	t "data/internal/common/topology"
	"data/internal/path"
	"log"
)

// HandleTopologyUpdates
func HandleTopologyUpdates(regionAssessments []*protocol.RegionPairAssessment, nodeList *protocol.NodeList) {
	topology, err := ProcessRegionAssessments(regionAssessments, nodeList)
	if err != nil {
		log.Printf(": %v", err)
		return
	}

	topologyManager := t.GetInstance()
	topologyManager.SetTopology(topology)

	network, ipToIndex, indexToIP, err := topologyManager.GetNetworkForKSP()
	if err == nil {
		pathManager := path.GetInstance()
		pathManager.CalculatePaths(network, ipToIndex, indexToIP)
	} else {
		log.Printf(": %v", err)
	}
}

func ProcessRegionAssessments(regionAssessments []*protocol.RegionPairAssessment, nodeList *protocol.NodeList) (*t.TopologyGraph, error) {

	topology := t.NewTopologyGraph()

	regionToIPs := make(map[string][]string)

	ipToRegion := make(map[string]string)

	for _, node := range nodeList.Nodes {
		regionToIPs[node.Region] = append(regionToIPs[node.Region], node.Ip)
		ipToRegion[node.Ip] = node.Region
	}

	for _, regionPair := range regionAssessments {
		sourceRegion := regionPair.Region1
		targetRegion := regionPair.Region2

		if len(sourceRegion) == 0 || len(targetRegion) == 0 {
			log.Printf(":  %s->%s", sourceRegion, targetRegion)
			continue
		}

		var defaultAssessment float32

		outlierPairs := make(map[string]map[string]float32)

		for _, ipPair := range regionPair.IpPairs {
			if ipPair.Ip1 == "default" && ipPair.Ip2 == "default" {

				defaultAssessment = ipPair.Assessment
				log.Printf(" %s->%s : %.2f", sourceRegion, targetRegion, defaultAssessment)
			} else {

				if _, exists := outlierPairs[ipPair.Ip1]; !exists {
					outlierPairs[ipPair.Ip1] = make(map[string]float32)
				}
				outlierPairs[ipPair.Ip1][ipPair.Ip2] = ipPair.Assessment

			}
		}

		sourceIPs, sourceExists := regionToIPs[sourceRegion]
		targetIPs, targetExists := regionToIPs[targetRegion]

		if !sourceExists || !targetExists {
			log.Printf(":  %s->%s ", sourceRegion, targetRegion)
			continue
		}

		for _, sourceIP := range sourceIPs {
			for _, targetIP := range targetIPs {

				if sourceIP == targetIP {
					continue
				}

				var assessmentValue float32

				if sourceOutliers, exists := outlierPairs[sourceIP]; exists {
					if targetValue, exists := sourceOutliers[targetIP]; exists {

						assessmentValue = targetValue
					} else {

						assessmentValue = defaultAssessment
					}
				} else {

					assessmentValue = defaultAssessment
				}

				topology.AddLink(sourceIP, targetIP, assessmentValue)
			}
		}
	}

	return topology, nil
}
