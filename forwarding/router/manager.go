package router

import (
	"data/metrics_processing/collector"
	"data/metrics_processing/storage"
	"data/scheduling_algorithms/k_shortest"
	"log"
	"sync"
)

type PathManager struct {
	pathChan chan []k_shortest.PathWithIP

	latestPaths []k_shortest.PathWithIP

	mu sync.RWMutex

	sourceIP      string
	destinationIP string

	k int
}

var (
	instance *PathManager
	once     sync.Once
)

// fileManagerInstance是当前应用使用的FileManager实例
var (
	fileManagerInstance *storage.FileManager
	fileManagerOnce     sync.Once
)

// getFileManager 获取FileManager的单例实例
func getFileManager() *storage.FileManager {
	fileManagerOnce.Do(func() {
		var err error
		fileManagerInstance, err = storage.NewFileManager("./config")
		if err != nil {
			log.Printf("无法创建FileManager: %v", err)
			return
		}
	})
	return fileManagerInstance
}

// GetTargetIPByDomain 从域名映射中查找对应的目标IP
func GetTargetIPByDomain(domain string) string {
	// 获取FileManager实例
	fileManager := getFileManager()
	if fileManager == nil {
		log.Printf("无法获取FileManager实例")
		return ""
	}

	// 获取所有域名映射
	mappings := fileManager.GetDomainIPMappings()
	if mappings == nil {
		log.Printf("域名映射为空")
		return ""
	}

	// 查找匹配的域名
	for _, mapping := range mappings {
		if mapping.Domain == domain {
			log.Printf("找到域名 %s 的映射IP: %s", domain, mapping.Ip)
			return mapping.Ip
		}
	}

	// 未找到映射时返回空字符串
	log.Printf("未找到域名 %s 的映射", domain)
	return ""
}

// GetDefaultTargetIP 获取默认目标IP
// 返回域名映射中的第一条记录的IP
// 如果没有映射记录，则返回空字符串
func GetDefaultTargetIP() string {
	// 获取FileManager实例
	fileManager := getFileManager()
	if fileManager == nil {
		log.Printf("无法获取FileManager实例")
		return ""
	}

	// 获取所有域名映射
	mappings := fileManager.GetDomainIPMappings()

	// 如果有映射记录，返回第一条记录的IP
	if mappings != nil && len(mappings) > 0 {
		defaultIP := mappings[0].Ip
		log.Printf("使用第一个域名映射的IP: %s 作为默认目标", defaultIP)
		return defaultIP
	}

	// 没有映射记录时返回空字符串
	log.Printf("域名映射为空，无默认目标IP")
	return ""
}

func GetInstance() *PathManager {
	once.Do(func() {
		ip, err := collector.GetIP()
		if err != nil {
			return
		}
		// 使用GetDefaultTargetIP获取默认目标IP
		defaultTargetIP := GetDefaultTargetIP()
		instance = &PathManager{
			pathChan:      make(chan []k_shortest.PathWithIP, 1),
			latestPaths:   nil,
			sourceIP:      ip,
			destinationIP: defaultTargetIP,
			k:             2,
		}

		go instance.pathListener()
		log.Println("")
	})
	return instance
}

func (pm *PathManager) pathListener() {
	for paths := range pm.pathChan {
		pm.mu.Lock()
		pm.latestPaths = paths
		pm.mu.Unlock()
		log.Printf(" %d ", len(paths))
	}
}

func (pm *PathManager) CalculatePaths(network k_shortest.Network,
	ipToIndex map[string]int,
	indexToIP map[int]string) {

	sourceIdx, srcExists := ipToIndex[pm.sourceIP]
	destIdx, destExists := ipToIndex[pm.destinationIP]

	if !srcExists || !destExists {
		log.Printf(": IPIP (: %v, : %v)",
			srcExists, destExists)
		return
	}

	flow := k_shortest.Flow{Source: sourceIdx, Destination: destIdx}

	paths := k_shortest.KShortest(network, flow, pm.k, 3, 2) // theta

	var pathsWithIP []k_shortest.PathWithIP

	totalLatency := 0
	for _, p := range paths {
		totalLatency += p.Latency
	}

	if len(paths) == 0 || totalLatency == 0 {
		log.Println("")

		select {
		case pm.pathChan <- []k_shortest.PathWithIP{}:

		default:

		}
		return
	}

	for _, p := range paths {

		ipNodes := make([]string, len(p.Nodes))
		for j, node := range p.Nodes {
			ipNodes[j] = indexToIP[node]
		}

		var weight int
		if p.Latency == 0 {

			weight = 100
			log.Printf(": ，: %d", weight)
		} else {
			weight = totalLatency / p.Latency
		}

		pathsWithIP = append(pathsWithIP, k_shortest.PathWithIP{
			IPList:  ipNodes,
			Latency: p.Latency,
			Weight:  weight,
		})

		log.Printf(": : %v, : %d, : %d", ipNodes, p.Latency, weight)
	}

	select {
	case pm.pathChan <- pathsWithIP:

	default:

		<-pm.pathChan
		pm.pathChan <- pathsWithIP
	}
}

func (pm *PathManager) GetPaths() []k_shortest.PathWithIP {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.latestPaths == nil {
		return []k_shortest.PathWithIP{}
	}

	paths := make([]k_shortest.PathWithIP, len(pm.latestPaths))
	copy(paths, pm.latestPaths)
	return paths
}

func (pm *PathManager) SetSourceDestination(source, destination string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.sourceIP = source
	pm.destinationIP = destination
}
