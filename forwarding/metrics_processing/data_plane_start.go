package metrics_processing

import (
	"context"
	t "data/common"
	"data/metrics_processing/client"
	collector2 "data/metrics_processing/collector"
	"data/metrics_processing/probe"
	"data/metrics_processing/protocol"
	"data/metrics_processing/storage"
	to "data/metrics_processing/topology"
	"data/router"
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
					log.Printf(": , ")
				} else {
					topology, err := to.ProcessRegionAssessments(syncResp.RegionAssessments, nodeList)
					if err != nil {
						log.Printf(": %v", err)
					} else {
						topologyManager := t.GetInstance()
						topologyManager.SetTopology(topology)

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
