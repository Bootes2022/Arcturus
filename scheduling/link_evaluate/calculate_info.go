package linkevaluate

import (
	"database/sql"
	"fmt"
	"log"
	"scheduling/config"
	"scheduling/models"
)

const (
	ThresholdCpuMean float64 = 50
	ThresholdCpuVar  float64 = 50
	Weight           float64 = 1
)

func CalculateLinkWeight(db *sql.DB, ip1 string, ip2 string, net config.NetState) config.Result {

	avgDelay, err := models.GetDelay(db, ip1, ip2)
	if err != nil {
		log.Fatalf("Failed to query IPs: %v", err)
		return config.Result{}
	}

	stat, err := models.GetCpuStats(db, ip2)
	if err != nil {
		log.Printf("Failed to get CPU usage for %s: %v", ip2, err)
		return config.Result{}
	}
	node := config.NodeState{
		CpuMean: stat.Mean,
		CpuVar:  stat.Variance,
	}

	params := SystemParams{
		ThresholdCpuMean: ThresholdCpuMean,
		ThresholdCpuVar:  ThresholdCpuVar,
		Weight:           Weight,
	}

	normalCpuMean, normalCpuVar := params.Normalize(&node, &net)
	QMean, QVar, err := models.QueryVirtualQueueCPUByIP(db, ip1, ip2)
	if err != nil {
		log.Printf("Failed to query virtual CPU %v", err)
	}

	e := Evaluation{
		Delay:         avgDelay,
		NormalCpuMean: normalCpuMean,
		NormalCpuVar:  normalCpuVar,
		QMean:         QMean,
		QVar:          QVar,
		Params:        params,
		State:         net,
	}
	VirtualQueueCPUMean := e.UpdateQMean()
	VirtualQueueCPUVariance := e.UpdateQVar()
	finalValue := e.DriftPlusPenalty()
	fmt.Println("：VirtualQueueCPUMean，VirtualQueueCPUVariance，finalValue:", VirtualQueueCPUMean, VirtualQueueCPUVariance, finalValue)
	err = models.UpdateVirtualQueueAndCPUMetrics(db, ip1, ip2, avgDelay, stat.Mean, stat.Variance, VirtualQueueCPUMean, VirtualQueueCPUVariance)
	if err != nil {
		return config.Result{}
	}

	res := config.Result{
		Ip1:   ip1,
		Ip2:   ip2,
		Value: finalValue,
	}
	return res
}
