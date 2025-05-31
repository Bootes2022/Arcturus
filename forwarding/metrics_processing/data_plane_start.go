package metrics_processing

import (
	"context"
	t "forwarding/common"
	"forwarding/metrics_processing/client"
	collector2 "forwarding/metrics_processing/collector"
	"forwarding/metrics_processing/probe"
	"forwarding/metrics_processing/protocol"
	"forwarding/metrics_processing/storage"
	to "forwarding/metrics_processing/topology"
	"forwarding/router"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	// ServerAddr     = "104.238.153.192:8080" // This will now be passed as a parameter
	ReportInterval = 5 * time.Second       //
	DataDir        = "../../agent_storage" //
)

func StartDataPlane(ctx context.Context, serverAddr string) {
	log.Printf("Metrics processing starting. ServerAddr: %s", serverAddr)

	absDataDir, err := filepath.Abs(DataDir)
	if err != nil {
		log.Fatalf("%v", err)
	}

	if err := os.MkdirAll(absDataDir, 0755); err != nil {
		log.Fatalf("%v", err)
	}

	fileManager, err := storage.NewFileManager(absDataDir)
	if err != nil {
		log.Fatalf("%v", err)
	}
	log.Println("gRPC")

	grpcClient, err := client.NewGrpcClient(serverAddr, fileManager)
	if err != nil {
		log.Fatalf("gRPC: %v", err)
	}
	defer grpcClient.Close()

	info, err := collector2.CollectSystemInfo()
	if err != nil {
		log.Fatalf(": %v", err)
	}

	if !fileManager.IsInitialized() {

		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer initCancel()

		metrics := collector2.ConvertToProtoMetrics(info)

		if err := grpcClient.InitDataPlane(initCtx, metrics); err != nil {
			log.Fatalf(": %v", err)
		}

	}

	ticker := time.NewTicker(ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Second)
			info, err := collector2.CollectSystemInfo()
			if err != nil {
				log.Printf(": %v", err)
				syncCancel()
				continue
			}
			metrics := collector2.ConvertToProtoMetrics(info)
			regionProbeResults, err := probe.CollectRegionProbeResults(fileManager)
			if err != nil {
				log.Printf(": %v", err)
				regionProbeResults = []*protocol.RegionProbeResult{} //
			}
			syncResp, err := grpcClient.SyncMetrics(syncCtx, metrics, regionProbeResults)
			if err != nil {
				log.Printf(": %v", err)
				syncCancel()
				continue
			}
			if len(syncResp.RegionAssessments) > 0 {
				nodeList := fileManager.GetNodeList()
				if nodeList == nil {
					log.Printf("NodeList is nil, cannot process RegionAssessments.")
				} else {
					// 1. Process RegionAssessments to get the base topology
					baseTopology, err := to.ProcessRegionAssessments(syncResp.RegionAssessments, nodeList)
					if err != nil {
						log.Printf("Error processing RegionAssessments: %v", err)
					} else {
						// ---> Integrate local probe results
						var currentNodeIP string
						// collector2.GetIP() is correct as per imports.
						ip, err := collector2.GetIP()
						if err != nil {
							log.Printf("Error getting current node IP via collector2.GetIP(): %v. Local probe integration might be skipped.", err)
							// If current IP cannot be obtained, direct links cannot be added, but continue with topology based on RegionAssessments
						} else {
							currentNodeIP = ip
							log.Printf("Current node IP for local probe integration: %s", currentNodeIP)

							localProbeResults, err := probe.CollectRegionProbeResults(fileManager)
							if err != nil {
								log.Printf("Error collecting local probe results: %v", err)
							} else {
								if len(localProbeResults) > 0 {
									log.Printf("Integrating %d region(s) of local probe results.", len(localProbeResults))
									for _, regionResult := range localProbeResults {
										if len(regionResult.IpProbes) > 0 {
											log.Printf("Found %d IP probes in region '%s' from local results.", len(regionResult.IpProbes), regionResult.Region)
											for _, probeEntry := range regionResult.IpProbes {
												if probeEntry.TcpDelay >= 0 { // Valid probe
													baseTopology.AddLink(currentNodeIP, probeEntry.TargetIp, float32(probeEntry.TcpDelay))
													log.Printf("Added/Updated local direct link to topology: %s -> %s, delay: %dms", currentNodeIP, probeEntry.TargetIp, probeEntry.TcpDelay)
												} else {
													log.Printf("Skipping local probe link due to negative delay: %s -> %s, delay: %dms", currentNodeIP, probeEntry.TargetIp, probeEntry.TcpDelay)
												}
											}
										}
									}
								} else {
									log.Println("No local probe results to integrate.")
								}
							}
						}
						// <--- Integration of local probe results ends here

						// topologyManager is from "forwarding/common" (imported as 't')
						topologyManager := t.GetInstance()
						topologyManager.SetTopology(baseTopology) // SetTopology will now process the graph including local links

						network, ipToIndex, indexToIP, err := topologyManager.GetNetworkForKSP()
						if err == nil {
							pathManager := router.GetInstance()
							pathManager.CalculatePaths(network, ipToIndex, indexToIP)
						} else {
							log.Printf(": %v", err)
						}
					}
				}
			}
			syncCancel()
		case <-ctx.Done():
			return
		}
	}
}
