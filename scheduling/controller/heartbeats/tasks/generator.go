package tasks

import (
	"context"
	pb "control/controller/heartbeats/proto"
	"control/controller/heartbeats/storage"
	"control/models"
	"database/sql"
	"log"
	"sync"
	"time"
)

// TaskGenerator
type TaskGenerator struct {
	db          *sql.DB
	fileManager *storage.FileManager
	mutex       sync.Mutex
	lastGenTime time.Time
	interval    time.Duration //
}

// NewTaskGenerator
func NewTaskGenerator(db *sql.DB, fileManager *storage.FileManager, interval time.Duration) *TaskGenerator {
	return &TaskGenerator{
		db:          db,
		fileManager: fileManager,
		interval:    interval,
	}
}

// GenerateTasksIfNeeded
func (tg *TaskGenerator) GenerateTasksIfNeeded() bool {
	tg.mutex.Lock()
	defer tg.mutex.Unlock()

	//
	if time.Since(tg.lastGenTime) < tg.interval {
		return false //
	}

	//
	tg.lastGenTime = time.Now()

	//
	nodeInfos, err := models.QueryNodeInfo(tg.db)
	if err != nil {
		log.Printf(": %v", err)
		return false
	}

	// ,
	// : proto, NodeListNodeInfo  ip
	nodeList := &pb.NodeList{
		Nodes: nodeInfos,
	}
	if err := tg.fileManager.SaveNodeList(nodeList); err != nil {
		log.Printf(": %v", err)
	}

	// -IP
	domainIPMappings, err := models.QueryDomainIPMappings(tg.db)
	if err != nil {
		log.Printf("-IP: %v", err)
	} else {
		// -IP
		if err := tg.fileManager.SaveDomainIPMappings(domainIPMappings); err != nil {
			log.Printf("-IP: %v", err)
		}
	}

	//
	for _, sourceNode := range nodeInfos {
		sourceIP := sourceNode.Ip
		var tasks []*pb.ProbeTask
		for _, targetNode := range nodeInfos {
			targetIP := targetNode.Ip
			if sourceIP == targetIP {
				continue //
			}
			//
			task := &pb.ProbeTask{
				TaskId:   generateTaskID(sourceIP, targetIP),
				TargetIp: targetIP,
			}
			tasks = append(tasks, task)
		}

		//
		if err := tg.fileManager.SaveNodeTasks(sourceIP, tasks); err != nil {
			log.Printf(" %s : %v", sourceIP, err)
		}
	}

	return true //
}

// ID
func generateTaskID(sourceIP, targetIP string) string {
	return "task_" + sourceIP + "_" + targetIP
}

// StartTaskGenerator
func (tg *TaskGenerator) StartTaskGenerator(ctx context.Context) {
	ticker := time.NewTicker(tg.interval / 2) //
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if tg.GenerateTasksIfNeeded() {
				log.Println("-IP")
			}
		}
	}
}
