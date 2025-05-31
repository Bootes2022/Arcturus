package client

import (
	"context"
	"fmt"
	protocol2 "forwarding/metrics_processing/protocol"
	"forwarding/metrics_processing/storage"
	"log"
	"strings"
	"time"

	"google.golang.org/grpc"
)

type GrpcClient struct {
	metricsClient protocol2.MetricsServiceClient
	configClient  protocol2.ConfigServiceClient
	faultClient   protocol2.FaultServiceClient
	conn          *grpc.ClientConn
	fileManager   *storage.FileManager
}

type UpdateStatus struct {
	NodeListUpdated         bool
	ProbeTasksUpdated       bool
	DomainIPMappingsUpdated bool
}

func NewGrpcClient(address string, fileManager *storage.FileManager) (*GrpcClient, error) {
	var conn *grpc.ClientConn
	var err error

	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		conn, err = grpc.Dial(address,
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithTimeout(10*time.Second))
		if err == nil {
			break
		}
		if i < maxRetries-1 {

			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to control plane after %d retries: %v", maxRetries, err)
	}

	metricsClient := protocol2.NewMetricsServiceClient(conn)
	configClient := protocol2.NewConfigServiceClient(conn)
	faultClient := protocol2.NewFaultServiceClient(conn)

	return &GrpcClient{
		metricsClient: metricsClient,
		configClient:  configClient,
		faultClient:   faultClient,
		conn:          conn,
		fileManager:   fileManager,
	}, nil
}

func (g *GrpcClient) InitDataPlane(ctx context.Context, metrics *protocol2.Metrics) error {

	req := &protocol2.InitRequest{
		Metrics: metrics,
	}

	resp, err := g.metricsClient.InitDataPlane(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to initialize data plane: %v", err)
	}

	log.Printf("Received initial data from control plane, status: %s, message: %s", resp.Status, resp.Message)

	return nil
}

func (g *GrpcClient) SyncMetrics(ctx context.Context, metrics *protocol2.Metrics, regionProbeResults []*protocol2.RegionProbeResult) (*protocol2.SyncResponse, error) {

	nodeListHash, probeTasksHash, domainIPMappingsHash, err := g.fileManager.GetConfigHashes()
	if err != nil {
		log.Printf(": %v", err)

		nodeListHash = ""
		probeTasksHash = ""
		domainIPMappingsHash = ""
	}

	req := &protocol2.SyncRequest{
		Metrics:              metrics,
		NodeListHash:         nodeListHash,
		ProbeTasksHash:       probeTasksHash,
		DomainIpMappingsHash: domainIPMappingsHash,
		RegionProbeResults:   regionProbeResults,
	}

	resp, err := g.metricsClient.SyncMetrics(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to sync metrics_processing: %v", err)
	}

	log.Printf("Server returned status: %s, message: %s", resp.Status, resp.Message)

	updateStatus := &UpdateStatus{}

	if resp.NeedUpdateNodeList && resp.NodeList != nil {
		if err := g.fileManager.SaveNodeList(resp.NodeList); err != nil {
			log.Printf("Warning: failed to save updated node list: %v", err)
		} else {
			log.Printf("Updated node list with %d nodes", len(resp.NodeList.Nodes))
			updateStatus.NodeListUpdated = true
		}
	}

	if resp.NeedUpdateProbeTasks && resp.ProbeTasks != nil {
		if err := g.fileManager.SaveProbeTasks(resp.ProbeTasks); err != nil {
			log.Printf("Warning: failed to save updated probe tasks: %v", err)
		} else {
			log.Printf("Updated probe tasks with %d tasks", len(resp.ProbeTasks))
			updateStatus.ProbeTasksUpdated = true
		}
	}

	if resp.NeedUpdateDomainIpMappings && resp.DomainIpMappings != nil {
		if err := g.fileManager.SaveDomainIPMappings(resp.DomainIpMappings); err != nil {
			log.Printf("Warning: failed to save updated domain IP mappings: %v", err)
		} else {
			log.Printf("Updated domain IP mappings with %d mappings", len(resp.DomainIpMappings))
			updateStatus.DomainIPMappingsUpdated = true
		}
	}

	if updateStatus.HasUpdates() {
		log.Printf(": %s", updateStatus.Summary())
	} else {
		log.Printf(": ，")
	}

	return resp, nil
}

func (g *GrpcClient) ReceiveConfig(ctx context.Context, req *protocol2.PushConfigRequest) error {
	resp, err := g.configClient.PushConfig(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to receive config: %v", err)
	}

	if resp.Status != "ok" {
		log.Printf("Warning: controller returned non-OK status for config push: %s, message: %s", resp.Status, resp.Message)
		return fmt.Errorf("controller error: %s", resp.Message)
	}

	updateStatus := &UpdateStatus{}

	if req.NodeList != nil {
		if err := g.fileManager.SaveNodeList(req.NodeList); err != nil {
			log.Printf("Warning: failed to save pushed node list: %v", err)
		} else {
			log.Printf("Saved pushed node list with %d nodes", len(req.NodeList.Nodes))
			updateStatus.NodeListUpdated = true
		}
	}

	if req.ProbeTasks != nil {
		if err := g.fileManager.SaveProbeTasks(req.ProbeTasks); err != nil {
			log.Printf("Warning: failed to save pushed probe tasks: %v", err)
		} else {
			log.Printf("Saved pushed probe tasks with %d tasks", len(req.ProbeTasks))
			updateStatus.ProbeTasksUpdated = true
		}
	}

	if req.DomainIpMappings != nil {
		if err := g.fileManager.SaveDomainIPMappings(req.DomainIpMappings); err != nil {
			log.Printf("Warning: failed to save pushed domain IP mappings: %v", err)
		} else {
			log.Printf("Saved pushed domain IP mappings with %d mappings", len(req.DomainIpMappings))
			updateStatus.DomainIPMappingsUpdated = true
		}
	}

	if updateStatus.HasUpdates() {
		log.Printf(": %s", updateStatus.Summary())
	} else {
		log.Printf(": ")
	}

	return nil
}

func (g *GrpcClient) ReportFault(ctx context.Context, faultInfo *protocol2.FaultInfo) error {

	req := &protocol2.ReportFaultRequest{
		FaultInfo: faultInfo,
	}

	resp, err := g.faultClient.ReportFault(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to report fault: %v", err)
	}

	if resp.Status != "ok" {
		log.Printf("Warning: controller returned non-OK status for fault report: %s, message: %s", resp.Status, resp.Message)
		return fmt.Errorf("controller error: %s", resp.Message)
	}

	log.Printf("Fault reported successfully: %s", faultInfo.FaultId)
	return nil
}

func (us *UpdateStatus) HasUpdates() bool {
	return us.NodeListUpdated || us.ProbeTasksUpdated || us.DomainIPMappingsUpdated
}

func (us *UpdateStatus) Summary() string {
	var updated []string
	if us.NodeListUpdated {
		updated = append(updated, "")
	}
	if us.ProbeTasksUpdated {
		updated = append(updated, "")
	}
	if us.DomainIPMappingsUpdated {
		updated = append(updated, "IP")
	}

	if len(updated) == 0 {
		return ""
	}

	return fmt.Sprintf(": %s", strings.Join(updated, ", "))
}

func (g *GrpcClient) Close() {
	g.conn.Close()
}
