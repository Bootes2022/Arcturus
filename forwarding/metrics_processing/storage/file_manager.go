package storage

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"forwarding/metrics_processing/protocol"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type FileManager struct {
	dataDir              string
	nodeListFile         string
	probeTasksFile       string
	domainIPMappingsFile string
	nodeListHash         string
	probeTasksHash       string
	domainIPMappingsHash string
	nodeLock             sync.RWMutex
	taskLock             sync.RWMutex
	domainIPLock         sync.RWMutex
	nodeList             *protocol.NodeList
	probeTasks           []*protocol.ProbeTask
	domainIPMappings     []*protocol.DomainIPMapping
	hashUpdateTrigger    chan struct{}
}

func NewFileManager(dataDir string) (*FileManager, error) {

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf(": %v", err)
	}

	manager := &FileManager{
		dataDir:              dataDir,
		nodeListFile:         filepath.Join(dataDir, "node_list.json"),
		probeTasksFile:       filepath.Join(dataDir, "probe_tasks.json"),
		domainIPMappingsFile: filepath.Join(dataDir, "domain_ip_mappings.json"),
		hashUpdateTrigger:    make(chan struct{}, 1),
	}

	manager.loadFiles()

	manager.calculateHashes()

	go manager.hashUpdateListener()

	return manager, nil
}

func (fm *FileManager) loadFiles() {

	if data, err := os.ReadFile(fm.nodeListFile); err == nil {
		var nodeList protocol.NodeList
		if err := json.Unmarshal(data, &nodeList); err == nil {
			fm.nodeList = &nodeList
			log.Printf(": %d ", len(nodeList.Nodes))
		}
	}

	if data, err := os.ReadFile(fm.probeTasksFile); err == nil {
		var tasks []*protocol.ProbeTask
		if err := json.Unmarshal(data, &tasks); err == nil {
			fm.probeTasks = tasks
			log.Printf(": %d ", len(tasks))
		}
	}

	if data, err := os.ReadFile(fm.domainIPMappingsFile); err == nil {
		var mappings []*protocol.DomainIPMapping
		if err := json.Unmarshal(data, &mappings); err == nil {
			fm.domainIPMappings = mappings
			log.Printf("IP: %d ", len(mappings))
		}
	}
}

func (fm *FileManager) hashUpdateListener() {
	for range fm.hashUpdateTrigger {
		fm.calculateHashes()
	}
}

func (fm *FileManager) triggerHashUpdate() {

	select {
	case fm.hashUpdateTrigger <- struct{}{}:

	default:

	}
}

func (fm *FileManager) SaveNodeList(nodeList *protocol.NodeList) error {
	fm.nodeLock.Lock()
	defer fm.nodeLock.Unlock()

	fm.nodeList = nodeList
	data, err := json.MarshalIndent(nodeList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal node list: %v", err)
	}

	err = os.WriteFile(fm.nodeListFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write node list file: %v", err)
	}

	fm.triggerHashUpdate()
	return nil
}

func (fm *FileManager) SaveProbeTasks(tasks []*protocol.ProbeTask) error {
	fm.taskLock.Lock()
	defer fm.taskLock.Unlock()

	fm.probeTasks = tasks
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal probe tasks: %v", err)
	}

	err = os.WriteFile(fm.probeTasksFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write probe tasks file: %v", err)
	}

	fm.triggerHashUpdate()
	return nil
}

func (fm *FileManager) SaveDomainIPMappings(mappings []*protocol.DomainIPMapping) error {
	fm.domainIPLock.Lock()
	defer fm.domainIPLock.Unlock()

	fm.domainIPMappings = mappings
	data, err := json.MarshalIndent(mappings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal domain IP mappings: %v", err)
	}

	err = os.WriteFile(fm.domainIPMappingsFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write domain IP mappings file: %v", err)
	}

	fm.triggerHashUpdate()
	return nil
}

func (fm *FileManager) GetNodeList() *protocol.NodeList {
	fm.nodeLock.RLock()
	defer fm.nodeLock.RUnlock()
	return fm.nodeList
}

func (fm *FileManager) GetProbeTasks() []*protocol.ProbeTask {
	fm.taskLock.RLock()
	defer fm.taskLock.RUnlock()
	return fm.probeTasks
}

func (fm *FileManager) GetDomainIPMappings() []*protocol.DomainIPMapping {
	fm.domainIPLock.RLock()
	defer fm.domainIPLock.RUnlock()
	return fm.domainIPMappings
}

func (fm *FileManager) GetNodeListHash() string {
	return fm.nodeListHash
}

func (fm *FileManager) GetProbeTasksHash() string {
	return fm.probeTasksHash
}

func (fm *FileManager) GetDomainIPMappingsHash() string {
	return fm.domainIPMappingsHash
}

func calculateFileMD5(filepath string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			// ，
			return "", nil
		}
		return "", err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {

		}
	}(f)

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (fm *FileManager) calculateHashes() {

	if hash, err := calculateFileMD5(fm.nodeListFile); err == nil {
		fm.nodeListHash = hash
	} else {
		log.Printf(": %v", err)
	}

	if hash, err := calculateFileMD5(fm.probeTasksFile); err == nil {
		fm.probeTasksHash = hash
	} else {
		log.Printf(": %v", err)
	}

	if hash, err := calculateFileMD5(fm.domainIPMappingsFile); err == nil {
		fm.domainIPMappingsHash = hash
	} else {
		log.Printf("IP: %v", err)
	}

	log.Printf(" - : %s, : %s, IP: %s",
		fm.nodeListHash, fm.probeTasksHash, fm.domainIPMappingsHash)
}

func (fm *FileManager) IsInitialized() bool {
	fm.nodeLock.RLock()
	defer fm.nodeLock.RUnlock()

	fm.taskLock.RLock()
	defer fm.taskLock.RUnlock()

	fm.domainIPLock.RLock()
	defer fm.domainIPLock.RUnlock()

	return fm.nodeList != nil && fm.probeTasks != nil && fm.domainIPMappings != nil &&
		len(fm.nodeListHash) > 0 && len(fm.probeTasksHash) > 0 && len(fm.domainIPMappingsHash) > 0
}

func (fm *FileManager) GetConfigHashes() (string, string, string, error) {

	fm.calculateHashes()

	return fm.nodeListHash, fm.probeTasksHash, fm.domainIPMappingsHash, nil
}
