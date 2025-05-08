// config/pusher.go
package config

import (
	"context"
	pb "control/controller/heartbeats/proto"
	"control/controller/heartbeats/storage"
	"control/controller/heartbeats/utils"
	"control/pool"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"log"
	"sync"
	"time"
)

type Pusher struct {
	connections map[string]*grpc.ClientConn
	mu          sync.RWMutex
}

func NewPusher() *Pusher {

	pool.InitPool(pool.ConfigPushPool, 100, pushTaskHandler)

	return &Pusher{
		connections: make(map[string]*grpc.ClientConn),
	}
}

func pushTaskHandler(payload interface{}) {

	params := payload.([]interface{})
	ip := params[0].(string)
	nodeList := params[1].(*pb.NodeList)
	tasks := params[2].([]*pb.ProbeTask)
	domainIPMappings := params[3].([]*pb.DomainIPMapping)
	pusher := params[4].(*Pusher)

	pusher.doPushToNode(ip, nodeList, tasks, domainIPMappings)
}

func (p *Pusher) PushToAllNodes(nodeList *pb.NodeList, fileManager *storage.FileManager) {

	domainIPMappings := fileManager.GetDomainIPMappings()

	for _, node := range nodeList.Nodes {
		ip := node.Ip
		log.Printf(" %s (%s) ", ip, node.Region)

		tasks := fileManager.GetNodeTasks(ip)

		configPool := pool.GetPool(pool.ConfigPushPool)
		if configPool != nil {
			err := configPool.Invoke([]interface{}{ip, nodeList, tasks, domainIPMappings, p})
			if err != nil {
				log.Printf("，IP: %s, : %v", ip, err)
			}
		} else {
			log.Printf("， %s", ip)
			p.doPushToNode(ip, nodeList, tasks, domainIPMappings)
		}
	}
}

func (p *Pusher) doPushToNode(ip string, nodeList *pb.NodeList, tasks []*pb.ProbeTask, domainIPMappings []*pb.DomainIPMapping) {

	conn, err := p.getConnection(ip)
	if err != nil {
		log.Printf(" %s : %v", ip, err)
		return
	}

	client := pb.NewConfigServiceClient(conn)

	req := &pb.PushConfigRequest{
		NodeList:         nodeList,
		ProbeTasks:       tasks,
		DomainIpMappings: domainIPMappings,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.PushConfig(ctx, req)
	if err != nil {
		log.Printf(" %s : %v", ip, err)
		return
	}

	log.Printf(" %s : %s", ip, resp.Status)
}

func (p *Pusher) getConnection(ip string) (*grpc.ClientConn, error) {

	p.mu.RLock()
	conn, ok := p.connections[ip]
	if ok && conn.GetState() != connectivity.Shutdown {
		p.mu.RUnlock()
		return conn, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok = p.connections[ip]; ok && conn.GetState() != connectivity.Shutdown {
		return conn, nil
	}

	if conn != nil {
		delete(p.connections, ip)
	}

	addr := fmt.Sprintf("%s:50052", ip)
	newConn, err := utils.CreateGRPCConnection(addr, 3*time.Second)
	if err != nil {
		return nil, err
	}

	p.connections[ip] = newConn
	return newConn, nil
}

func (p *Pusher) Release() {

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		conn.Close()
	}
	p.connections = make(map[string]*grpc.ClientConn)

	pool.ReleasePool(pool.ConfigPushPool)
}
