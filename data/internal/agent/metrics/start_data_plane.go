package metrics

import (
	"context"
	"data/internal/agent/metrics/client"
	"data/internal/agent/metrics/collector"
	"data/internal/agent/metrics/probe"
	"data/internal/agent/metrics/protocol"
	"data/internal/agent/metrics/storage"
	to "data/internal/agent/metrics/topology"
	t "data/internal/common/topology"
	"data/internal/path"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	ServerAddr     = "104.238.153.192:8080" //
	ReportInterval = 5 * time.Second        //
	DataDir        = "../../agent_storage"  //
)

func StartDataPlane(ctx context.Context) {

	absDataDir, err := filepath.Abs(DataDir)
	if err != nil {
		log.Fatalf(": %v", err)
	}

	if err := os.MkdirAll(absDataDir, 0755); err != nil {
		log.Fatalf(": %v", err)
	}

	fileManager, err := storage.NewFileManager(absDataDir)
	if err != nil {
		log.Fatalf(": %v", err)
	}
	log.Println("gRPC，...")

	grpcClient, err := client.NewGrpcClient(ServerAddr, fileManager)
	if err != nil {
		log.Fatalf("gRPC: %v", err)
	}
	defer grpcClient.Close()

	info, err := collector.CollectSystemInfo()
	if err != nil {
		log.Fatalf(": %v", err)
	}

	if !fileManager.IsInitialized() {
		log.Println("...")

		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer initCancel()

		metrics := collector.ConvertToProtoMetrics(info)

		if err := grpcClient.InitDataPlane(initCtx, metrics); err != nil {
			log.Fatalf(": %v", err)
		}
		log.Println("")
	}

	ticker := time.NewTicker(ReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

			syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Second)

			info, err := collector.CollectSystemInfo()
			if err != nil {
				log.Printf(": %v", err)
				syncCancel()
				continue
			}

			metrics := collector.ConvertToProtoMetrics(info)

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
							pathManager := path.GetInstance()
							pathManager.CalculatePaths(network, ipToIndex, indexToIP)
						} else {
							log.Printf(": %v", err)
						}
					}
				}
			}
			log.Println("")

			syncCancel()

		case <-ctx.Done():
			log.Println("，")
			return
		}
	}
}
