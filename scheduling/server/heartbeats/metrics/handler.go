// metric/handler.go
package metrics

import (
	"context"
	"control/models"
	"control/server/heartbeats/assessment"
	"control/server/heartbeats/config"
	pb "control/server/heartbeats/proto"
	"control/server/heartbeats/storage"
	"control/server/heartbeats/tasks"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type Handler struct {
	pb.UnimplementedMetricsServiceServer
	db               *sql.DB
	fileManager      *storage.FileManager
	taskGenerator    *tasks.TaskGenerator
	configPusher     *config.Pusher
	assessmentCalc   *assessment.Calculator
	processor        *Processor
	generatorStarted atomic.Bool
	calcStarted      atomic.Bool
	etcdSync         *EtcdSync
	lastNodeInit     time.Time
	initMutex        sync.Mutex
	initTimer        *time.Timer
	bufferPeriod     time.Duration
}

func NewHandler(
	db *sql.DB,
	fileManager *storage.FileManager,
	taskGenerator *tasks.TaskGenerator,
	configPusher *config.Pusher,
	etcdSync *EtcdSync, // etcd
	assessmentCalc *assessment.Calculator,
	bufferPeriod time.Duration,
) *Handler {
	handler := &Handler{
		db:             db,
		fileManager:    fileManager,
		taskGenerator:  taskGenerator,
		configPusher:   configPusher,
		etcdSync:       etcdSync,
		assessmentCalc: assessmentCalc,
		processor:      NewProcessor(db),
		bufferPeriod:   bufferPeriod,
	}

	handler.generatorStarted.Store(false)
	handler.calcStarted.Store(false)

	return handler
}

func (h *Handler) InitDataPlane(ctx context.Context, req *pb.InitRequest) (*pb.SimpleResponse, error) {
	nodeIP := req.Metrics.Ip
	log.Printf(" %s ", nodeIP)

	err := models.InsertMetricsInfo(h.db, req.Metrics)
	if err != nil {
		return &pb.SimpleResponse{
			Status:  "error",
			Message: fmt.Sprintf(": %v", err),
		}, nil
	}

	if !h.generatorStarted.Load() {
		if h.generatorStarted.CompareAndSwap(false, true) {
			log.Println("，")
			go h.taskGenerator.StartTaskGenerator(context.Background())
		}
	}

	h.initMutex.Lock()
	defer h.initMutex.Unlock()

	h.lastNodeInit = time.Now()

	if h.initTimer != nil {
		h.initTimer.Stop()
	}

	h.initTimer = time.AfterFunc(h.bufferPeriod, func() {
		h.handleBufferPeriodEnd()
	})

	log.Printf(" %s ， %v ", nodeIP, h.bufferPeriod)

	return &pb.SimpleResponse{
		Status:  "ok",
		Message: fmt.Sprintf("，"),
	}, nil
}

func (h *Handler) handleBufferPeriodEnd() {
	log.Printf(" %v ，", h.bufferPeriod)

	if h.taskGenerator.GenerateTasksIfNeeded() {
		log.Println("，")

		nodeList := h.fileManager.GetNodeList()
		if nodeList != nil && len(nodeList.Nodes) > 0 {

			h.configPusher.PushToAllNodes(nodeList, h.fileManager)
			log.Printf(" %d ", len(nodeList.Nodes))
		} else {
			log.Println("nil，")
		}
	} else {
		log.Println("")
	}

	if !h.calcStarted.Load() {
		if h.calcStarted.CompareAndSwap(false, true) {
			log.Println("，")
			go h.assessmentCalc.StartAssessmentCalculator(context.Background())
		}
	}
}

func (h *Handler) SyncMetrics(ctx context.Context, req *pb.SyncRequest) (*pb.SyncResponse, error) {

	err := models.InsertMetricsInfo(h.db, req.Metrics)
	if err != nil {
		return &pb.SyncResponse{
			Status:  "error",
			Message: fmt.Sprintf(": %v", err),
		}, nil
	}

	nodeListNeedsUpdate := false
	probeTasksNeedUpdate := false
	domainIPMappingsNeedUpdate := false

	if req.NodeListHash != h.fileManager.GetNodeListHash() {
		nodeListNeedsUpdate = true
	}

	if req.ProbeTasksHash != h.fileManager.GetNodeTasksHash(req.Metrics.Ip) {
		probeTasksNeedUpdate = true
	}

	if req.DomainIpMappingsHash != h.fileManager.GetDomainIPMappingsHash() {
		domainIPMappingsNeedUpdate = true
	}

	resp := &pb.SyncResponse{
		Status:                     "ok",
		Message:                    "",
		NeedUpdateNodeList:         nodeListNeedsUpdate,
		NeedUpdateProbeTasks:       probeTasksNeedUpdate,
		NeedUpdateDomainIpMappings: domainIPMappingsNeedUpdate,
	}

	if nodeListNeedsUpdate {
		resp.NodeList = h.fileManager.GetNodeList()
	}

	if probeTasksNeedUpdate {
		resp.ProbeTasks = h.fileManager.GetNodeTasks(req.Metrics.Ip)
	}

	if domainIPMappingsNeedUpdate {
		resp.DomainIpMappings = h.fileManager.GetDomainIPMappings()
	}

	if len(req.RegionProbeResults) > 0 {
		err := h.processor.ProcessProbeResults(req.Metrics.Ip, req.RegionProbeResults, h.fileManager)
		if err != nil {
			log.Printf(": %v", err)

		}
	}

	regionAssessments := h.assessmentCalc.GetCachedAssessments()
	if len(regionAssessments) > 0 {
		resp.RegionAssessments = regionAssessments
		log.Printf(" %s  %d ", req.Metrics.Ip, len(regionAssessments))
	}

	return resp, nil
}

func (h *Handler) StartBackgroundServicesWhenReady(ctx context.Context) {

	if h.generatorStarted.Load() && h.calcStarted.Load() {
		log.Println("，")
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("，")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:

			if h.generatorStarted.Load() && h.calcStarted.Load() {
				log.Println("，")
				return
			}

			nodesCount, err := models.CountMetricsNodes(h.db)
			if err != nil {
				log.Printf(": %v", err)
				continue
			}

			if nodesCount > 0 {

				h.startBackgroundServicesIfNeeded(ctx)

				if h.generatorStarted.Load() && h.calcStarted.Load() {
					log.Println("，")
					return
				}
			}

			log.Println("，: ", nodesCount)
		}
	}
}

func (h *Handler) startBackgroundServicesIfNeeded(ctx context.Context) {

	if !h.generatorStarted.Load() {
		if h.generatorStarted.CompareAndSwap(false, true) {
			log.Println("，")
			go h.taskGenerator.StartTaskGenerator(ctx)
		}
	}

	if !h.calcStarted.Load() {
		if h.calcStarted.CompareAndSwap(false, true) {
			log.Println("，")
			go h.assessmentCalc.StartAssessmentCalculator(ctx)
		}
	}
}
