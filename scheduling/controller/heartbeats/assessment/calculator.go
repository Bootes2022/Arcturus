package assessment

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"runtime"
	"scheduling/config"
	pb "scheduling/controller/heartbeats/proto"
	linkevaluate "scheduling/link_evaluate"
	"scheduling/models"
	"scheduling/pool"
	"sync"
	"time"
)

type Calculator struct {
	db           *sql.DB
	mutex        sync.Mutex
	lastCalcTime time.Time
	interval     time.Duration
	assessments  []*pb.RegionPairAssessment
	assessmutex  sync.RWMutex
}

func NewAssessmentCalculator(db *sql.DB, interval time.Duration) *Calculator {
	return &Calculator{
		db:          db,
		interval:    interval,
		assessments: make([]*pb.RegionPairAssessment, 0),
	}
}

func (ac *Calculator) CalculateAssessmentsIfNeeded() bool {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()
	if time.Since(ac.lastCalcTime) < ac.interval {
		return false //
	}
	log.Println("...")
	startTime := time.Now()
	ac.lastCalcTime = time.Now()
	regionAssessments, err := ac.calculateRegionAssessments()
	if err != nil {
		log.Printf(": %v", err)
		return false
	}
	ac.assessmutex.Lock()
	ac.assessments = regionAssessments
	ac.assessmutex.Unlock()
	elapsed := time.Since(startTime)
	log.Printf("，：%v， %d ", elapsed, len(regionAssessments))
	return true
}

func (ac *Calculator) GetCachedAssessments() []*pb.RegionPairAssessment {
	ac.assessmutex.RLock()
	defer ac.assessmutex.RUnlock()
	result := make([]*pb.RegionPairAssessment, len(ac.assessments))
	copy(result, ac.assessments)
	return result
}

func (ac *Calculator) calculateRegionAssessments() ([]*pb.RegionPairAssessment, error) {

	regions, err := models.GetAllRegions(ac.db)
	if err != nil {
		return nil, err
	}

	netState, err := ac.getCurrentNetState()
	if err != nil {
		return nil, err
	}
	var assessments []*pb.RegionPairAssessment
	var assessmentsMutex sync.Mutex
	var wg sync.WaitGroup
	getIPPoolName := func(region1, region2 string) string {
		return fmt.Sprintf("IP_TASKS_POOL_%s_%s", region1, region2)
	}

	regionTaskFunc := func(payload interface{}) {
		defer wg.Done()
		task := payload.(*RegionPairTask)
		region1 := task.region1
		region2 := task.region2
		ipPoolName := getIPPoolName(region1, region2)
		assessment := &pb.RegionPairAssessment{
			Region1: region1,
			Region2: region2,
			IpPairs: []*pb.IPPairAssessment{},
		}
		sourceIPs, err := models.GetRegionIPs(ac.db, region1)
		if err != nil {
			log.Printf(" %s IP: %v", region1, err)
			return
		}
		targetIPs, err := models.GetRegionIPs(ac.db, region2)
		if err != nil {
			log.Printf(" %s IP: %v", region2, err)
			return
		}
		var ipPairs []*pb.IPPairAssessment
		var ipPairsMutex sync.Mutex
		var ipWg sync.WaitGroup

		ipTaskFunc := func(payload interface{}) {
			defer ipWg.Done()
			ipTask := payload.(*IpPairTask)
			sourceIP := ipTask.sourceIP
			targetIP := ipTask.targetIP
			result := linkevaluate.CalculateLinkWeight(ac.db, sourceIP, targetIP, netState)
			if result.Value > 0 {
				ipPair := &pb.IPPairAssessment{
					Ip1:        sourceIP,
					Ip2:        targetIP,
					Assessment: float32(result.Value),
				}
				ipPairsMutex.Lock()
				ipPairs = append(ipPairs, ipPair)
				ipPairsMutex.Unlock()
			}
		}
		ipPoolSize := runtime.NumCPU()
		pool.InitPool(ipPoolName, ipPoolSize, ipTaskFunc)
		defer pool.ReleasePool(ipPoolName)
		safeSubmitTask := func(task *IpPairTask) {
			p := pool.GetPool(ipPoolName)
			if p == nil {
				log.Printf(":  %s", ipPoolName)
				ipWg.Done()
				return
			}
			if err := p.Invoke(task); err != nil {
				log.Printf("IP: %v", err)
				ipWg.Done()
			}
		}
		for _, sourceIP := range sourceIPs {
			for _, targetIP := range targetIPs {
				ipWg.Add(1)
				ipTask := &IpPairTask{
					sourceIP: sourceIP,
					targetIP: targetIP,
				}
				safeSubmitTask(ipTask)
			}
		}
		ipWg.Wait()
		if len(ipPairs) > 0 {
			analyzer := NewAnalyzer()
			processedPairs, err := analyzer.AnalyzeOutliersAndNormalizeValues(ipPairs)
			if err != nil {
				log.Printf(" %s->%s : %v", region1, region2, err)
				assessment.IpPairs = ipPairs
			} else {
				assessment.IpPairs = processedPairs
			}
			assessmentsMutex.Lock()
			assessments = append(assessments, assessment)
			assessmentsMutex.Unlock()
		}
	}
	poolSize := runtime.NumCPU() * 2
	pool.InitPool(pool.RegionTasksPool, poolSize, regionTaskFunc)
	defer pool.ReleasePool(pool.RegionTasksPool)
	safeSubmitRegionTask := func(task *RegionPairTask) {
		p := pool.GetPool(pool.RegionTasksPool)
		if p == nil {
			log.Printf(": ")
			wg.Done()
			return
		}
		if err := p.Invoke(task); err != nil {
			log.Printf(": %v", err)
			wg.Done()
		}
	}
	regionPairCount := 0
	for i, region1 := range regions {
		for j, region2 := range regions {
			if i == j {
				continue
			}
			regionPairCount++
			wg.Add(1)
			task := &RegionPairTask{
				region1: region1,
				region2: region2,
			}
			safeSubmitRegionTask(task)
		}
	}
	log.Printf(" %d ", regionPairCount)
	wg.Wait()
	log.Printf(" %d ", len(assessments))
	return assessments, nil
}

func (ac *Calculator) getCurrentNetState() (config.NetState, error) {
	aboveCpuMeans, belowCpuMeans, aboveCpuVars, belowCpuVars, err := models.GetCPUPerformanceList(
		ac.db, linkevaluate.ThresholdCpuMean, linkevaluate.ThresholdCpuVar)
	if err != nil {
		return config.NetState{}, err
	}
	return config.NetState{
		AboveThresholdCpuMeans: aboveCpuMeans,
		AboveThresholdCpuVars:  aboveCpuVars,
		BelowThresholdCpuMeans: belowCpuMeans,
		BelowThresholdCpuVars:  belowCpuVars,
	}, nil
}
func (ac *Calculator) StartAssessmentCalculator(ctx context.Context) {
	initialDelay := 30 * time.Second
	log.Println("，", initialDelay, "")
	initialTimer := time.NewTimer(initialDelay)
	select {
	case <-ctx.Done():
		initialTimer.Stop()
		return
	case <-initialTimer.C:
		log.Println("")
		if ac.CalculateAssessmentsIfNeeded() {
			log.Println("")
		}
	}
	ticker := time.NewTicker(ac.interval / 4)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ac.CalculateAssessmentsIfNeeded() {
				log.Println("")
			}
		}
	}
}
